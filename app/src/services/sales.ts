import { and, asc, desc, eq, getTableColumns, like } from 'drizzle-orm';
import { customers, refundItems, refunds, saleItems, sales } from '@/db/schema';
import type { Customer, Refund, Sale, SaleItem } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { pointsToReverse } from '@/domain/loyalty';
import { buildReceipt, buildRefundReceipt } from '@/domain/receipt';
import { getBusiness } from './business';
import { PosError } from './errors';
import { recordMovement } from './inventory';

export type RefundDetail = Refund & { items: { saleItemId: number; name: string; qty: number; amount: number }[] };

export interface SaleDetail {
  sale: Sale;
  items: SaleItem[];
  refunds: RefundDetail[];
  customer: Customer | undefined;
}

export function getSale(db: PosDb, id: number): SaleDetail | undefined {
  const sale = db.select().from(sales).where(eq(sales.id, id)).get();
  if (!sale) return undefined;
  const items = db.select().from(saleItems).where(eq(saleItems.saleId, id)).orderBy(asc(saleItems.id)).all();
  const refundRows = db.select().from(refunds).where(eq(refunds.saleId, id)).orderBy(asc(refunds.id)).all();
  const refundDetails: RefundDetail[] = refundRows.map((r) => ({
    ...r,
    items: db
      .select({
        saleItemId: refundItems.saleItemId,
        name: saleItems.name,
        qty: refundItems.qty,
        amount: refundItems.amount,
      })
      .from(refundItems)
      .innerJoin(saleItems, eq(refundItems.saleItemId, saleItems.id))
      .where(eq(refundItems.refundId, r.id))
      .all(),
  }));
  const customer = sale.customerId
    ? db.select().from(customers).where(eq(customers.id, sale.customerId)).get()
    : undefined;
  return { sale, items, refunds: refundDetails, customer };
}

export function listSales(db: PosDb, opts: { status?: string; query?: string; limit?: number } = {}): Sale[] {
  const conds = [];
  if (opts.status) conds.push(eq(sales.status, opts.status));
  if (opts.query?.trim()) conds.push(like(sales.receiptNo, `%${opts.query.trim()}%`));
  return db
    .select()
    .from(sales)
    .where(conds.length ? and(...conds) : undefined)
    .orderBy(desc(sales.id))
    .limit(opts.limit ?? 200)
    .all();
}

export function receiptText(db: PosDb, d: SaleDetail): string {
  const s = d.sale;
  const profile = getBusiness(db);
  return buildReceipt(
    {
      receiptNo: s.receiptNo,
      createdAt: s.createdAt,
      status: s.status,
      customerName: d.customer?.name,
      items: d.items.map((i) => ({ name: i.name, qty: i.qty, unitPrice: i.unitPrice, lineDiscount: i.lineDiscount })),
      subtotal: s.subtotal,
      discountAmount: s.discountAmount,
      pointsDiscount: s.pointsDiscount,
      total: s.total,
      paymentMethod: s.paymentMethod,
      cryptoAmount: s.cryptoAmount,
      txHash: s.txHash,
      failureReason: s.failureReason,
      pointsEarned: s.pointsEarned,
      pointsRedeemed: s.pointsRedeemed,
      refundedTotal: s.refundedTotal,
    },
    { merchant: profile?.name, website: profile?.website, address: profile?.address },
  );
}

export function refundReceiptText(db: PosDb, d: SaleDetail, refund: RefundDetail): string {
  const profile = getBusiness(db);
  return buildRefundReceipt(
    {
      receiptNo: d.sale.receiptNo,
      refundedAt: refund.createdAt,
      items: refund.items,
      amount: refund.amount,
      reason: refund.reason,
      restocked: refund.restocked,
    },
    { merchant: profile?.name, website: profile?.website, address: profile?.address },
  );
}

/** Cancels a completed sale that has no refunds: puts all stock back and reverses loyalty points. */
export function voidSale(db: PosDb, saleId: number, reason = '', now = Date.now()): void {
  db.transaction((tx) => {
    const d = getSale(tx, saleId);
    if (!d) throw new PosError('Sale not found');
    if (d.sale.status === 'voided') throw new PosError('This sale is already voided');
    if (d.sale.status !== 'completed') throw new PosError('Only completed sales without refunds can be voided');

    for (const item of d.items) {
      recordMovement(tx, {
        productId: item.productId,
        type: 'void',
        quantity: item.qty,
        unitCost: item.unitCost,
        reference: d.sale.receiptNo,
        note: reason || null,
        now,
      });
    }
    if (d.customer) {
      const next = Math.max(0, d.customer.loyaltyPoints + d.sale.pointsRedeemed - d.sale.pointsEarned);
      tx.update(customers).set({ loyaltyPoints: next }).where(eq(customers.id, d.customer.id)).run();
    }
    tx.update(sales)
      .set({ status: 'voided', voidedAt: now, voidReason: reason.trim() || 'No reason given' })
      .where(eq(sales.id, saleId))
      .run();
  });
}

