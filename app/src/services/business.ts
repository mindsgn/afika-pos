import { eq } from 'drizzle-orm';
import { business } from '@/db/schema';
import type { Business } from '@/db/schema';
import type { PosDb } from '@/db/types';
import { PosError } from './errors';

export const BUSINESS_ID = 1;

export interface BusinessInput {
  name: string;
  website?: string;
  address?: string;
  walletAddress?: string;
  logoUri?: string | null;
}

/** Returns the merchant profile, or null before onboarding has run. */
export function getBusiness(db: PosDb): Business | null {
  return db.select().from(business).where(eq(business.id, BUSINESS_ID)).get() ?? null;
}

export function hasBusiness(db: PosDb): boolean {
  return getBusiness(db) !== null;
}

function normalizeUrl(raw: string): string {
  const value = raw.trim();
  if (!value) return '';
  if (!/^https?:\/\//i.test(value)) return `https://${value}`;
  return value;
}

/** Creates or updates the single business row. */
export function saveBusiness(db: PosDb, input: BusinessInput, now = Date.now()): Business {
  const name = input.name.trim();
  if (name.length < 2) throw new PosError('Enter your business name');
  const website = normalizeUrl(input.website ?? '');
  if (website && !/^https?:\/\/[^\s.]+\.[^\s]{2,}$/i.test(website)) {
    throw new PosError('Enter a valid website, e.g. afika.app');
  }
  const address = (input.address ?? '').trim();
  if (address.length < 4) throw new PosError('Enter your business address');
  const walletAddress = (input.walletAddress ?? '').trim().toLowerCase();

  const existing = getBusiness(db);
  const values = {
    name,
    website,
    address,
    walletAddress,
    logoUri: input.logoUri ?? existing?.logoUri ?? null,
    createdAt: existing?.createdAt ?? now,
    updatedAt: now,
  };

  db.insert(business)
    .values({ id: BUSINESS_ID, ...values })
    .onConflictDoUpdate({ target: business.id, set: values })
    .run();

  return getBusiness(db)!;
}

/** Merchant display details for receipts. */
export function merchantName(db: PosDb): string {
  return getBusiness(db)?.name || 'CRYPTO POS';
}

export function merchantWebsite(db: PosDb): string {
  return getBusiness(db)?.website || '';
}

export function merchantAddress(db: PosDb): string {
  return getBusiness(db)?.address || '';
}
