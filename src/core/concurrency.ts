// Advisory write leases (V2 multi-agent safety).
// Per-(workspace, path) lock with TTL auto-expiry. Backed by SQLite.

import type { Database } from "bun:sqlite";

export interface Lease {
  workspace: string;
  path: string;
  holder: string;
  acquiredAt: string;
  expiresAt: string;
}

export interface AcquireOptions {
  workspace: string;
  path: string;
  holder: string;
  ttlMs?: number;
  nowIso?: string;
}

const DEFAULT_TTL_MS = 60_000;

export function acquireLease(db: Database, options: AcquireOptions): Lease {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const ttl = options.ttlMs ?? DEFAULT_TTL_MS;
  const expiresAt = new Date(Date.parse(nowIso) + ttl).toISOString();
  db.prepare(`
    INSERT INTO leases (workspace, path, holder, acquired_at, expires_at)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT (workspace, path) DO UPDATE SET
      holder = excluded.holder,
      acquired_at = excluded.acquired_at,
      expires_at = excluded.expires_at
  `).run(options.workspace, options.path, options.holder, nowIso, expiresAt);
  return {
    workspace: options.workspace,
    path: options.path,
    holder: options.holder,
    acquiredAt: nowIso,
    expiresAt,
  };
}

export function releaseLease(db: Database, workspace: string, path: string, holder: string): boolean {
  const result = db.prepare(`
    DELETE FROM leases WHERE workspace = ? AND path = ? AND holder = ?
  `).run(workspace, path, holder);
  return result.changes > 0;
}

export function getLease(db: Database, workspace: string, path: string, nowIso?: string): Lease | null {
  const row = db.prepare(`
    SELECT * FROM leases WHERE workspace = ? AND path = ?
  `).get(workspace, path) as
    | { workspace: string; path: string; holder: string; acquired_at: string; expires_at: string }
    | null;
  if (!row) return null;
  const now = nowIso ?? new Date().toISOString();
  if (Date.parse(row.expires_at) <= Date.parse(now)) {
    // Expired — auto-clean.
    db.prepare(`DELETE FROM leases WHERE workspace = ? AND path = ?`).run(workspace, path);
    return null;
  }
  return {
    workspace: row.workspace,
    path: row.path,
    holder: row.holder,
    acquiredAt: row.acquired_at,
    expiresAt: row.expires_at,
  };
}

export function listLeases(db: Database, workspace?: string): Lease[] {
  const sql = workspace
    ? `SELECT * FROM leases WHERE workspace = ?`
    : `SELECT * FROM leases`;
  const rows = (workspace ? db.prepare(sql).all(workspace) : db.prepare(sql).all()) as Array<{
    workspace: string;
    path: string;
    holder: string;
    acquired_at: string;
    expires_at: string;
  }>;
  const now = new Date().toISOString();
  return rows
    .filter((r) => Date.parse(r.expires_at) > Date.parse(now))
    .map((r) => ({
      workspace: r.workspace,
      path: r.path,
      holder: r.holder,
      acquiredAt: r.acquired_at,
      expiresAt: r.expires_at,
    }));
}

export function expireLeases(db: Database, nowIso?: string): number {
  const now = nowIso ?? new Date().toISOString();
  const result = db.prepare(`DELETE FROM leases WHERE expires_at <= ?`).run(now);
  return result.changes;
}
