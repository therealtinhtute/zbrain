// `zbrain sync` — turns a workspace directory into a git-backed shared store.
// Files remain the source of truth (per V2 architecture); git is the transport
// between machines. The DB is rebuilt from files after every pull, never synced
// directly, so it never needs to be merge-resolved.

import { existsSync, writeFileSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { hostname } from "node:os";
import type { Database } from "bun:sqlite";
import { workspaceRoot, type RuntimePaths } from "./runtime-paths";
import { rebuildWorkspace, type RebuildResult } from "./indexer";

export interface SyncResult {
  workspace: string;
  committed: boolean;
  pulled: boolean;
  pushed: boolean;
  reindexed: RebuildResult;
  warnings: string[];
}

export class GitSyncError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "GitSyncError";
  }
}

function runGit(wsRoot: string, args: string[]): { status: number; stdout: string; stderr: string } {
  const result = spawnSync("git", ["-C", wsRoot, ...args], { encoding: "utf8" });
  return {
    status: result.status ?? 1,
    stdout: (result.stdout ?? "").trim(),
    stderr: (result.stderr ?? "").trim(),
  };
}

export function isGitWorkspace(paths: RuntimePaths, workspace: string): boolean {
  return existsSync(`${workspaceRoot(paths, workspace)}/.git`);
}

export function hasRemote(wsRoot: string): boolean {
  const result = runGit(wsRoot, ["remote"]);
  return result.status === 0 && result.stdout.trim().length > 0;
}

function currentBranch(wsRoot: string): string {
  const result = runGit(wsRoot, ["rev-parse", "--abbrev-ref", "HEAD"]);
  if (result.status !== 0) {
    throw new GitSyncError(`git rev-parse --abbrev-ref HEAD failed: ${result.stderr}`);
  }
  return result.stdout.trim();
}

const REMOTE_BRANCH_CANDIDATES = ["main", "master"];

// The exact set of untracked files `workspace create` seeds before a workspace
// is ever git-backed (helpers.ts:createWorkspaceScaffold) plus the `.gitignore`
// this module seeds. Anything else untracked is real user content (notes,
// evidence) that must never be silently discarded when joining a remote.
const SAFE_TO_DISCARD_SCAFFOLD = new Set(["workspace.md", "evidence/_index.md", ".zbrain-layout-version", ".gitignore"]);

export function initGitWorkspace(paths: RuntimePaths, workspace: string, remoteUrl?: string): void {
  const wsRoot = workspaceRoot(paths, workspace);
  if (!existsSync(wsRoot)) {
    throw new GitSyncError(`Workspace "${workspace}" does not exist. Run \`zbrain workspace create ${workspace}\` first.`);
  }
  if (isGitWorkspace(paths, workspace)) {
    throw new GitSyncError(`Workspace "${workspace}" is already a git repo.`);
  }
  const init = runGit(wsRoot, ["init"]);
  if (init.status !== 0) {
    throw new GitSyncError(`git init failed: ${init.stderr}`);
  }

  let joinedExisting = false;
  if (remoteUrl) {
    const remote = runGit(wsRoot, ["remote", "add", "origin", remoteUrl]);
    if (remote.status !== 0) {
      throw new GitSyncError(`git remote add failed: ${remote.stderr}`);
    }

    // If the remote already has history (a teammate joining a workspace someone
    // else already pushed), adopt that branch instead of leaving this machine's
    // `git init` as an unrelated root — otherwise the first `sync` rebases an
    // independent history onto the remote's and conflicts on the very first file.
    const fetch = runGit(wsRoot, ["fetch", "origin"]);
    if (fetch.status === 0) {
      for (const branch of REMOTE_BRANCH_CANDIDATES) {
        const verify = runGit(wsRoot, ["rev-parse", "--verify", "-q", `refs/remotes/origin/${branch}`]);
        if (verify.status === 0) {
          // `workspace create` already scaffolded starter files (workspace.md,
          // evidence/_index.md, .zbrain-layout-version) as untracked content in
          // this fresh, commit-less repo, and those same files exist in the
          // remote's history too (the first teammate's own scaffold got
          // committed) — so it's normally safe to drop the local copies and
          // let checkout restore them from origin. But if this workspace was
          // already in use before being turned git-backed (real notes written
          // via `learn`/`note add`), those files are untracked too and must
          // NOT be swept away by a blind `git clean`. `--untracked-files=all`
          // lists every individual untracked path (unlike `git clean -n`,
          // which collapses whole directories into one line and would hide a
          // real note sitting next to an empty scaffold dir of the same name).
          const status = runGit(wsRoot, ["status", "--porcelain", "--untracked-files=all"]);
          const untracked = status.stdout
            .split("\n")
            .filter(Boolean)
            .map((line) => line.replace(/^\?\?\s+/, ""));
          const unexpected = untracked.filter((p) => !SAFE_TO_DISCARD_SCAFFOLD.has(p));
          if (unexpected.length > 0) {
            // Leave the workspace exactly as it was before this call — undo the
            // `git init` above — so the error's suggested retry actually works
            // instead of immediately hitting "already a git repo".
            rmSync(`${wsRoot}/.git`, { recursive: true, force: true });
            throw new GitSyncError(
              `Refusing to join existing remote branch "${branch}" for workspace "${workspace}": ` +
                `local content beyond the fresh scaffold would be discarded (${unexpected.join(", ")}). ` +
                `Move it aside manually, then re-run \`zbrain sync init ${workspace} --remote <url>\`.`,
            );
          }
          const clean = runGit(wsRoot, ["clean", "-fd"]);
          if (clean.status !== 0) {
            throw new GitSyncError(`git clean failed while joining existing remote branch "${branch}": ${clean.stderr}`);
          }
          const checkout = runGit(wsRoot, ["checkout", "-B", branch, `origin/${branch}`]);
          if (checkout.status !== 0) {
            throw new GitSyncError(`git checkout of existing remote branch "${branch}" failed: ${checkout.stderr}`);
          }
          joinedExisting = true;
          break;
        }
      }
    }
  }

  if (joinedExisting) {
    return;
  }

  // Git doesn't track empty directories (the freshly-scaffolded wiki/<tier>/
  // dirs have no files yet), so without a real file HEAD stays unborn and the
  // first `sync` has no commit to rebase onto. Seed one so `sync` always has
  // something to work with. Nothing is excluded by default — .trash/ syncs
  // too, so `forget` propagates to the team.
  const gitignorePath = `${wsRoot}/.gitignore`;
  if (!existsSync(gitignorePath)) {
    writeFileSync(gitignorePath, "# zbrain: nothing ignored by default — .trash/ syncs so forget propagates.\n", "utf8");
  }
}

