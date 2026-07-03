import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath, workspaceRoot } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { createNote } from "../src/core/note-service";
import { Fts5Adapter } from "../src/adapters/retrieval/fts5-adapter";
import { GitSyncError, initGitWorkspace, isGitWorkspace, syncWorkspace } from "../src/core/git-sync";

let tempRoot: string;
let bareRepo: string;

function makeHome(name: string): { paths: ReturnType<typeof resolveRuntimePaths>; db: ReturnType<typeof initDb> } {
  const runtimeDir = mkdtempSync(join(tempRoot, `home-${name}-`));
  const paths = resolveRuntimePaths({ runtimeDir });
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "team", tier), { recursive: true });
  }
  // Mirrors what the real `workspace create` scaffold writes (helpers.ts:createWorkspaceScaffold):
  // untracked starter files that must not block adopting an existing remote's history.
  const wsRoot = workspaceRoot(paths, "team");
  mkdirSync(join(wsRoot, "evidence"), { recursive: true });
  writeFileSync(join(wsRoot, "workspace.md"), "# team\n", "utf8");
  writeFileSync(join(wsRoot, "evidence", "_index.md"), "# evidence index\n", "utf8");
  writeFileSync(join(wsRoot, ".zbrain-layout-version"), "2\n", "utf8");
  const db = initDb(runtimeDir);
  return { paths, db };
}

