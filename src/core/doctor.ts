// `zbrain doctor` — reconciliation + health command.
// Per SPEC §5 AC-P1-6 / Phase 05: detects DB↔files drift, orphaned evidence,
// stale review_by, broken supersede links, expired leases, idle sessions,
// schema version, FTS5 vs notes row count drift.

import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import type { Database } from "bun:sqlite";
import { SCHEMA_VERSION } from "./db";
import { WikiTiers } from "./workspace-layout";
import { wikiRoot, type RuntimePaths } from "./runtime-paths";

export type CheckStatus = "ok" | "warn" | "error";

export interface Finding {
  message: string;
  detail?: string;
}

export interface CheckResult {
  name: string;
  status: CheckStatus;
  findings: Finding[];
}

export interface DoctorReport {
  ok: boolean;
  results: CheckResult[];
}

function empty(name: string): CheckResult {
  return { name, status: "ok", findings: [] };
}

function push(result: CheckResult, status: CheckStatus, message: string, detail?: string): void {
  if (status === "error") result.status = "error";
  else if (status === "warn" && result.status === "ok") result.status = "warn";
  result.findings.push(detail ? { message, detail } : { message });
}

export function checkDbFileConsistency(
  paths: RuntimePaths,
  workspace: string,
  db: Database,
): CheckResult {
  const result = empty("db-file-consistency");
  if (!existsSync(paths.workspacesDir)) return result;
  const dbRows = db.prepare(`SELECT id, path FROM notes WHERE workspace = ?`).all(workspace) as Array<{
    id: string;
    path: string;
  }>;
  const dbByPath = new Map(dbRows.map((r) => [r.path, r.id]));

  // Walk files.
  const filesByPath = new Set<string>();
  const wiki = wikiRoot(paths, workspace);
  if (existsSync(wiki)) {
    for (const tier of WikiTiers) {
      const tierDir = join(wiki, tier);
      if (!existsSync(tierDir)) continue;
      for (const entry of readdirSync(tierDir, { withFileTypes: true })) {
        if (!entry.isFile() || !entry.name.endsWith(".md")) continue;
        filesByPath.add(`${tier}/${entry.name.replace(/\.md$/, ".md")}`);
      }
    }
  }

  for (const [relPath, id] of dbByPath) {
    if (!filesByPath.has(relPath)) {
      push(result, "warn", `DB has note id=${id} at ${relPath} but file is missing`, "run `zbrain reindex` to clean");
    }
  }
  for (const relPath of filesByPath) {
    if (!dbByPath.has(relPath)) {
      push(result, "warn", `File ${relPath} has no DB row`, "run `zbrain reindex` to index");
    }
  }
  return result;
}

export function checkOrphanedEvidence(
  paths: RuntimePaths,
  workspace: string,
  db: Database,
): CheckResult {
  const result = empty("orphaned-evidence");
  const rows = db.prepare(`
    SELECT id, raw_filename FROM evidence_sources WHERE workspace = ?
  `).all(workspace) as Array<{ id: string; raw_filename: string }>;
  for (const row of rows) {
    const rawPath = join(paths.workspacesDir, workspace, "evidence", "sources", row.id, row.raw_filename);
    if (!existsSync(rawPath)) {
      push(result, "error", `Evidence ${row.id} has DB row but no raw file at ${rawPath}`);
    }
  }
  return result;
}

export function checkStaleReview(
  workspace: string,
  db: Database,
  nowIso?: string,
): CheckResult {
  const result = empty("stale-review");
  const now = nowIso ?? new Date().toISOString();
  const rows = db.prepare(`
    SELECT id, path, review_by FROM notes WHERE workspace = ? AND review_by IS NOT NULL AND review_by < ?
  `).all(workspace, now) as Array<{ id: string; path: string; review_by: string }>;
  for (const row of rows) {
    push(result, "warn", `Note ${row.id} (${row.path}) review_by=${row.review_by} is past`, "re-review recommended");
  }
  return result;
}

export function checkBrokenLinks(workspace: string, db: Database): CheckResult {
  const result = empty("broken-links");
  const links = db.prepare(`
    SELECT from_id, to_id, type FROM links WHERE workspace = ?
  `).all(workspace) as Array<{ from_id: string; to_id: string; type: string }>;
  for (const link of links) {
    const target = db.prepare(`SELECT 1 FROM notes WHERE id = ? AND workspace = ?`).get(link.to_id, workspace);
    if (!target) {
      push(result, "warn", `Broken ${link.type} link: ${link.from_id} -> ${link.to_id} (target missing)`);
    }
  }
  return result;
}

export function checkExpiredLeases(db: Database, threshold = 10): CheckResult {
  const result = empty("expired-leases");
  const now = new Date().toISOString();
  const expired = db.prepare(`SELECT COUNT(*) as c FROM leases WHERE expires_at <= ?`).get(now) as { c: number };
  const active = db.prepare(`SELECT COUNT(*) as c FROM leases WHERE expires_at > ?`).get(now) as { c: number };
  if (active.c > threshold) {
    push(result, "warn", `${active.c} active leases (threshold ${threshold})`, "consider running `zbrain lease list`");
  }
  if (expired.c > 0) {
    push(result, "warn", `${expired.c} expired leases in DB (auto-clean on next read)`, "running an `acquire` will clean");
  }
  return result;
}

export function checkIdleSessions(thresholdDays = 30): CheckResult {
  const result = empty("idle-sessions");
  // No sessions table touched in this phase; report none.
  push(result, "ok", "session GC: not yet wired (deferred to follow-up)");
  return result;
}

export function checkSchemaVersion(db: Database): CheckResult {
  const result = empty("schema-version");
  const row = db.prepare(`PRAGMA user_version`).get() as { user_version: number };
  if (row.user_version !== SCHEMA_VERSION) {
    push(result, "warn", `DB schema version ${row.user_version} != expected ${SCHEMA_VERSION}`, "run `zbrain reindex` to migrate");
  }
  return result;
}

export function checkFtsSync(workspace: string, db: Database): CheckResult {
  const result = empty("fts5-sync");
  const notesCount = db.prepare(`SELECT COUNT(*) as c FROM notes WHERE workspace = ?`).get(workspace) as { c: number };
  const mapCount = db.prepare(`
    SELECT COUNT(*) as c FROM note_fts_map m
    JOIN notes n ON n.id = m.note_id AND n.workspace = m.workspace
    WHERE n.workspace = ?
  `).get(workspace) as { c: number };
  if (notesCount.c !== mapCount.c) {
    push(result, "warn", `FTS5 index out of sync: ${notesCount.c} notes vs ${mapCount.c} map rows`, "run `zbrain reindex`");
  }
  return result;
}

export function runDoctor(
  paths: RuntimePaths,
  workspace: string,
  db: Database,
): DoctorReport {
  const results: CheckResult[] = [
    checkSchemaVersion(db),
    checkDbFileConsistency(paths, workspace, db),
    checkOrphanedEvidence(paths, workspace, db),
    checkStaleReview(workspace, db),
    checkBrokenLinks(workspace, db),
    checkExpiredLeases(db),
    checkIdleSessions(),
    checkFtsSync(workspace, db),
  ];
  return {
    ok: results.every((r) => r.status !== "error"),
    results,
  };
}
