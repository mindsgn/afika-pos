import type { BaseSQLiteDatabase } from 'drizzle-orm/sqlite-core';
import type * as schema from './schema';

/**
 * Services accept this type so the exact same code runs on expo-sqlite (app)
 * and better-sqlite3 (Jest). Transactions (`tx`) are assignable to it too.
 */
export type PosDb = BaseSQLiteDatabase<'sync', any, typeof schema>;