function git(cwd: string, args: string[]): void {
  const result = spawnSync("git", ["-C", cwd, ...args], { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(" ")} failed: ${result.stderr}`);
  }
}

beforeEach(() => {
  tempRoot = mkdtempSync(join(tmpdir(), "zbrain-sync-"));
  bareRepo = join(tempRoot, "remote.git");
  spawnSync("git", ["init", "--bare", bareRepo], { encoding: "utf8" });
});

afterEach(() => {
  rmSync(tempRoot, { recursive: true, force: true });
});

test("isGitWorkspace: false before init, true after", () => {
  const { paths } = makeHome("a");
  expect(isGitWorkspace(paths, "team")).toBe(false);
  initGitWorkspace(paths, "team");
  expect(isGitWorkspace(paths, "team")).toBe(true);
});

test("syncWorkspace: throws a clear hint when workspace is not a git repo", () => {
  const { paths, db } = makeHome("a");
  expect(() => syncWorkspace(paths, db, "team")).toThrow(GitSyncError);
  expect(() => syncWorkspace(paths, db, "team")).toThrow(/sync init/);
});

test("syncWorkspace: no remote configured commits locally, reindexes, and warns", () => {
  const { paths, db } = makeHome("a");
  initGitWorkspace(paths, "team");
  git(workspaceRoot(paths, "team"), ["config", "user.email", "a@test.local"]);
  git(workspaceRoot(paths, "team"), ["config", "user.name", "Home A"]);
  createNote(paths, { workspace: "team", tier: "axioms", slug: "solo", body: "solo note body" });

  const result = syncWorkspace(paths, db, "team");
  expect(result.committed).toBe(true);
  expect(result.pulled).toBe(false);
  expect(result.pushed).toBe(false);
  expect(result.warnings).toContain("no remote configured");
  expect(result.reindexed.added).toBe(1);
});

test("syncWorkspace: two machines round-trip notes through a shared bare repo", () => {
  const home1 = makeHome("a");
  const home2 = makeHome("b");

  // Home A: init with remote, write a note, sync (push).
  initGitWorkspace(home1.paths, "team", bareRepo);
  const wsRootA = workspaceRoot(home1.paths, "team");
  git(wsRootA, ["config", "user.email", "a@test.local"]);
  git(wsRootA, ["config", "user.name", "Home A"]);
  createNote(home1.paths, { workspace: "team", tier: "axioms", slug: "from-a", body: "note written on machine A" });
  const resultA = syncWorkspace(home1.paths, home1.db, "team");
  expect(resultA.committed).toBe(true);
  expect(resultA.pushed).toBe(true);
  expect(resultA.warnings).not.toContain("no remote configured");

  // Home B: clone the same remote instead of `git init` (simulates a teammate's first checkout).
  rmSync(workspaceRoot(home2.paths, "team"), { recursive: true, force: true });
  git(home2.paths.workspacesDir, ["clone", bareRepo, "team"]);
  const wsRootB = workspaceRoot(home2.paths, "team");
  git(wsRootB, ["config", "user.email", "b@test.local"]);
  git(wsRootB, ["config", "user.name", "Home B"]);

  const resultB = syncWorkspace(home2.paths, home2.db, "team");
  expect(resultB.pulled).toBe(true);
  expect(resultB.reindexed.added).toBe(1);

  // Home B sees Home A's note through retrieval (files -> DB, not a manual copy).
  const hitsB = new Fts5Adapter(home2.db, home2.paths).search({ workspace: "team", query: "machine", limit: 5 });
  expect(hitsB.hits.some((h) => h.path === "axioms/from-a.md")).toBe(true);

  // Home B writes its own note and syncs back.
  createNote(home2.paths, { workspace: "team", tier: "decisions", slug: "from-b", body: "note written on machine B" });
  const resultB2 = syncWorkspace(home2.paths, home2.db, "team");
  expect(resultB2.pushed).toBe(true);

  // Home A pulls B's note.
  const resultA2 = syncWorkspace(home1.paths, home1.db, "team");
  expect(resultA2.pulled).toBe(true);
  const hitsA = new Fts5Adapter(home1.db, home1.paths).search({ workspace: "team", query: "machine", limit: 5 });
  expect(hitsA.hits.some((h) => h.path === "decisions/from-b.md")).toBe(true);
  expect(hitsA.hits.some((h) => h.path === "axioms/from-a.md")).toBe(true);

  home1.db.close();
  home2.db.close();
});

test("initGitWorkspace: joining a workspace whose remote already has history adopts it instead of diverging", () => {
  const home1 = makeHome("a");
  const home2 = makeHome("b");

  // Home A: init with remote, write a note, push.
  initGitWorkspace(home1.paths, "team", bareRepo);
  const wsRootA = workspaceRoot(home1.paths, "team");
  git(wsRootA, ["config", "user.email", "a@test.local"]);
  git(wsRootA, ["config", "user.name", "Home A"]);
  createNote(home1.paths, { workspace: "team", tier: "axioms", slug: "from-a", body: "note written on machine A" });
  const resultA = syncWorkspace(home1.paths, home1.db, "team");
  expect(resultA.pushed).toBe(true);

  // Home B: join through the real CLI path (`sync init --remote`), not a manual `git clone`.
  const wsRootB = workspaceRoot(home2.paths, "team");
  expect(() => initGitWorkspace(home2.paths, "team", bareRepo)).not.toThrow();
  git(wsRootB, ["config", "user.email", "b@test.local"]);
  git(wsRootB, ["config", "user.name", "Home B"]);

  // A's note should already be on disk from the adopted history, no sync needed to see it.
  expect(existsSync(join(wsRootB, "wiki", "axioms", "from-a.md"))).toBe(true);

  // First sync on B should pull cleanly (no rebase conflict) and push its own note back.
  createNote(home2.paths, { workspace: "team", tier: "decisions", slug: "from-b", body: "note written on machine B" });
  const resultB = syncWorkspace(home2.paths, home2.db, "team");
  expect(resultB.pulled).toBe(true);
  expect(resultB.pushed).toBe(true);

  home1.db.close();
  home2.db.close();
});

test("initGitWorkspace: refuses to join and discard a workspace that already has real local content", () => {
  const home1 = makeHome("a");
  const home2 = makeHome("b");

  initGitWorkspace(home1.paths, "team", bareRepo);
  const wsRootA = workspaceRoot(home1.paths, "team");
  git(wsRootA, ["config", "user.email", "a@test.local"]);
  git(wsRootA, ["config", "user.name", "Home A"]);
  createNote(home1.paths, { workspace: "team", tier: "axioms", slug: "from-a", body: "note written on machine A" });
  syncWorkspace(home1.paths, home1.db, "team");

  // Home B had already been using "team" as a local, non-git workspace before
  // deciding to share it — it has a real note that was never pushed anywhere.
  createNote(home2.paths, { workspace: "team", tier: "axioms", slug: "pre-existing-local", body: "B's own note, written before joining" });
  const wsRootB = workspaceRoot(home2.paths, "team");

  let caught: unknown;
  try {
    initGitWorkspace(home2.paths, "team", bareRepo);
  } catch (err) {
    caught = err;
  }
  expect(caught).toBeInstanceOf(GitSyncError);
  expect((caught as Error).message).toMatch(/would be discarded/);

  // The note must survive the refused join attempt untouched.
  expect(existsSync(join(wsRootB, "wiki", "axioms", "pre-existing-local.md"))).toBe(true);

  // The refusal must leave the workspace retryable: no leftover `.git` from the
  // aborted attempt (otherwise a fixed retry would hit "already a git repo").
  expect(existsSync(join(wsRootB, ".git"))).toBe(false);
  rmSync(join(wsRootB, "wiki", "axioms", "pre-existing-local.md"));
  expect(() => initGitWorkspace(home2.paths, "team", bareRepo)).not.toThrow();

  home1.db.close();
  home2.db.close();
});

test("syncWorkspace: divergent-but-non-conflicting notes from both machines merge cleanly", () => {
  const home1 = makeHome("a");
  const home2 = makeHome("b");

  initGitWorkspace(home1.paths, "team", bareRepo);
  const wsRootA = workspaceRoot(home1.paths, "team");
  git(wsRootA, ["config", "user.email", "a@test.local"]);
  git(wsRootA, ["config", "user.name", "Home A"]);
  // Seed + push so B has something to clone.
  syncWorkspace(home1.paths, home1.db, "team");

  rmSync(workspaceRoot(home2.paths, "team"), { recursive: true, force: true });
  git(home2.paths.workspacesDir, ["clone", bareRepo, "team"]);
  const wsRootB = workspaceRoot(home2.paths, "team");
  git(wsRootB, ["config", "user.email", "b@test.local"]);
  git(wsRootB, ["config", "user.name", "Home B"]);

  // Both machines write a different note before either syncs again.
  createNote(home1.paths, { workspace: "team", tier: "axioms", slug: "note-a", body: "authored on A" });
  createNote(home2.paths, { workspace: "team", tier: "axioms", slug: "note-b", body: "authored on B" });

  const resA = syncWorkspace(home1.paths, home1.db, "team"); // pushes note-a
  expect(resA.pushed).toBe(true);
  const resB = syncWorkspace(home2.paths, home2.db, "team"); // rebases on note-a, pushes note-b
  expect(resB.pulled).toBe(true);
  expect(resB.pushed).toBe(true);
  const resA2 = syncWorkspace(home1.paths, home1.db, "team"); // pulls note-b
  expect(resA2.pulled).toBe(true);

  const hitsA = new Fts5Adapter(home1.db, home1.paths).search({ workspace: "team", query: "authored", limit: 5 });
  const hitsB = new Fts5Adapter(home2.db, home2.paths).search({ workspace: "team", query: "authored", limit: 5 });
  expect(hitsA.hits.length).toBe(2);
  expect(hitsB.hits.length).toBe(2);

  home1.db.close();
  home2.db.close();
});
