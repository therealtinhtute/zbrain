import { Database } from "bun:sqlite";
import { join } from "node:path";
import { ensureDir } from "./fs";
import { applyPendingMigrations } from "../db/migrator";

// Embedded fallback schema (V2) — used when migration files are not present
// (e.g. in a published binary where the src/ tree is not shipped).
// The migrator below also reads from src/db/migrations/ at dev-time.
const EMBEDDED_SCHEMA_V2 = `
  CREATE TABLE IF NOT EXISTS projects (...);
`;

export const SCHEMA_VERSION = 2;

export function openDb(runtimeDir: string): Database {
  ensureDir(runtimeDir);
  const dbPath = join(runtimeDir, "zbrain.db");
  const db = new Database(dbPath, { create: true });
  db.exec("PRAGMA journal_mode=WAL");
  return db;
}

export function initDb(runtimeDir: string, migrationsDir?: string): Database {
  const db = openDb(runtimeDir);
  if (migrationsDir) {
    applyPendingMigrations(db, migrationsDir);
  } else {
    // Legacy bootstrap (V2 baseline inline).
    bootstrapV2Schema(db);
  }
  return db;
}

function bootstrapV2Schema(db: Database): void {
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

    CREATE TABLE IF NOT EXISTS notes (
      id            TEXT NOT NULL,
      workspace     TEXT NOT NULL,
      path          TEXT NOT NULL,
      tier          TEXT NOT NULL,
      status        TEXT NOT NULL,
      title         TEXT,
      content_sha   TEXT NOT NULL,
      created_at    TEXT NOT NULL,
      updated_at    TEXT NOT NULL,
      review_by     TEXT,
      PRIMARY KEY (id, workspace),
      UNIQUE (workspace, path)
    );

    CREATE INDEX IF NOT EXISTS idx_notes_ws_status ON notes(workspace, status);

    CREATE VIRTUAL TABLE IF NOT EXISTS note_fts USING fts5(
      title, body
    );

    CREATE TABLE IF NOT EXISTS note_fts_map (
      rowid    INTEGER PRIMARY KEY,
      note_id  TEXT NOT NULL,
      workspace TEXT NOT NULL,
      UNIQUE (workspace, note_id)
    );

    CREATE TABLE IF NOT EXISTS links (
      from_id   TEXT NOT NULL,
      workspace TEXT NOT NULL,
      type      TEXT NOT NULL,
      to_id     TEXT NOT NULL,
      PRIMARY KEY (from_id, workspace, type, to_id)
    );

    CREATE TABLE IF NOT EXISTS leases (
      workspace   TEXT NOT NULL,
      path        TEXT NOT NULL,
      holder      TEXT NOT NULL,
      acquired_at TEXT NOT NULL,
      expires_at  TEXT NOT NULL,
      PRIMARY KEY (workspace, path)
    );

    CREATE INDEX IF NOT EXISTS idx_leases_expires ON leases(expires_at);

    CREATE INDEX IF NOT EXISTS idx_evidence_ws ON evidence_sources(workspace, ingested_at);

    DROP TABLE IF EXISTS queries;
  `);

  if (currentVersion < SCHEMA_VERSION) {
    db.exec(`PRAGMA user_version = ${SCHEMA_VERSION}`);
  }
}
