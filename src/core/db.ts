import { Database } from "bun:sqlite";
import { join } from "node:path";
import { ensureDir } from "./fs";

export const SCHEMA_VERSION = 1;

export function openDb(runtimeDir: string): Database {
  ensureDir(runtimeDir);
  const dbPath = join(runtimeDir, "zbrain.db");
  const db = new Database(dbPath, { create: true });
  db.exec("PRAGMA journal_mode=WAL");
  return db;
}

export function initDb(runtimeDir: string): Database {
  const db = openDb(runtimeDir);
  initSchema(db);
  return db;
}

function initSchema(db: Database): void {
  const row = db.prepare("PRAGMA user_version").get() as { user_version: number };
  const currentVersion = row.user_version;

  if (currentVersion > SCHEMA_VERSION) {
    throw new Error(
      `DB schema version ${currentVersion} is newer than supported ${SCHEMA_VERSION}. Upgrade zbrain.`,
    );
  }

  db.exec(`
    CREATE TABLE IF NOT EXISTS projects (
      project_root          TEXT PRIMARY KEY,
      workspace             TEXT NOT NULL,
      context_file          TEXT NOT NULL,
      runtimes              TEXT NOT NULL DEFAULT '[]',
      secondary_workspaces  TEXT NOT NULL DEFAULT '[]',
      created_at            TEXT NOT NULL,
      updated_at            TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS evidence_sources (
      id                    TEXT NOT NULL,
      workspace             TEXT NOT NULL,
      source_type           TEXT NOT NULL,
      origin                TEXT NOT NULL,
      label                 TEXT NOT NULL,
      workspace_at_ingest   TEXT NOT NULL,
      ingested_at           TEXT NOT NULL,
      state                 TEXT NOT NULL,
      raw_filename          TEXT NOT NULL DEFAULT 'raw.md',
      raw_sha256            TEXT NOT NULL,
      source_sha256         TEXT NOT NULL,
      state_updated_at      TEXT NOT NULL,
      PRIMARY KEY (id, workspace)
    );

    CREATE TABLE IF NOT EXISTS sessions (
      id                TEXT PRIMARY KEY,
      project_root      TEXT NOT NULL,
      workspace         TEXT NOT NULL,
      started_at        TEXT NOT NULL,
      last_activity_at  TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS queries (
      id               INTEGER PRIMARY KEY AUTOINCREMENT,
      session_id       TEXT NOT NULL REFERENCES sessions(id),
      query_text       TEXT NOT NULL,
      workspace        TEXT NOT NULL,
      context_file     TEXT NOT NULL,
      retrieved_count  INTEGER NOT NULL DEFAULT 0,
      queried_at       TEXT NOT NULL
    );
  `);

  if (currentVersion === 0) {
    db.exec(`PRAGMA user_version = ${SCHEMA_VERSION}`);
  }
}
