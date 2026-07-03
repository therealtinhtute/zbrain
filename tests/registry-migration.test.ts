import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { initProject } from "../src/commands/helpers";
import { resolveWorkspaceCurrent } from "../src/commands/workspace";

function scaffoldWorkspace(name: string): void {
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, name, tier), { recursive: true });
  }
}

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-registry-"));
  mkdirSync(tempHome, { recursive: true });
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("initProject on a fresh home never creates projects.json", () => {
  scaffoldWorkspace("research");
  const db = initDb(tempHome);
  initProject(db, paths, { workspace: "research", injectTargets: [] });
  db.close();

  expect(existsSync(join(tempHome, "projects.json"))).toBe(false);
});

test("legacy projects.json is imported into SQLite on initDb, then archived to .bak", () => {
  const legacyRegistry = {
    projects: [
      {
        project_root: "/some/legacy/project",
        workspace: "legacy-ws",
        context_file: "/some/legacy/project/.claude/current-task.md",
        runtimes: ["claude"],
        secondary_workspaces: [],
      },
    ],
  };
  writeFileSync(join(tempHome, "projects.json"), JSON.stringify(legacyRegistry, null, 2), "utf8");

  const db = initDb(tempHome);
  const row = db.prepare(`SELECT * FROM projects WHERE project_root = ?`).get("/some/legacy/project") as any;
  expect(row).not.toBeNull();
  expect(row.workspace).toBe("legacy-ws");
  db.close();

  expect(existsSync(join(tempHome, "projects.json"))).toBe(false);
  expect(existsSync(join(tempHome, "projects.json.bak"))).toBe(true);
  const archived = JSON.parse(readFileSync(join(tempHome, "projects.json.bak"), "utf8"));
  expect(archived.projects[0].project_root).toBe("/some/legacy/project");
});

test("legacy projects.json migration does not clobber an existing SQLite binding for the same project", () => {
  const db1 = initDb(tempHome);
  db1.prepare(`
    INSERT INTO projects (project_root, workspace, context_file, runtimes, secondary_workspaces, created_at, updated_at)
    VALUES (?, ?, ?, '[]', '[]', ?, ?)
  `).run("/proj", "current-truth", "/proj/.claude/current-task.md", "2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z");
  db1.close();

  writeFileSync(
    join(tempHome, "projects.json"),
    JSON.stringify({ projects: [{ project_root: "/proj", workspace: "stale-legacy-value", context_file: "x" }] }),
    "utf8",
  );

  const db2 = initDb(tempHome);
  const row = db2.prepare(`SELECT workspace FROM projects WHERE project_root = ?`).get("/proj") as any;
  expect(row.workspace).toBe("current-truth");
  db2.close();
});

test("malformed legacy projects.json is archived (not deleted) without throwing", () => {
  writeFileSync(join(tempHome, "projects.json"), "{ not valid json", "utf8");
  expect(() => initDb(tempHome)).not.toThrow();
  expect(existsSync(join(tempHome, "projects.json"))).toBe(false);
  expect(existsSync(join(tempHome, "projects.json.bak"))).toBe(true);
});

test("workspace current returns the resolved binding after init", () => {
  scaffoldWorkspace("research");
  const db = initDb(tempHome);
  initProject(db, paths, { workspace: "research", injectTargets: [] });
  db.close();

  const result = resolveWorkspaceCurrent(paths) as any;
  expect(result.workspace).toBe("research");
  expect(result.project_root).toBe(paths.cwd);
  expect(typeof result.context_file).toBe("string");
});
