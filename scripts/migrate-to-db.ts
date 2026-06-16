/**
 * One-shot migration: reads legacy projects.json + source.yaml files and seeds zbrain.db.
 *
 * Usage:
 *   bun run scripts/migrate-to-db.ts [--home <dir>]
 *
 * Defaults to ~/.zbrain. Safe to run multiple times (INSERT OR IGNORE).
 */

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { homedir } from "node:os";
import YAML from "js-yaml";
import { initDb } from "../src/core/db";

// ── resolve runtime dir ──────────────────────────────────────────────────────

const homeArg = process.argv.indexOf("--home");
const homeDir = homeArg !== -1 ? resolve(process.argv[homeArg + 1]) : homedir();
const runtimeDir = process.env.ZBRAIN_HOME ?? join(homeDir, ".zbrain");

console.log(`Migrating: ${runtimeDir}`);

if (!existsSync(runtimeDir)) {
  console.error(`Runtime directory not found: ${runtimeDir}`);
  console.error("Run `zbrain setup` first.");
  process.exit(1);
}

// ── init DB (creates schema if not present) ──────────────────────────────────

const db = initDb(runtimeDir);
const nowIso = new Date().toISOString();

// ── migrate projects.json ────────────────────────────────────────────────────

const projectsFile = join(runtimeDir, "projects.json");
let migratedProjects = 0;
let skippedProjects = 0;

if (existsSync(projectsFile)) {
  const registry = JSON.parse(readFileSync(projectsFile, "utf8")) as {
    projects: Array<{
      project_root: string;
      workspace: string;
      context_file: string;
      runtimes?: string[];
      secondary_workspaces?: Array<{ workspace: string; keywords: string[]; limit?: number }>;
    }>;
  };

  const upsert = db.prepare(`
    INSERT OR IGNORE INTO projects
      (project_root, workspace, context_file, runtimes, secondary_workspaces, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?)
  `);

  for (const p of registry.projects ?? []) {
    const runtimes = JSON.stringify(p.runtimes ?? []);
    const secondaries = JSON.stringify(p.secondary_workspaces ?? []);
    const result = upsert.run(p.project_root, p.workspace, p.context_file, runtimes, secondaries, nowIso, nowIso);
    if (result.changes > 0) {
      console.log(`  project  +  ${p.project_root}  →  ${p.workspace}`);
      migratedProjects++;
    } else {
      console.log(`  project  ~  ${p.project_root}  (already exists, skipped)`);
      skippedProjects++;
    }
  }
} else {
  console.log("  projects.json not found — skipping project migration");
}

// ── migrate source.yaml files ────────────────────────────────────────────────

const workspacesDir = join(runtimeDir, "workspaces");
let migratedEvidence = 0;
let skippedEvidence = 0;

if (!existsSync(workspacesDir)) {
  console.log("  workspaces/ not found — skipping evidence migration");
} else {
  const insert = db.prepare(`
    INSERT OR IGNORE INTO evidence_sources
      (id, workspace, source_type, origin, label, workspace_at_ingest,
       ingested_at, state, raw_filename, raw_sha256, source_sha256, state_updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `);

  const workspaces = readdirSync(workspacesDir, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => e.name);

  for (const workspace of workspaces) {
    const sourcesDir = join(workspacesDir, workspace, "evidence", "sources");
    if (!existsSync(sourcesDir)) continue;

    const evidenceDirs = readdirSync(sourcesDir, { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => e.name);

    for (const evidenceId of evidenceDirs) {
      const yamlPath = join(sourcesDir, evidenceId, "source.yaml");
      if (!existsSync(yamlPath)) continue;

      const raw = YAML.load(readFileSync(yamlPath, "utf8")) as Record<string, string>;
      const result = insert.run(
        raw.id,
        raw.workspace_at_ingest,
        raw.type,
        raw.origin,
        raw.label,
        raw.workspace_at_ingest,
        raw.ingested_at,
        raw.state,
        raw.raw_filename ?? "raw.md",
        raw.raw_sha256,
        raw.source_sha256,
        raw.ingested_at,
      );

      if (result.changes > 0) {
        console.log(`  evidence +  [${workspace}] ${raw.id}  (${raw.state})`);
        migratedEvidence++;
      } else {
        console.log(`  evidence ~  [${workspace}] ${raw.id}  (already exists, skipped)`);
        skippedEvidence++;
      }
    }
  }
}

// ── summary ──────────────────────────────────────────────────────────────────

console.log();
console.log(`Done.`);
console.log(`  projects : ${migratedProjects} migrated, ${skippedProjects} skipped`);
console.log(`  evidence : ${migratedEvidence} migrated, ${skippedEvidence} skipped`);
console.log();
console.log("Next: rebuild the binary so it picks up the DB changes.");
console.log("  bun run build");
