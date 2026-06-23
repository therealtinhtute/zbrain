import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { createNote } from "../src/core/note-service";
import { rebuildWorkspace } from "../src/core/indexer";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-lazy-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("AC-P1-8: indexer.rebuildWorkspace recovers from a wiped DB", () => {
  // Seed 3 notes.
  createNote(paths, { workspace: "research", tier: "axioms", slug: "n1", body: "alpha content" });
  createNote(paths, { workspace: "research", tier: "projects", slug: "n2", body: "beta content" });
  createNote(paths, { workspace: "research", tier: "decisions", slug: "n3", body: "gamma content" });

  // Wipe the DB.
  const db = initDb(tempHome);
  db.exec("DELETE FROM notes");
  db.exec("DELETE FROM note_fts_map");
  db.exec("DELETE FROM note_fts");
  db.exec("DELETE FROM links");

  // Files still on disk; rebuild should restore.
  const result = rebuildWorkspace({ paths, workspace: "research", db });
  expect(result.added).toBe(3);
  const count = db.prepare(`SELECT COUNT(*) as c FROM notes WHERE workspace = ?`).get("research") as { c: number };
  expect(count.c).toBe(3);
  db.close();
});

test("AC-P1-8: rebuildWorkspace is a no-op on a fresh workspace", () => {
  const db = initDb(tempHome);
  const result = rebuildWorkspace({ paths, workspace: "research", db });
  expect(result.added).toBe(0);
  expect(result.total).toBe(0);
  db.close();
});
