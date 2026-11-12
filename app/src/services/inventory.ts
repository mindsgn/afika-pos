import { and, desc, eq, getTableColumns, gt, lte } from 'drizzle-orm';
import { products, stockMovements } from '@/db/schema';
import type { Product } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { PosError } from './errors';

export type MovementType = 'initial' | 'sale' | 'refund' | 'void' | 'restock' | 'adjustment';

/** Low-level: change stock by a signed quantity and write a movement row. Call inside a transaction. */
export function recordMovement(
  db: PosDb,
  a: {
    productId: number;
    type: MovementType;
    quantity: number;
    unitCost?: number | null;
    reference?: string | null;
    note?: string | null;
    now?: number;
  },
): number {
  const product = db.select().from(products).where(eq(products.id, a.productId)).get();
  if (!product) throw new PosError('Product not found');
  const balance = product.stock + a.quantity;
  if (balance < 0) throw new PosError(`Not enough stock for ${product.name}`);
  db.update(products).set({ stock: balance }).where(eq(products.id, a.productId)).run();
  db.insert(stockMovements)
    .values({
      productId: a.productId,
      type: a.type,
      quantity: a.quantity,
      balanceAfter: balance,
      unitCost: a.unitCost ?? null,
      reference: a.reference ?? null,
      note: a.note ?? null,
      createdAt: a.now ?? Date.now(),
    })
    .run();
  return balance;
}

/**
 * Manual stock change. A restock may carry the new unit cost, which updates the product's cost price
 * (and is recorded on the movement for cost-price history).
 */
export function adjustStock(
  db: PosDb,
  args: {
    productId: number;
    delta: number;
    kind: 'restock' | 'adjustment';
    note?: string;
    unitCost?: number | null;
    now?: number;
  },
): number {
  const { productId, delta, kind } = args;
  if (!Number.isInteger(delta) || delta === 0) throw new PosError('Enter a whole number other than zero');
  if (kind === 'restock' && delta < 0) throw new PosError('A restock must be a positive quantity');
  if (args.unitCost != null && args.unitCost < 0) throw new PosError('Cost cannot be negative');
  return db.transaction((tx) => {
    const balance = recordMovement(tx, {
      productId,
      type: kind,
      quantity: delta,
      unitCost: kind === 'restock' ? args.unitCost ?? null : null,
      note: args.note ?? null,
      now: args.now,
    });
    if (kind === 'restock' && args.unitCost != null) {
      tx.update(products).set({ costPrice: args.unitCost }).where(eq(products.id, productId)).run();
    }
    return balance;
  });
}

export type MovementRow = typeof stockMovements.$inferSelect & { productName: string; sku: string };

export function listMovements(db: PosDb, opts: { productId?: number; limit?: number } = {}): MovementRow[] {
  return db
    .select({ ...getTableColumns(stockMovements), productName: products.name, sku: products.sku })
    .from(stockMovements)
    .innerJoin(products, eq(stockMovements.productId, products.id))
    .where(opts.productId ? eq(stockMovements.productId, opts.productId) : undefined)
    .orderBy(desc(stockMovements.id))
    .limit(opts.limit ?? 200)
    .all();
}

export function lowStockProducts(db: PosDb): Product[] {
  return db
    .select()
    .from(products)
    .where(and(eq(products.active, true), gt(products.reorderLevel, 0), lte(products.stock, products.reorderLevel)))
    .all();
}

/** Cost price history for a product: every movement that recorded a unit cost. */
export function costHistory(db: PosDb, productId: number): MovementRow[] {
  return listMovements(db, { productId, limit: 500 }).filter((m) => m.unitCost != null);
}

export function inventoryValuation(db: PosDb): { units: number; costValue: number; retailValue: number } {
  const rows = db.select().from(products).where(eq(products.active, true)).all();
  return rows.reduce(
    (acc, p) => ({
      units: acc.units + p.stock,
      costValue: acc.costValue + p.stock * p.costPrice,
      retailValue: acc.retailValue + p.stock * p.price,
    }),
    { units: 0, costValue: 0, retailValue: 0 },
  );
}
