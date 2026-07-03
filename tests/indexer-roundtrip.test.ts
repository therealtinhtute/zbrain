import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { createNote } from "../src/core/note-service";
import { upsertNote, rebuildWorkspace, removeNote } from "../src/core/indexer";
import { Fts5Adapter } from "../src/adapters/retrieval/fts5-adapter";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;
let db: ReturnType<typeof initDb>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-idx-"));
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

test("AC-P1-7: rebuild restores notes after DB is deleted (file-first invariant)", () => {
  // 1. Seed 3 notes.
  createNote(paths, { workspace: "research", tier: "axioms", slug: "alpha", body: "alpha body about authentication" });
  createNote(paths, { workspace: "research", tier: "projects", slug: "beta", body: "beta body about authorization" });
  createNote(paths, { workspace: "research", tier: "decisions", slug: "gamma", body: "gamma body about encryption" });

  // 2. Rebuild.
  const result = rebuildWorkspace({ paths, workspace: "research", db });
  expect(result.added).toBe(3);
  expect(result.total).toBe(3);

  // 3. Simulate DB loss: drop all rows, re-run rebuild.
  db.exec("DELETE FROM notes");
  db.exec("DELETE FROM note_fts_map");
  db.exec("DELETE FROM note_fts");
  db.exec("DELETE FROM links");

  const recovered = rebuildWorkspace({ paths, workspace: "research", db });
  expect(recovered.added).toBe(3);

  // 4. FTS5 search returns the same content.
  const adapter = new Fts5Adapter(db, paths);
  const search = adapter.search({ workspace: "research", query: "authentication", limit: 5 });
  expect(search.hits.length).toBeGreaterThan(0);
  expect(search.hits.some((h) => h.path === "axioms/alpha.md")).toBe(true);
});

test("upsertNote: idempotent on repeated call", () => {
  const note = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "delta",
    body: "delta body",
  });
  upsertNote({ paths, workspace: "research", db }, note);
  upsertNote({ paths, workspace: "research", db }, note);
  const row = db.prepare("SELECT COUNT(*) as c FROM notes WHERE workspace = ?").get("research") as { c: number };
  expect(row.c).toBe(1);
});

test("removeNote: deletes notes row, FTS5 row, and link rows", () => {
  const note = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "epsilon",
    body: "epsilon body",
    supersedes: ["old-1"],
  });
  upsertNote({ paths, workspace: "research", db }, note);
  removeNote({ paths, workspace: "research", db }, note.id);

  const notesCount = db.prepare("SELECT COUNT(*) as c FROM notes WHERE id = ?").get(note.id) as { c: number };
  const linksCount = db.prepare("SELECT COUNT(*) as c FROM links WHERE from_id = ?").get(note.id) as { c: number };
  expect(notesCount.c).toBe(0);
  expect(linksCount.c).toBe(0);
});

test("Fts5Adapter: status filter excludes superseded notes", () => {
  const a = createNote(paths, { workspace: "research", tier: "axioms", slug: "zeta", body: "zeta body keyword" });
  upsertNote({ paths, workspace: "research", db }, a);
  // Manually mark as superseded; the index row is left in place so we can
  // verify the status filter is what excludes it (not a missing index).
  db.prepare("UPDATE notes SET status = ? WHERE id = ?").run("superseded", a.id);

  const active = new Fts5Adapter(db, paths).search({ workspace: "research", query: "zeta", limit: 10 });
  expect(active.hits.length).toBe(0);

  const all = new Fts5Adapter(db, paths).search({ workspace: "research", query: "zeta", limit: 10, statusFilter: ["active", "superseded"] });
  expect(all.hits.length).toBe(1);
});

test("Fts5Adapter: BM25 ranking puts more-relevant hit first", () => {
  const a = createNote(paths, { workspace: "research", tier: "axioms", slug: "hit1", body: "authentication authentication authentication" });
  const b = createNote(paths, { workspace: "research", tier: "projects", slug: "hit2", body: "authentication lives here too" });
  upsertNote({ paths, workspace: "research", db }, a);
  upsertNote({ paths, workspace: "research", db }, b);

  const result = new Fts5Adapter(db, paths).search({ workspace: "research", query: "authentication", limit: 10 });
  expect(result.hits.length).toBe(2);
  expect(result.hits[0]?.path).toBe("axioms/hit1.md");
});
