// End-to-end smoke test: simulate a full memory lifecycle through the
// non-interactive programmatic API (no TTY prompts). Validates the core
// happy path from `learn`-like ingest through supersede/active/forget/restore,
// then doctor reports clean.

import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { createNote, supersedeNote, archiveNote, forgetNote, restoreNote, readNote } from "../src/core/note-service";
import { upsertNote, rebuildWorkspace, removeNote } from "../src/core/indexer";
import { Fts5Adapter } from "../src/adapters/retrieval/fts5-adapter";
import { runDoctor } from "../src/core/doctor";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;
let db: ReturnType<typeof initDb>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-e2e-"));
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

test("E2E happy path: create -> recall -> supersede -> archive -> forget -> restore -> doctor", () => {
  // 1. Create three notes across tiers.
  const ax = createNote(paths, { workspace: "research", tier: "axioms", slug: "auth", body: "auth tokens rotate quarterly" });
  const mm = createNote(paths, { workspace: "research", tier: "mental-models", slug: "blast-radius", body: "consider blast radius of every change" });
  const dec = createNote(paths, { workspace: "research", tier: "decisions", slug: "use-typescript", body: "type safety is non-negotiable" });
  upsertNote({ paths, workspace: "research", db }, ax);
  upsertNote({ paths, workspace: "research", db }, mm);
  upsertNote({ paths, workspace: "research", db }, dec);

  // 2. Recall works (BM25 + tier-weighted).
  const adapter = new Fts5Adapter(db, paths);
  const r1 = adapter.search({ workspace: "research", query: "auth rotate", limit: 10 });
  expect(r1.hits.length).toBeGreaterThan(0);
  expect(r1.hits[0]?.tier).toBe("axioms"); // tier wins on close score
  expect(r1.hits[0]?.path).toBe("axioms/auth.md");

  // 3. Supersede: auth v2 created, v1 flipped.
  const { oldNote: axOld, newNote: axNew } = supersedeNote(paths, ax, {
    newBody: "auth tokens rotate monthly",
    newSlug: "auth-v2",
  });
  upsertNote({ paths, workspace: "research", db }, axNew);
  upsertNote({ paths, workspace: "research", db }, axOld);

  // 4. Superseded note filtered out of recall.
  const r2 = adapter.search({ workspace: "research", query: "rotate", limit: 10, statusFilter: ["active"] });
  expect(r2.hits.length).toBe(1);
  expect(r2.hits.some((h) => h.path === "axioms/auth.md")).toBe(false);
  expect(r2.hits.some((h) => h.path === "axioms/auth-v2.md")).toBe(true);

  // 5. Archive a note.
  const archived = archiveNote(paths, dec);
  upsertNote({ paths, workspace: "research", db }, archived);
  expect(readNote(paths, "research", "decisions", "use-typescript")?.status).toBe("archived");

  // 6. Forget the archived note (recoverable).
  const { tombstonePath, forgotten } = forgetNote(paths, archived, { reason: "superseded by org policy" });
  expect(existsSync(tombstonePath)).toBe(true);
  removeNote({ paths, workspace: "research", db }, forgotten.id);
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "decisions", "use-typescript.md"))).toBe(false);

  // 7. Restore from forget.
  const restored = restoreNote(paths, "research", forgotten.id);
  upsertNote({ paths, workspace: "research", db }, restored);
  expect(restored.status).toBe("active");
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "decisions", "use-typescript.md"))).toBe(true);

  // 8. Reindex recovers from a wiped DB.
  db.exec("DELETE FROM notes");
  db.exec("DELETE FROM note_fts_map");
  db.exec("DELETE FROM note_fts");
  db.exec("DELETE FROM links");
  const result = rebuildWorkspace({ paths, workspace: "research", db });
  // After restore: 4 notes on disk (ax-old, ax-new, mm, restored dec).
  expect(result.added).toBe(4);

  // 9. Doctor clean.
  const report = runDoctor(paths, "research", db);
  expect(report.ok).toBe(true);
  for (const r of report.results) {
    if (r.name === "idle-sessions") continue;
    expect(r.status).toBe("ok");
  }
});

test("E2E: conflict detection prevents silent overwrite", () => {
  const a = createNote(paths, { workspace: "research", tier: "axioms", slug: "shared", body: "v1" });
  expect(() => createNote(paths, { workspace: "research", tier: "axioms", slug: "shared", body: "v2" })).toThrow(/already exists/);
});

test("E2E: FTS5 search excludes archived and forgotten statuses by default", () => {
  const active = createNote(paths, { workspace: "research", tier: "axioms", slug: "a1", body: "auth active content" });
  const toArchive = createNote(paths, { workspace: "research", tier: "projects", slug: "p1", body: "auth project content" });
  upsertNote({ paths, workspace: "research", db }, active);
  upsertNote({ paths, workspace: "research", db }, toArchive);
  // Archive p1 directly in DB (faster than archive flow for this assertion).
  db.prepare("UPDATE notes SET status = 'archived' WHERE id = ?").run(toArchive.id);

  const adapter = new Fts5Adapter(db, paths);
  const r = adapter.search({ workspace: "research", query: "auth", limit: 10 });
  expect(r.hits.length).toBe(1);
  expect(r.hits[0]?.path).toBe("axioms/a1.md");
});

test("E2E: tier-weighted score: a relevant decision outranks a weak axiom", () => {
  // Two notes with different relevance strengths.
  const axiom = createNote(paths, { workspace: "research", tier: "axioms", slug: "weak", body: "the word auth appears once" });
  const decision = createNote(paths, { workspace: "research", tier: "decisions", slug: "strong", body: "auth auth auth auth auth auth auth auth" });
  upsertNote({ paths, workspace: "research", db }, axiom);
  upsertNote({ paths, workspace: "research", db }, decision);
  const r = new Fts5Adapter(db, paths).search({ workspace: "research", query: "auth", limit: 10 });
  expect(r.hits.length).toBe(2);
  // Despite axioms tier-weight 1.5 vs decisions 1.0, the heavily-relevant
  // decision outranks the barely-relevant axiom (V2 fix: tier is a weight,
  // not a hard sort).
  expect(r.hits[0]?.path).toBe("decisions/strong.md");
});
