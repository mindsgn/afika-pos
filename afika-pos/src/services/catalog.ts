import { and, asc, eq, getTableColumns, like, or } from 'drizzle-orm';
import { categories, products } from '@/db/schema';
import type { Category, Product } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { PosError } from './errors';
import { recordMovement } from './inventory';

// ---------- categories ----------

export function createCategory(db: PosDb, name: string, now = Date.now()): Category {
  const n = name.trim();
  if (!n) throw new PosError('Category name is required');
  if (db.select().from(categories).where(eq(categories.name, n)).get()) {
    throw new PosError('That category already exists');
  }
  return db.insert(categories).values({ name: n, createdAt: now }).returning().get();
}

export function renameCategory(db: PosDb, id: number, name: string): void {
  const n = name.trim();
  if (!n) throw new PosError('Category name is required');
  const clash = db.select().from(categories).where(eq(categories.name, n)).get();
  if (clash && clash.id !== id) throw new PosError('That category already exists');
  db.update(categories).set({ name: n }).where(eq(categories.id, id)).run();
}

export function deleteCategory(db: PosDb, id: number): void {
  const used = db.select().from(products).where(eq(products.categoryId, id)).all();
  if (used.length > 0) throw new PosError(`Move or delete the ${used.length} product(s) in this category first`);
  db.delete(categories).where(eq(categories.id, id)).run();
}

export function listCategories(db: PosDb): Category[] {
  return db.select().from(categories).orderBy(asc(categories.name)).all();
}

// ---------- products ----------

export interface ProductInput {
  sku: string;
  name: string;
  categoryId: number | null;
  price: number;
  costPrice: number;
  stock?: number;
  reorderLevel?: number;
  imageUri?: string | null;
}

export type ProductWithCategory = Product & { categoryName: string | null };

function validate(input: Pick<ProductInput, 'name' | 'sku' | 'price' | 'costPrice'>) {
  if (!input.name.trim()) throw new PosError('Product name is required');
  if (!input.sku.trim()) throw new PosError('SKU is required');
  if (!Number.isInteger(input.price) || input.price < 0) throw new PosError('Price must be zero or more');
  if (!Number.isInteger(input.costPrice) || input.costPrice < 0) throw new PosError('Cost price must be zero or more');
}

export function createProduct(db: PosDb, input: ProductInput, now = Date.now()): Product {
  validate(input);
  const sku = input.sku.trim().toUpperCase();
  const stock = input.stock ?? 0;
  if (!Number.isInteger(stock) || stock < 0) throw new PosError('Stock must be zero or more');
  if (db.select().from(products).where(eq(products.sku, sku)).get()) {
    throw new PosError(`SKU ${sku} is already in use`);
  }
  return db.transaction((tx) => {
    const product = tx
      .insert(products)
      .values({
        sku,
        name: input.name.trim(),
        imageUri: input.imageUri ?? null,
        categoryId: input.categoryId,
        price: input.price,
        costPrice: input.costPrice,
        stock: 0,
        reorderLevel: input.reorderLevel ?? 0,
        active: true,
        createdAt: now,
      })
      .returning()
      .get();
    if (stock > 0) {
      recordMovement(tx, {
        productId: product.id,
        type: 'initial',
        quantity: stock,
        unitCost: input.costPrice,
        note: 'Opening stock',
        now,
      });
    }
    return tx.select().from(products).where(eq(products.id, product.id)).get()!;
  });
}

/** Edits product details. Stock is changed only through inventory.adjustStock so history stays complete. */
export function updateProduct(db: PosDb, id: number, input: Omit<ProductInput, 'stock'>): void {
  validate(input);
  const sku = input.sku.trim().toUpperCase();
  const clash = db.select().from(products).where(eq(products.sku, sku)).get();
  if (clash && clash.id !== id) throw new PosError(`SKU ${sku} is already in use`);
  db.update(products)
    .set({
      sku,
      name: input.name.trim(),
      imageUri: input.imageUri ?? null,
      categoryId: input.categoryId,
      price: input.price,
      costPrice: input.costPrice,
      reorderLevel: input.reorderLevel ?? 0,
    })
    .where(eq(products.id, id))
    .run();
}

export function setProductActive(db: PosDb, id: number, active: boolean): void {
  db.update(products).set({ active }).where(eq(products.id, id)).run();
}

export function getProduct(db: PosDb, id: number): ProductWithCategory | undefined {
  return db
    .select({ ...getTableColumns(products), categoryName: categories.name })
    .from(products)
    .leftJoin(categories, eq(products.categoryId, categories.id))
    .where(eq(products.id, id))
    .get();
}

export function listProducts(
  db: PosDb,
  opts: { query?: string; categoryId?: number | null; includeInactive?: boolean } = {},
): ProductWithCategory[] {
  const q = opts.query?.trim();
  const conds = [];
  if (!opts.includeInactive) conds.push(eq(products.active, true));
  if (opts.categoryId) conds.push(eq(products.categoryId, opts.categoryId));
  if (q) conds.push(or(like(products.name, `%${q}%`), like(products.sku, `%${q}%`), like(categories.name, `%${q}%`))!);
  return db
    .select({ ...getTableColumns(products), categoryName: categories.name })
    .from(products)
    .leftJoin(categories, eq(products.categoryId, categories.id))
    .where(conds.length ? and(...conds) : undefined)
    .orderBy(asc(products.name))
    .all();
}
