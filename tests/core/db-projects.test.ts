import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { initDb } from "../../src/core/db";
import { upsertProject, readProject, listProjects, readProjectRegistry } from "../../src/core/db-projects";

function setup() {
  const dir = mkdtempSync(join(tmpdir(), "zbrain-proj-"));
  const db = initDb(dir);
  return { dir, db };
}

function teardown(dir: string, db: ReturnType<typeof initDb>) {
  db.close();
  rmSync(dir, { recursive: true, force: true });
}

const NOW = "2026-06-16T00:00:00.000Z";

const BINDING = {
  project_root: "/Users/test/myproject",
  workspace: "research",
  context_file: "/Users/test/.zbrain/projects/abc/current-task.md",
  runtimes: ["claude"] as ["claude"],
};

describe("upsertProject / readProject", () => {
  test("roundtrip: insert and read back", () => {
    const { dir, db } = setup();
    try {
      upsertProject(db, BINDING, NOW);
      const result = readProject(db, BINDING.project_root);
      expect(result?.project_root).toBe(BINDING.project_root);
      expect(result?.workspace).toBe(BINDING.workspace);
      expect(result?.context_file).toBe(BINDING.context_file);
      expect(result?.runtimes).toEqual(["claude"]);
    } finally {
      teardown(dir, db);
    }
  });

  test("returns null for missing project", () => {
    const { dir, db } = setup();
    try {
      expect(readProject(db, "/nonexistent")).toBeNull();
    } finally {
      teardown(dir, db);
    }
  });

  test("upsert updates existing row", () => {
    const { dir, db } = setup();
    try {
      upsertProject(db, BINDING, NOW);
      upsertProject(db, { ...BINDING, workspace: "finance" }, NOW);
      const result = readProject(db, BINDING.project_root);
      expect(result?.workspace).toBe("finance");
    } finally {
      teardown(dir, db);
    }
  });

  test("secondary_workspaces JSON roundtrip", () => {
    const { dir, db } = setup();
    try {
      const binding = {
        ...BINDING,
        secondary_workspaces: [{ workspace: "finance", keywords: ["money"], limit: 2 }],
      };
      upsertProject(db, binding, NOW);
      const result = readProject(db, BINDING.project_root);
      expect(result?.secondary_workspaces).toEqual([
        { workspace: "finance", keywords: ["money"], limit: 2 },
      ]);
    } finally {
      teardown(dir, db);
    }
  });
});

describe("listProjects / readProjectRegistry", () => {
  test("lists all projects sorted by project_root", () => {
    const { dir, db } = setup();
    try {
      upsertProject(db, { ...BINDING, project_root: "/z/proj" }, NOW);
      upsertProject(db, { ...BINDING, project_root: "/a/proj" }, NOW);
      const list = listProjects(db);
      expect(list[0]?.project_root).toBe("/a/proj");
      expect(list[1]?.project_root).toBe("/z/proj");
    } finally {
      teardown(dir, db);
    }
  });

  test("readProjectRegistry wraps projects array", () => {
    const { dir, db } = setup();
    try {
      upsertProject(db, BINDING, NOW);
      const registry = readProjectRegistry(db);
      expect(registry.projects).toHaveLength(1);
      expect(registry.projects[0]?.project_root).toBe(BINDING.project_root);
    } finally {
      teardown(dir, db);
    }
  });

  test("empty DB returns empty registry", () => {
    const { dir, db } = setup();
    try {
      const registry = readProjectRegistry(db);
      expect(registry.projects).toHaveLength(0);
    } finally {
      teardown(dir, db);
    }
  });
});
