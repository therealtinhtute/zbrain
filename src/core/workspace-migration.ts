// V1 → V2 workspace layout migration.
// V1: <workspaceRoot>/{axioms,mental-models,projects,decisions}/*.md
// V2: <workspaceRoot>/wiki/{axioms,mental-models,projects,decisions}/*.md
//
// Idempotent. Safe to call on every CLI boot. Failure of one workspace
// does not block others (caller may wrap in try/catch).

import { existsSync, readdirSync, renameSync } from "node:fs";
import { join } from "node:path";
import { ensureDir } from "./fs";
import {
  WikiTiers,
  WORKSPACE_LAYOUT_VERSION,
  getWorkspaceLayoutVersion,
  setWorkspaceLayoutVersion,
} from "./workspace-layout";

export interface MigrationResult {
  fromVersion: number;
  toVersion: number;
  movedFiles: string[];
  createdDirs: string[];
  skipped: boolean;
}

export interface MigrationSummary {
  results: Array<{ workspace: string; result: MigrationResult }>;
  anyMigrated: boolean;
}

export function migrateV1ToV2(workspaceRoot: string): MigrationResult {
  const fromVersion = getWorkspaceLayoutVersion(workspaceRoot);
  if (fromVersion >= WORKSPACE_LAYOUT_VERSION) {
    return { fromVersion, toVersion: fromVersion, movedFiles: [], createdDirs: [], skipped: true };
  }

  const movedFiles: string[] = [];
  const createdDirs: string[] = [];

  for (const tier of WikiTiers) {
    const oldDir = join(workspaceRoot, tier);
    const newDir = join(workspaceRoot, "wiki", tier);

    ensureDir(newDir);
    createdDirs.push(newDir);

    if (!existsSync(oldDir)) continue;

    for (const entry of readdirSync(oldDir, { withFileTypes: true })) {
      if (!entry.isFile()) continue;
      const oldPath = join(oldDir, entry.name);
      const newPath = join(newDir, entry.name);
      if (existsSync(newPath)) {
        // Destination already exists (partial migration). Skip and log via caller.
        continue;
      }
      renameSync(oldPath, newPath);
      movedFiles.push(`${oldPath} -> ${newPath}`);
    }
  }

  setWorkspaceLayoutVersion(workspaceRoot, WORKSPACE_LAYOUT_VERSION);

  return {
    fromVersion,
    toVersion: WORKSPACE_LAYOUT_VERSION,
    movedFiles,
    createdDirs,
    skipped: false,
  };
}

export function migrateAllWorkspaces(workspacesDir: string): MigrationSummary {
  if (!existsSync(workspacesDir)) {
    return { results: [], anyMigrated: false };
  }

  const results: Array<{ workspace: string; result: MigrationResult }> = [];
  let anyMigrated = false;

  for (const entry of readdirSync(workspacesDir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const workspaceRoot = join(workspacesDir, entry.name);
    const result = migrateV1ToV2(workspaceRoot);
    results.push({ workspace: entry.name, result });
    if (!result.skipped) anyMigrated = true;
  }

  return { results, anyMigrated };
}
