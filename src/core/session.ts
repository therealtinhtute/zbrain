// Per-session context file management (V2 multi-agent safety).
// Replaces V1's shared current-task.md (which clobbered between agents).
// Each agent/CLI invocation gets its own session id; per-session context
// file lives under projects/<hash>/sessions/<sid>.md.

import { existsSync, mkdirSync, renameSync, writeFileSync, readFileSync, readdirSync, unlinkSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";
import { createHash } from "node:crypto";
import type { Database } from "bun:sqlite";
import type { RuntimePaths } from "./runtime-paths";

const DEFAULT_SESSION_ENV = "ZBRAIN_SESSION_ID";

// AC: idle session GC threshold for `zbrain doctor --fix` (checkIdleSessions / fixIdleSessions).
export const SESSION_IDLE_GC_DAYS = 30;

export interface SessionRow {
  id: string;
  projectRoot: string;
  workspace: string;
  startedAt: string;
  lastActivityAt: string;
}

export function getSessionId(override?: string): string {
  if (override) return override;
  const fromEnv = process.env[DEFAULT_SESSION_ENV];
  if (fromEnv && fromEnv.trim().length > 0) return fromEnv.trim();
  return randomUUID();
}

export function projectHash(projectRoot: string): string {
  return createHash("sha256").update(projectRoot).digest("hex").slice(0, 16);
}

export function sessionContextPath(paths: RuntimePaths, projectRoot: string, sessionId: string): string {
  const hash = projectHash(projectRoot);
  return join(paths.projectsDir, hash, "sessions", `${sessionId}.md`);
}

export function writeSessionContext(
  paths: RuntimePaths,
  projectRoot: string,
  sessionId: string,
  content: string,
): string {
  const target = sessionContextPath(paths, projectRoot, sessionId);
  mkdirSync(dirname(target), { recursive: true });
  const tmp = `${target}.tmp`;
  writeFileSync(tmp, content, "utf8");
  renameSync(tmp, target);
  return target;
}

export function readSessionContext(paths: RuntimePaths, projectRoot: string, sessionId: string): string | null {
  const target = sessionContextPath(paths, projectRoot, sessionId);
  if (!existsSync(target)) return null;
  return readFileSync(target, "utf8");
}

export function listSessionIds(paths: RuntimePaths, projectRoot: string): string[] {
  const dir = join(paths.projectsDir, projectHash(projectRoot), "sessions");
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .filter((f) => f.endsWith(".md"))
    .map((f) => f.replace(/\.md$/, ""))
    .sort();
}

function rowToSession(row: {
  id: string;
  project_root: string;
  workspace: string;
  started_at: string;
  last_activity_at: string;
}): SessionRow {
  return {
    id: row.id,
    projectRoot: row.project_root,
    workspace: row.workspace,
    startedAt: row.started_at,
    lastActivityAt: row.last_activity_at,
  };
}

// Queryable session metadata lives in SQLite; the context body stays file-based
// (see writeSessionContext above). Upsert on every ask/recall so `last_activity_at`
// reflects real usage for the doctor idle-session GC.
export function touchSession(db: Database, s: { id: string; projectRoot: string; workspace: string }): void {
  const now = new Date().toISOString();
  db.prepare(`
    INSERT INTO sessions (id, project_root, workspace, started_at, last_activity_at)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET last_activity_at = excluded.last_activity_at
  `).run(s.id, s.projectRoot, s.workspace, now, now);
}

export function listStaleSessions(db: Database, idleDays: number, workspace?: string): SessionRow[] {
  const cutoff = new Date(Date.now() - idleDays * 24 * 60 * 60 * 1000).toISOString();
  const rows = workspace
    ? (db.prepare(`SELECT * FROM sessions WHERE workspace = ? AND last_activity_at < ?`).all(workspace, cutoff) as any[])
    : (db.prepare(`SELECT * FROM sessions WHERE last_activity_at < ?`).all(cutoff) as any[]);
  return rows.map(rowToSession);
}

export function deleteSession(db: Database, paths: RuntimePaths, id: string, projectRoot: string): void {
  db.prepare(`DELETE FROM sessions WHERE id = ?`).run(id);
  const target = sessionContextPath(paths, projectRoot, id);
  try {
    unlinkSync(target);
  } catch {
    // Context file already gone — nothing to clean up.
  }
}
