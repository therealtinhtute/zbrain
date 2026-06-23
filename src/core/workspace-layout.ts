// Single source of truth for workspace layout constants.
// Phase 01 of V2 — `wiki/` subtree quarantines `evidence/` from retrieval.

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { ensureDir, writeTextFile } from "./fs";

export const WikiTiers = ["axioms", "mental-models", "projects", "decisions"] as const;
export type WikiTier = (typeof WikiTiers)[number];

export const WORKSPACE_LAYOUT_VERSION = 2;

const LAYOUT_MARKER_FILENAME = ".zbrain-layout-version";

export function isWikiTier(value: string): value is WikiTier {
  return (WikiTiers as readonly string[]).includes(value);
}

export function workspaceLayoutMarkerPath(workspaceRoot: string): string {
  return join(workspaceRoot, LAYOUT_MARKER_FILENAME);
}

export function getWorkspaceLayoutVersion(workspaceRoot: string): number {
  const marker = workspaceLayoutMarkerPath(workspaceRoot);
  if (!existsSync(marker)) return 1;
  const raw = readFileSync(marker, "utf8").trim();
  const v = Number.parseInt(raw, 10);
  return Number.isFinite(v) && v > 0 ? v : 1;
}

export function setWorkspaceLayoutVersion(workspaceRoot: string, version: number): void {
  const marker = workspaceLayoutMarkerPath(workspaceRoot);
  ensureDir(join(marker, ".."));
  writeTextFile(marker, `${version}\n`, { overwrite: true });
}
