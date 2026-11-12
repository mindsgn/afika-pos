import { eq, inArray, max } from 'drizzle-orm';
import { customers, products, saleItems, sales } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { Discount, NO_DISCOUNT, allocate, discountAmount } from '@/domain/discount';
import { POINT_VALUE_CENTS, pointsEarned } from '@/domain/loyalty';
import { PaymentOption, getPaymentOption, toAssetAmount } from '@/domain/payment';
import { PosError } from './errors';
import { recordMovement } from './inventory';

export interface CheckoutInput {
  items: { productId: number; qty: number }[];
  discount?: Discount;
  customerId?: number | null;
  redeemPoints?: number;
  /** Payment option id, e.g. `usdc-base` (see domain/payment). */
  method: string;
  now?: number;
}

export interface PendingSaleResult {
  saleId: number;
  receiptNo: string;
  total: number;
  subtotal: number;
  discountAmount: number;
  pointsDiscount: number;
  pointsEarned: number;
}

export interface CompleteSaleInput {
  txHash?: string | null;
  cryptoAmount?: number;
  cryptoRate?: number;
  now?: number;
}

export const formatReceiptNo = (n: number) => `R-${String(n).padStart(6, '0')}`;

interface PricedLine {
  productId: number;
  sku: string;
  name: string;
  qty: number;
  unitPrice: number;
  unitCost: number;
  lineDiscount: number;
}

interface PricedCart {
  lines: PricedLine[];
  subtotal: number;
  costTotal: number;
  manualDiscount: number;
  pointsDiscount: number;
  total: number;
  customerId: number | null;
  customerPoints: number;
  redeemPoints: number;
  discount: Discount;
}

/** Prices a cart and validates stock, discounts and loyalty redemption. */
function priceCart(db: PosDb, input: CheckoutInput): PricedCart {
  const discount = input.discount ?? NO_DISCOUNT;
  const redeem = input.redeemPoints ?? 0;

  if (!input.items.length) throw new PosError('The cart is empty');
  if (!Number.isInteger(redeem) || redeem < 0) throw new PosError('Invalid points amount');

  // merge duplicate product lines
  const qtyById = new Map<number, number>();
  for (const it of input.items) {
    if (!Number.isInteger(it.qty) || it.qty <= 0) throw new PosError('Quantities must be whole numbers above zero');
    qtyById.set(it.productId, (qtyById.get(it.productId) ?? 0) + it.qty);
  }

  const ids = [...qtyById.keys()];
  const rows = db.select().from(products).where(inArray(products.id, ids)).all();
  if (rows.length !== ids.length) throw new PosError('A product in the cart no longer exists');

  const priced = rows.map((p) => {
    const qty = qtyById.get(p.id)!;
    if (!p.active) throw new PosError(`${p.name} is no longer for sale`);
    if (qty > p.stock) throw new PosError(`Only ${p.stock} of ${p.name} in stock`);
    return { product: p, qty };
  });

  const subtotal = priced.reduce((s, l) => s + l.product.price * l.qty, 0);
  const costTotal = priced.reduce((s, l) => s + l.product.costPrice * l.qty, 0);
  const manualDiscount = discountAmount(subtotal, discount);

  let customer: ReturnType<typeof getCustomerRow>;
  if (input.customerId) {
    customer = getCustomerRow(db, input.customerId);
    if (!customer) throw new PosError('Customer not found');
  }
  if (redeem > 0) {
    if (!customer) throw new PosError('Select a customer to redeem points');
    if (redeem > customer.loyaltyPoints) throw new PosError('Customer does not have enough points');
    if (redeem * POINT_VALUE_CENTS > subtotal - manualDiscount) {
      throw new PosError('Points value is more than the amount due');
    }
  }
  const pointsDiscount = redeem * POINT_VALUE_CENTS;
  const total = subtotal - manualDiscount - pointsDiscount;

  const discountByLine = allocate(
    manualDiscount + pointsDiscount,
    priced.map((l) => l.product.price * l.qty),
  );

  return {
    lines: priced.map((l, i) => ({
      productId: l.product.id,
      sku: l.product.sku,
      name: l.product.name,
      qty: l.qty,
      unitPrice: l.product.price,
      unitCost: l.product.costPrice,
      lineDiscount: discountByLine[i],
    })),
    subtotal,
    costTotal,
    manualDiscount,
    pointsDiscount,
    total,
    customerId: customer?.id ?? null,
    customerPoints: customer?.loyaltyPoints ?? 0,
    redeemPoints: redeem,
    discount,
  };
}

function insertSale(db: PosDb, cart: PricedCart, status: string, paymentStatus: string, method: string, now: number, failureReason?: string | null) {
  const last = db.select({ m: max(sales.id) }).from(sales).get();
  const receiptNo = formatReceiptNo((last?.m ?? 0) + 1);

  const sale = db
    .insert(sales)
    .values({
      receiptNo,
      createdAt: now,
      status,
      customerId: cart.customerId,
      subtotal: cart.subtotal,
      discountType: cart.discount.type,
      discountValue: cart.discount.type === 'none' ? 0 : cart.discount.value,
      discountAmount: cart.manualDiscount,
      pointsRedeemed: cart.redeemPoints,
      pointsDiscount: cart.pointsDiscount,
      total: cart.total,
      costTotal: cart.costTotal,
      paymentMethod: method,
      paymentStatus,
      cryptoAmount: 0,
      cryptoRate: 0,
      txHash: null,
      failureReason: failureReason ?? null,
      pointsEarned: status === 'completed' ? pointsEarned(cart.total) : 0,
      refundedTotal: 0,
    })
    .returning()
    .get();

  for (const line of cart.lines) {
    db.insert(saleItems)
      .values({
        saleId: sale.id,
        productId: line.productId,
        sku: line.sku,
        name: line.name,
        qty: line.qty,
        unitPrice: line.unitPrice,
        unitCost: line.unitCost,
        lineDiscount: line.lineDiscount,
      })
      .run();
  }

  return { sale, receiptNo };
}

