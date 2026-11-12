import { createCategory, createProduct, listCategories } from '../services/catalog';
import { createCustomer } from '../services/customers';
import { products, customers } from './schema';
import { eq } from 'drizzle-orm';
import type { PosDb } from './types';

const rands = (r: number) => Math.round(r * 100);

/** Optional demo data for a manual/demo shop. Not called at start-up: a real
 *  merchant onboards into an empty catalogue, and the Maestro flows create
 *  their own product. Invoke seedIfEmpty(getDb()) from a dev menu if wanted. */
export function seedIfEmpty(db: PosDb): void {
  if (db.select().from(products).limit(1).all().length > 0) return;
  if (listCategories(db).length === 0) {
    createCategory(db, 'Drinks');
    createCategory(db, 'Food');
    createCategory(db, 'Merch');
  }
  const cat = (name: string) => listCategories(db).find((c) => c.name === name)!.id;
  const demo = [
    { sku: 'DRK-001', name: 'Flat White', category: 'Drinks', price: 38, cost: 12, stock: 50, reorder: 10 },
    { sku: 'DRK-002', name: 'Cappuccino', category: 'Drinks', price: 36, cost: 11, stock: 50, reorder: 10 },
    { sku: 'DRK-003', name: 'Cold Brew', category: 'Drinks', price: 42, cost: 15, stock: 30, reorder: 8 },
    { sku: 'FOD-001', name: 'Croissant', category: 'Food', price: 32, cost: 14, stock: 24, reorder: 6 },
    { sku: 'FOD-002', name: 'Cheese Toastie', category: 'Food', price: 55, cost: 24, stock: 20, reorder: 5 },
    { sku: 'MRC-001', name: 'Branded Mug', category: 'Merch', price: 120, cost: 45, stock: 15, reorder: 4 },
    { sku: 'MRC-002', name: 'Tote Bag', category: 'Merch', price: 95, cost: 38, stock: 3, reorder: 5 },
  ];
  for (const d of demo) {
    createProduct(db, {
      sku: d.sku,
      name: d.name,
      categoryId: cat(d.category),
      price: rands(d.price),
      costPrice: rands(d.cost),
      stock: d.stock,
      reorderLevel: d.reorder,
    });
  }
  if (db.select().from(customers).limit(1).all().length === 0) {
    const c = createCustomer(db, { name: 'Thandi Mokoena', phone: '0821234567' });
    db.update(customers).set({ loyaltyPoints: 120 }).where(eq(customers.id, c.id)).run();
  }
}