export interface RefundInput {
  saleId: number;
  items: { saleItemId: number; qty: number }[];
  reason?: string;
  restock?: boolean;
  now?: number;
}

/** Full or partial return. Items go back to stock unless restock is false (e.g. damaged goods). */
export function refundSale(db: PosDb, input: RefundInput): { refundId: number; amount: number; pointsReversed: number } {
  const now = input.now ?? Date.now();
  const restock = input.restock ?? true;
  const requested = input.items.filter((i) => i.qty > 0);
  if (!requested.length) throw new PosError('Choose at least one item to refund');
  if (new Set(requested.map((i) => i.saleItemId)).size !== requested.length) {
    throw new PosError('Each item can only appear once in a refund');
  }

  return db.transaction((tx) => {
    const d = getSale(tx, input.saleId);
    if (!d) throw new PosError('Sale not found');
    if (d.sale.status !== 'completed' && d.sale.status !== 'partially_refunded') {
      throw new PosError('This sale cannot be refunded');
    }

    let total = 0;
    const pending: { item: SaleItem; qty: number; amount: number }[] = [];
    for (const req of requested) {
      if (!Number.isInteger(req.qty)) throw new PosError('Quantities must be whole numbers');
      const item = d.items.find((i) => i.id === req.saleItemId);
      if (!item) throw new PosError('Item does not belong to this sale');
      const remaining = item.qty - item.refundedQty;
      if (req.qty > remaining) throw new PosError(`Only ${remaining} of ${item.name} left to refund`);
      const lineTotal = item.unitPrice * item.qty - item.lineDiscount;
      const amount =
        req.qty === remaining
          ? lineTotal - item.refundedAmount // last piece takes the rounding remainder
          : Math.floor((lineTotal * req.qty) / item.qty);
      total += amount;
      pending.push({ item, qty: req.qty, amount });
    }

    const refund = tx
      .insert(refunds)
      .values({
        saleId: d.sale.id,
        createdAt: now,
        amount: total,
        reason: input.reason?.trim() || null,
        restocked: restock,
        pointsReversed: 0,
      })
      .returning()
      .get();

    for (const p of pending) {
      tx.insert(refundItems)
        .values({
          refundId: refund.id,
          saleItemId: p.item.id,
          qty: p.qty,
          amount: p.amount,
          costRecovered: restock ? p.item.unitCost * p.qty : 0,
        })
        .run();
      tx.update(saleItems)
        .set({ refundedQty: p.item.refundedQty + p.qty, refundedAmount: p.item.refundedAmount + p.amount })
        .where(eq(saleItems.id, p.item.id))
        .run();
      if (restock) {
        recordMovement(tx, {
          productId: p.item.productId,
          type: 'refund',
          quantity: p.qty,
          unitCost: p.item.unitCost,
          reference: d.sale.receiptNo,
          note: input.reason?.trim() || null,
          now,
        });
      }
    }

    // loyalty: claw back a proportional share of the points earned on this sale
    const alreadyReversed = d.refunds.reduce((s, r) => s + r.pointsReversed, 0);
    const reverse = Math.min(
      pointsToReverse(d.sale.pointsEarned, total, d.sale.total),
      d.sale.pointsEarned - alreadyReversed,
    );
    if (d.customer && reverse > 0) {
      tx.update(customers)
        .set({ loyaltyPoints: Math.max(0, d.customer.loyaltyPoints - reverse) })
        .where(eq(customers.id, d.customer.id))
        .run();
    }
    tx.update(refunds).set({ pointsReversed: Math.max(0, reverse) }).where(eq(refunds.id, refund.id)).run();

    const fullyRefunded = d.items.every((item) => {
      const p = pending.find((x) => x.item.id === item.id);
      return item.refundedQty + (p?.qty ?? 0) === item.qty;
    });
    tx.update(sales)
      .set({
        refundedTotal: d.sale.refundedTotal + total,
        status: fullyRefunded ? 'refunded' : 'partially_refunded',
      })
      .where(eq(sales.id, d.sale.id))
      .run();

    return { refundId: refund.id, amount: total, pointsReversed: Math.max(0, reverse) };
  });
}