/**
 * Records a sale that still needs to be paid. Stock and loyalty points are not
 * touched until `completeSale` runs, so an abandoned payment never changes the
 * shop's inventory.
 */
export function createPendingSale(db: PosDb, input: CheckoutInput): PendingSaleResult {
  const now = input.now ?? Date.now();
  return db.transaction((tx) => {
    const cart = priceCart(tx, input);
    const { sale, receiptNo } = insertSale(tx, cart, 'pending', 'pending', input.method, now);
    return {
      saleId: sale.id,
      receiptNo,
      total: cart.total,
      subtotal: cart.subtotal,
      discountAmount: cart.manualDiscount,
      pointsDiscount: cart.pointsDiscount,
      pointsEarned: pointsEarned(cart.total),
    };
  });
}

/** Marks a pending sale paid: deducts stock and updates loyalty once. */
export function completeSale(db: PosDb, saleId: number, input: CompleteSaleInput = {}): void {
  const now = input.now ?? Date.now();
  db.transaction((tx) => {
    const sale = tx.select().from(sales).where(eq(sales.id, saleId)).get();
    if (!sale) throw new PosError('Sale not found');
    if (sale.status === 'completed') return;
    if (sale.status !== 'pending') throw new PosError(`Sale is ${sale.status} and cannot be completed`);

    const items = tx.select().from(saleItems).where(eq(saleItems.saleId, saleId)).all();
    const earned = pointsEarned(sale.total);

    tx.update(sales)
      .set({
        status: 'completed',
        paymentStatus: 'success',
        txHash: input.txHash ?? null,
        cryptoAmount: input.cryptoAmount ?? 0,
        cryptoRate: input.cryptoRate ?? 0,
        pointsEarned: earned,
        failureReason: null,
      })
      .where(eq(sales.id, saleId))
      .run();

    for (const item of items) {
      recordMovement(tx, {
        productId: item.productId,
        type: 'sale',
        quantity: -item.qty,
        unitCost: item.unitCost,
        reference: sale.receiptNo,
        now,
      });
    }

    if (sale.customerId) {
      const customer = getCustomerRow(tx, sale.customerId);
      if (customer) {
        tx.update(customers)
          .set({ loyaltyPoints: customer.loyaltyPoints - sale.pointsRedeemed + earned })
          .where(eq(customers.id, customer.id))
          .run();
      }
    }
  });
}

/** Marks a pending sale failed (cancelled, expired or underpaid). */
export function failSale(db: PosDb, saleId: number, reason: string): void {
  db.transaction((tx) => {
    const sale = tx.select().from(sales).where(eq(sales.id, saleId)).get();
    if (!sale) throw new PosError('Sale not found');
    if (sale.status !== 'pending') return;
    tx.update(sales)
      .set({ status: 'failed', paymentStatus: 'failed', failureReason: reason })
      .where(eq(sales.id, saleId))
      .run();
  });
}

/**
 * Prices the cart, records the sale and immediately completes it. Used for the
 * in-app "mark paid" shortcut and the device-local demo mode.
 */
export function checkout(db: PosDb, input: CheckoutInput & { txHash?: string | null; now?: number }) {
  const now = input.now ?? Date.now();
  return db.transaction((tx) => {
    const cart = priceCart(tx, input);
    const { sale, receiptNo } = insertSale(tx, cart, 'completed', 'success', input.method, now);
    const option: PaymentOption = getPaymentOption(input.method);
    const cryptoAmount = toAssetAmount(cart.total, option.id);

    tx.update(sales)
      .set({ cryptoAmount, cryptoRate: 0, txHash: input.txHash ?? null })
      .where(eq(sales.id, sale.id))
      .run();

    const items = tx.select().from(saleItems).where(eq(saleItems.saleId, sale.id)).all();
    for (const item of items) {
      recordMovement(tx, {
        productId: item.productId,
        type: 'sale',
        quantity: -item.qty,
        unitCost: item.unitCost,
        reference: receiptNo,
        now,
      });
    }

    if (sale.customerId) {
      const customer = getCustomerRow(tx, sale.customerId);
      if (customer) {
        const earned = pointsEarned(sale.total);
        tx.update(customers)
          .set({ loyaltyPoints: customer.loyaltyPoints - cart.redeemPoints + earned })
          .where(eq(customers.id, customer.id))
          .run();
      }
    }

    return { saleId: sale.id, receiptNo, status: 'completed' as const, total: sale.total };
  });
}

function getCustomerRow(db: PosDb, id: number) {
  return db.select().from(customers).where(eq(customers.id, id)).get();
}
