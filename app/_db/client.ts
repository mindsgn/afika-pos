import { drizzle } from "drizzle-orm/expo-sqlite";
import { openDatabaseSync } from "expo-sqlite";
import * as schema from "@/db/schema";

export const DATABASE_NAME = "afika.db";

export const expo_sqlite = openDatabaseSync(DATABASE_NAME);
export const db = drizzle(expo_sqlite, { schema });
