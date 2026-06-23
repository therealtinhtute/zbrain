// Per-session context file management (V2 multi-agent safety).
// Replaces V1's shared current-task.md (which clobbered between agents).
// Each agent/CLI invocation gets its own session id; per-session context
// file lives under projects/<hash>/sessions/<sid>.md.

import { existsSync, mkdirSync, renameSync, writeFileSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";
import { createHash } from "node:crypto";
import type { RuntimePaths } from "./runtime-paths";

const DEFAULT_SESSION_ENV = "ZBRAIN_SESSION_ID";

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
