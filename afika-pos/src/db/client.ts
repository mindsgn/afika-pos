import { drizzle } from 'drizzle-orm/expo-sqlite';
import { openDatabaseSync } from 'expo-sqlite';
import { SCHEMA_SQL } from './bootstrap';
import * as schema from './schema';
import type { PosDb } from './types';

let instance: PosDb | null = null;

export const DATABASE_NAME: string = "crypto-pos.db"

/** Opens the local SQLite file, creates tables on first run and seeds demo data. */
export function getDb(): PosDb {
  if (!instance) {
    const sqlite = openDatabaseSync(DATABASE_NAME);
    sqlite.execSync('PRAGMA journal_mode = WAL;');
    sqlite.execSync(SCHEMA_SQL);
    // Additive migrations for databases created by earlier app versions.
    try { sqlite.execSync('ALTER TABLE products ADD COLUMN image_uri TEXT;'); } catch {}
    try { sqlite.execSync('ALTER TABLE business ADD COLUMN logo_uri TEXT;'); } catch {}
    const db = drizzle(sqlite, { schema }) as unknown as PosDb;
    instance = db;
  }
  return instance;
}
