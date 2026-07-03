import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { writeGlobalConfig } from "../src/core/config";
import { Fts5Adapter } from "../src/adapters/retrieval/fts5-adapter";
import { runNoteAdd, runNoteUpdate } from "../src/commands/note";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

const notes: string[] = [];
const testUi = {
  intro() {},
  outro() {},
  info() {},
  note(message: string) { notes.push(message); },
  spinner() { return { start() {}, stop() {} }; },
  async confirm() { return true; },
  async text() { return ""; },
  async select() { return ""; },
  async multiselect() { return []; },
};

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-note-add-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
  writeGlobalConfig(paths.configFile, { default_workspace: "research" });
  notes.length = 0;
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("note add: creates a file, DB row, and is immediately searchable", async () => {
  await runNoteAdd({
    tier: "axioms",
    title: "Rate Limits",
    body: "The public API caps at 100 req/min per key.",
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
  });

  expect(notes[0]).toContain("id:");
  expect(notes[0]).toContain("axioms/rate-limits.md");

  const db = initDb(tempHome);
  const row = db.prepare("SELECT * FROM notes WHERE workspace = 'research'").get() as { path: string; status: string };
  expect(row.path).toBe("axioms/rate-limits.md");
  expect(row.status).toBe("active");

  const hits = new Fts5Adapter(db, paths).search({ workspace: "research", query: "rate limits", limit: 5 });
  expect(hits.hits.some((h) => h.path === "axioms/rate-limits.md")).toBe(true);
  db.close();
});

test("note add: rejects a slug conflict and names the existing note id", async () => {
  await runNoteAdd({
    tier: "axioms",
    title: "Dup",
    slug: "dup",
    body: "first",
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
  });
  const firstId = notes[0].match(/id: ([\w-]+)/)?.[1];
  expect(firstId).toBeTruthy();

  await expect(
    runNoteAdd({
      tier: "axioms",
      title: "Dup",
      slug: "dup",
      body: "second",
      ui: testUi as any,
      pathOptions: { runtimeDir: tempHome },
    }),
  ).rejects.toThrow(new RegExp(`Conflict.*${firstId}`, "s"));
});

test("note add: reads body from --file when --body is absent", async () => {
  const filePath = join(tempHome, "body.md");
  writeFileSync(filePath, "content read from a file", "utf8");
  await runNoteAdd({
    tier: "decisions",
    title: "From File",
    file: filePath,
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
  });
  expect(notes[0]).toContain("decisions/from-file.md");
});

test("note add: rejects an invalid tier", async () => {
  await expect(
    runNoteAdd({
      tier: "not-a-tier",
      title: "Bad Tier",
      body: "x",
      ui: testUi as any,
      pathOptions: { runtimeDir: tempHome },
    }),
  ).rejects.toThrow(/Invalid tier/);
});

test("note update: only checks conflict when --slug is explicitly passed (regression)", async () => {
  await runNoteAdd({
    tier: "axioms",
    title: "Original",
    slug: "original",
    body: "v1",
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
  });
  const id = notes[0].match(/id: ([\w-]+)/)?.[1]!;
  notes.length = 0;

  // No --slug given: must NOT throw a false conflict (old bug compared slug against tier).
  await runNoteUpdate(id, {
    body: "v2",
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
  });
  expect(notes[0]).toContain("superseded");
  expect(notes[0]).toContain("(active)");
});
