import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { initDb } from "../src/core/db";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { writeGlobalConfig } from "../src/core/config";
import { runAsk } from "../src/commands/ask";
import {
  touchSession,
  listStaleSessions,
  deleteSession,
  sessionContextPath,
  writeSessionContext,
} from "../src/core/session";

const testUi = {
  intro() {},
  outro() {},
  info() {},
  note() {},
  spinner() { return { start() {}, stop() {} }; },
  async confirm() { return true; },
  async text() { return ""; },
  async select() { return ""; },
  async multiselect() { return []; },
};

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;
let db: ReturnType<typeof initDb>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-session-db-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  db = initDb(tempHome);
});

afterEach(() => {
  db.close();
  rmSync(tempHome, { recursive: true, force: true });
});

test("touchSession: inserts a row, then bumps last_activity_at on repeat calls", () => {
  touchSession(db, { id: "s1", projectRoot: "/proj", workspace: "research" });
  const row1 = db.prepare(`SELECT * FROM sessions WHERE id = 's1'`).get() as any;
  expect(row1.project_root).toBe("/proj");
  expect(row1.started_at).toBe(row1.last_activity_at);

  const firstActivity = row1.last_activity_at;
  // Force a distinguishable timestamp so the update is provably a bump, not a no-op.
  db.prepare(`UPDATE sessions SET last_activity_at = '2020-01-01T00:00:00.000Z' WHERE id = 's1'`).run();
  touchSession(db, { id: "s1", projectRoot: "/proj", workspace: "research" });
  const row2 = db.prepare(`SELECT * FROM sessions WHERE id = 's1'`).get() as any;
  expect(row2.last_activity_at).not.toBe("2020-01-01T00:00:00.000Z");
  expect(row2.started_at).toBe(row1.started_at);
});

test("listStaleSessions: filters by idle threshold and optional workspace", () => {
  touchSession(db, { id: "fresh", projectRoot: "/proj", workspace: "research" });
  touchSession(db, { id: "stale-research", projectRoot: "/proj", workspace: "research" });
  touchSession(db, { id: "stale-other", projectRoot: "/proj", workspace: "other" });
  const oldTs = "2020-01-01T00:00:00.000Z";
  db.prepare(`UPDATE sessions SET last_activity_at = ? WHERE id = 'stale-research'`).run(oldTs);
  db.prepare(`UPDATE sessions SET last_activity_at = ? WHERE id = 'stale-other'`).run(oldTs);

  const allStale = listStaleSessions(db, 30);
  expect(allStale.map((s) => s.id).sort()).toEqual(["stale-other", "stale-research"]);

  const staleResearch = listStaleSessions(db, 30, "research");
  expect(staleResearch.map((s) => s.id)).toEqual(["stale-research"]);
});

test("deleteSession: removes the DB row and the context file", () => {
  const projectRoot = "/proj";
  writeSessionContext(paths, projectRoot, "s1", "# context");
  touchSession(db, { id: "s1", projectRoot, workspace: "research" });
  const target = sessionContextPath(paths, projectRoot, "s1");
  expect(existsSync(target)).toBe(true);

  deleteSession(db, paths, "s1", projectRoot);
  expect(db.prepare(`SELECT * FROM sessions WHERE id = 's1'`).get()).toBeNull();
  expect(existsSync(target)).toBe(false);
});

test("deleteSession: tolerates a missing context file (row-only cleanup)", () => {
  touchSession(db, { id: "s1", projectRoot: "/proj", workspace: "research" });
  expect(() => deleteSession(db, paths, "s1", "/proj")).not.toThrow();
  expect(db.prepare(`SELECT * FROM sessions WHERE id = 's1'`).get()).toBeNull();
});

test("runAsk: writes a queryable session row", async () => {
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
  writeGlobalConfig(paths.configFile, { default_workspace: "research" });

  await runAsk("what is the rate limit?", {
    ui: testUi as any,
    pathOptions: { runtimeDir: tempHome },
    sessionId: "ask-session-1",
  });

  const row = db.prepare(`SELECT * FROM sessions WHERE id = 'ask-session-1'`).get() as any;
  expect(row).not.toBeNull();
  expect(row.workspace).toBe("research");
  expect(row.project_root).toBe(paths.cwd);
});