export function syncWorkspace(paths: RuntimePaths, db: Database, workspace: string): SyncResult {
  const wsRoot = workspaceRoot(paths, workspace);
  if (!isGitWorkspace(paths, workspace)) {
    throw new GitSyncError(
      `Workspace "${workspace}" is not a git repo. Run \`zbrain sync init ${workspace} [--remote <url>]\` first.`,
    );
  }

  const warnings: string[] = [];

  // 1. Commit any local changes first, so a pull --rebase has a clean tree to rebase.
  const status = runGit(wsRoot, ["status", "--porcelain"]);
  let committed = false;
  if (status.stdout.length > 0) {
    const add = runGit(wsRoot, ["add", "-A"]);
    if (add.status !== 0) {
      throw new GitSyncError(`git add failed: ${add.stderr}`);
    }
    const commit = runGit(wsRoot, [
      "commit",
      "-m",
      `sync: ${hostname()} ${new Date().toISOString()}`,
    ]);
    if (commit.status !== 0) {
      throw new GitSyncError(`git commit failed: ${commit.stderr}`);
    }
    committed = true;
  }

  const remotePresent = hasRemote(wsRoot);
  let pulled = false;
  let pushed = false;

  if (remotePresent) {
    const branch = currentBranch(wsRoot);
    const pull = runGit(wsRoot, ["pull", "--rebase", "origin", branch]);
    if (pull.status !== 0) {
      // A brand-new remote has no ref for this branch yet — nothing to pull, not an error.
      if (/couldn't find remote ref|unknown revision/i.test(pull.stderr)) {
        warnings.push(`remote has no "${branch}" ref yet (first sync)`);
      } else {
        // Never auto-resolve a rebase conflict — abort and surface it.
        runGit(wsRoot, ["rebase", "--abort"]);
        throw new GitSyncError(
          `git pull --rebase failed for "${workspace}" (conflict or network issue). ` +
            `Resolve manually in ${wsRoot}, then re-run \`zbrain sync ${workspace}\`.\n${pull.stderr}`,
        );
      }
    } else {
      pulled = true;
    }

    const push = runGit(wsRoot, ["push", "-u", "origin", branch]);
    if (push.status !== 0) {
      throw new GitSyncError(`git push failed for "${workspace}": ${push.stderr}`);
    }
    pushed = true;
  } else {
    warnings.push("no remote configured");
  }

  const reindexed = rebuildWorkspace({ paths, workspace, db });

  return { workspace, committed, pulled, pushed, reindexed, warnings };
}
