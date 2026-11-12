import { asc, eq, like, or } from 'drizzle-orm';
import { customers } from '@/db/schema';
import type { Customer } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { PosError } from './errors';

export function createCustomer(db: PosDb, input: { name: string; phone?: string | null }, now = Date.now()): Customer {
  const name = input.name.trim();
  const phone = input.phone?.trim() || null;
  if (!name) throw new PosError('Customer name is required');
  if (phone && db.select().from(customers).where(eq(customers.phone, phone)).get()) {
    throw new PosError('A customer with that phone number already exists');
  }
  return db.insert(customers).values({ name, phone, loyaltyPoints: 0, createdAt: now }).returning().get();
}

export function listCustomers(db: PosDb, query?: string): Customer[] {
  const q = query?.trim();
  return db
    .select()
    .from(customers)
    .where(q ? or(like(customers.name, `%${q}%`), like(customers.phone, `%${q}%`)) : undefined)
    .orderBy(asc(customers.name))
    .all();
}

export function getCustomer(db: PosDb, id: number): Customer | undefined {
  return db.select().from(customers).where(eq(customers.id, id)).get();
}
