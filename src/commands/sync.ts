// `zbrain sync` — commit local changes, pull --rebase, push, reindex.
// The git-backed transport for sharing a workspace across machines/teammates.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { openDb } from "../core/db";
import { initGitWorkspace, syncWorkspace } from "../core/git-sync";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface SyncOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

export interface SyncInitOptions extends SyncOptions {
  remote?: string;
}

export async function runSync(workspace: string, options: SyncOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = openDb(context.paths.runtimeDir);

  ui.intro(`zbrain sync ${workspace}`);
  const spinner = ui.spinner();
  spinner.start("syncing");
  const result = syncWorkspace(context.paths, db, workspace);
  spinner.stop("sync complete");

  const lines = [
    `committed: ${result.committed ? "yes" : "no"}`,
    `pulled: ${result.pulled ? "yes" : "no"}`,
    `pushed: ${result.pushed ? "yes" : "no"}`,
    `reindexed: +${result.reindexed.added} ~${result.reindexed.updated} -${result.reindexed.removed} (total ${result.reindexed.total})`,
  ];
  if (result.warnings.length > 0) {
    lines.push(`warnings: ${result.warnings.join(", ")}`);
  }
  ui.note(lines.join("\n"), "Sync summary");
  ui.outro(`Workspace "${workspace}" synced.`);
}

export async function runSyncInit(workspace: string, options: SyncInitOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  ui.intro(`zbrain sync init ${workspace}`);
  initGitWorkspace(context.paths, workspace, options.remote);
  ui.note(
    options.remote
      ? `git repo initialized with remote: ${options.remote}`
      : "git repo initialized (no remote — local-only until you add one with `git remote add origin <url>`)",
    "Sync init",
  );
  ui.outro(`Workspace "${workspace}" is now git-backed.`);
}

export function registerSyncCommand(program: Command): void {
  const sync = program
    .command("sync")
    .description("Sync a workspace via git (commit, pull --rebase, push, reindex)");

  sync
    .argument("<workspace>", "workspace name")
    .description("Commit local changes, pull --rebase, push, and reindex")
    .action((workspace: string) => runSync(workspace));

  sync
    .command("init")
    .argument("<workspace>", "workspace name")
    .description("Turn a workspace into a git repo, optionally with a remote")
    .option("--remote <url>", "git remote URL to add as origin")
    .action((workspace: string, options: { remote?: string }) => runSyncInit(workspace, { remote: options.remote }));
}
