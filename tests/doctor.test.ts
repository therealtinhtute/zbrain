import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { initDb } from "../src/core/db";
import { runDoctor, checkDbFileConsistency, checkBrokenLinks, checkFtsSync, checkSchemaVersion } from "../src/core/doctor";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { createNote, supersedeNote } from "../src/core/note-service";
import { upsertNote, rebuildWorkspace, removeNote } from "../src/core/indexer";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;
let db: ReturnType<typeof initDb>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-doc-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  db = initDb(tempHome);
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
});

afterEach(() => {
  db.close();
  rmSync(tempHome, { recursive: true, force: true });
});

test("checkSchemaVersion: passes when version matches", () => {
  const result = checkSchemaVersion(db);
  expect(result.status).toBe("ok");
});

test("checkDbFileConsistency: detects DB row with missing file", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "alpha", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  // Delete the file but leave the DB row.
  const { unlinkSync } = require("node:fs");
  unlinkSync(note.path);
  const result = checkDbFileConsistency(paths, "research", db);
  expect(result.status).not.toBe("ok");
  expect(result.findings.some((f) => f.message.includes("alpha.md"))).toBe(true);
});

test("checkDbFileConsistency: clean when DB matches files", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "beta", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  const result = checkDbFileConsistency(paths, "research", db);
  expect(result.status).toBe("ok");
});

test("checkFtsSync: warns on row count drift", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "gamma", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  // Manually delete the map row to create drift.
  db.prepare(`DELETE FROM note_fts_map WHERE note_id = ?`).run(note.id);
  const result = checkFtsSync("research", db);
  expect(result.status).not.toBe("ok");
  expect(result.findings.some((f) => f.message.includes("out of sync"))).toBe(true);
});

test("checkFtsSync: clean when notes + map in sync", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "delta", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  const result = checkFtsSync("research", db);
  expect(result.status).toBe("ok");
});

test("checkBrokenLinks: detects target missing", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "epsilon", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  // Insert a link to a nonexistent target.
  db.prepare(`
    INSERT INTO links (from_id, workspace, type, to_id) VALUES (?, 'research', 'supersedes', 'nonexistent-id')
  `).run(note.id);
  const result = checkBrokenLinks("research", db);
  expect(result.status).not.toBe("ok");
  expect(result.findings.some((f) => f.message.includes("nonexistent-id"))).toBe(true);
});

test("runDoctor: full report on clean workspace", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "zeta", body: "x" });
  upsertNote({ paths, workspace: "research", db }, note);
  const report = runDoctor(paths, "research", db);
  expect(report.results.length).toBe(8);
  // 7 of 8 are deterministic-ok; idle-sessions is a no-op.
  for (const r of report.results) {
    if (r.name === "idle-sessions") continue;
    expect(r.status).toBe("ok");
  }
});

test("runDoctor: detects orphaned evidence row", () => {
  // Insert a DB row but no raw file.
  db.prepare(`
    INSERT INTO evidence_sources (id, workspace, source_type, origin, label, workspace_at_ingest, ingested_at, state, raw_filename, raw_sha256, source_sha256, state_updated_at)
    VALUES ('orphan', 'research', 'paste', '/dev/null', 'orphan', 'research', '2026-06-23T00:00:00.000Z', 'ingested', 'raw.md', 'x', 'y', '2026-06-23T00:00:00.000Z')
  `).run();
  const report = runDoctor(paths, "research", db);
  const orph = report.results.find((r) => r.name === "orphaned-evidence");
  expect(orph?.status).toBe("error");
});
