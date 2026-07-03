// V2 schema migration framework.
// Migration files live in src/db/migrations/ as NNN-name.sql.
// Applied in lexical order. schema_meta tracks applied migrations.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import type { Database } from "bun:sqlite";

export interface MigrationResult {
  applied: string[];
  skipped: string[];
}

export function applyPendingMigrations(db: Database, migrationsDir: string): MigrationResult {
  if (!existsSync(migrationsDir)) {
    return { applied: [], skipped: [] };
  }

  // Ensure schema_meta exists.
  db.exec(`
    CREATE TABLE IF NOT EXISTS schema_meta (
      key    TEXT PRIMARY KEY,
      value  TEXT NOT NULL
    );
  `);

  // Discover migrations.
  const files = readdirSync(migrationsDir)
    .filter((f) => f.endsWith(".sql"))
    .sort();

  // Track which have been applied.
  const applied = new Set(
    (db.prepare(`SELECT value FROM schema_meta WHERE key = 'applied_migrations'`).get() as
      | { value: string }
      | null)?.value.split(",").filter(Boolean) ?? [],
  );

  const result: MigrationResult = { applied: [], skipped: [] };

  for (const file of files) {
    if (applied.has(file)) {
      result.skipped.push(file);
      continue;
    }
    const sql = readFileSync(join(migrationsDir, file), "utf8");
    db.exec(sql);
    applied.add(file);
    db.prepare(`
      INSERT OR REPLACE INTO schema_meta (key, value) VALUES ('applied_migrations', ?)
    `).run([...applied].join(","));
    result.applied.push(file);
  }

  // Bump user_version to the number of applied migrations.
  db.exec(`PRAGMA user_version = ${applied.size}`);

  return result;
}
