import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { migrateV1ToV2, migrateAllWorkspaces } from "../src/core/workspace-migration";
import { getWorkspaceLayoutVersion, WORKSPACE_LAYOUT_VERSION } from "../src/core/workspace-layout";

let tempHome: string;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-mig-"));
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

function seedV1Workspace(workspaceName: string, files: Record<string, string> = {}): string {
  const ws = join(tempHome, "workspaces", workspaceName);
  mkdirSync(join(ws, "axioms"), { recursive: true });
  mkdirSync(join(ws, "mental-models"), { recursive: true });
  mkdirSync(join(ws, "projects"), { recursive: true });
  mkdirSync(join(ws, "decisions"), { recursive: true });
  for (const [relPath, content] of Object.entries(files)) {
    const fullPath = join(ws, relPath);
    mkdirSync(join(fullPath, ".."), { recursive: true });
    writeFileSync(fullPath, content);
  }
  return ws;
}

test("migrateV1ToV2: V1 axioms/ files move to wiki/axioms/", () => {
  const ws = seedV1Workspace("alpha", {
    "axioms/a1.md": "# axiom one",
    "axioms/a2.md": "# axiom two",
    "mental-models/m1.md": "# mental model",
  });

  expect(getWorkspaceLayoutVersion(ws)).toBe(1);

  const result = migrateV1ToV2(ws);

  expect(result.fromVersion).toBe(1);
  expect(result.toVersion).toBe(WORKSPACE_LAYOUT_VERSION);
  expect(result.skipped).toBe(false);
  expect(result.movedFiles.length).toBe(3);
  expect(existsSync(join(ws, "wiki", "axioms", "a1.md"))).toBe(true);
  expect(existsSync(join(ws, "wiki", "axioms", "a2.md"))).toBe(true);
  expect(existsSync(join(ws, "wiki", "mental-models", "m1.md"))).toBe(true);
  expect(existsSync(join(ws, "axioms", "a1.md"))).toBe(false);
  expect(getWorkspaceLayoutVersion(ws)).toBe(WORKSPACE_LAYOUT_VERSION);
});

test("migrateV1ToV2: idempotent on second call", () => {
  const ws = seedV1Workspace("beta", { "axioms/a.md": "# a" });

  const first = migrateV1ToV2(ws);
  const second = migrateV1ToV2(ws);

  expect(first.skipped).toBe(false);
  expect(first.movedFiles.length).toBe(1);
  expect(second.skipped).toBe(true);
  expect(second.movedFiles.length).toBe(0);
  expect(existsSync(join(ws, "wiki", "axioms", "a.md"))).toBe(true);
});

test("migrateV1ToV2: V2 workspace is a no-op", () => {
  const ws = join(tempHome, "workspaces", "gamma");
  mkdirSync(join(ws, "wiki", "axioms"), { recursive: true });
  writeFileSync(join(ws, ".zbrain-layout-version"), "2\n");
  writeFileSync(join(ws, "wiki", "axioms", "x.md"), "# x");

  const result = migrateV1ToV2(ws);

  expect(result.fromVersion).toBe(2);
  expect(result.toVersion).toBe(2);
  expect(result.skipped).toBe(true);
  expect(result.movedFiles.length).toBe(0);
});

test("migrateV1ToV2: missing tier dir creates wiki/<tier>/ with no files moved", () => {
  const ws = join(tempHome, "workspaces", "delta");
  mkdirSync(join(ws, "axioms"), { recursive: true });
  writeFileSync(join(ws, "axioms", "only.md"), "# only");

  const result = migrateV1ToV2(ws);

  // axioms had a file (moved); mental-models, projects, decisions had no source dir
  expect(result.movedFiles.length).toBe(1);
  expect(result.createdDirs.length).toBe(4); // all 4 tier dirs created under wiki/
  expect(existsSync(join(ws, "wiki", "mental-models"))).toBe(true);
  expect(existsSync(join(ws, "wiki", "projects"))).toBe(true);
  expect(existsSync(join(ws, "wiki", "decisions"))).toBe(true);
});

test("migrateAllWorkspaces: migrates every V1 workspace under the dir", () => {
  seedV1Workspace("ws-1", { "axioms/a.md": "# a" });
  seedV1Workspace("ws-2", { "projects/p.md": "# p" });

  const summary = migrateAllWorkspaces(join(tempHome, "workspaces"));

  expect(summary.results.length).toBe(2);
  expect(summary.anyMigrated).toBe(true);
  expect(summary.results[0]?.result.movedFiles.length).toBe(1);
  expect(summary.results[1]?.result.movedFiles.length).toBe(1);
});

test("migrateAllWorkspaces: empty dir returns empty summary", () => {
  const summary = migrateAllWorkspaces(join(tempHome, "does-not-exist"));
  expect(summary.results.length).toBe(0);
  expect(summary.anyMigrated).toBe(false);
});
