// `zbrain reindex` — rebuilds the V2 index (notes + FTS5 + links) from files.
// Disaster-recovery primitive: `rm zbrain.db && zbrain reindex` is lossless.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { openDb } from "../core/db";
import { rebuildWorkspace } from "../core/indexer";
import { listWorkspaceNames } from "./helpers";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface ReindexOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  workspace?: string;
}

export async function runReindex(options: ReindexOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = openDb(context.paths.runtimeDir);

  const targets = options.workspace
    ? [options.workspace]
    : listWorkspaceNames(context.paths);

  if (targets.length === 0) {
    ui.note("No workspaces found.", "zbrain reindex");
    return;
  }

  ui.intro("zbrain reindex");
  const spinner = ui.spinner();
  for (const workspace of targets) {
    spinner.start(`reindexing ${workspace}`);
    const result = rebuildWorkspace({ paths: context.paths, workspace, db });
    spinner.stop(
      `${workspace}: +${result.added} ~${result.updated} -${result.removed} (total ${result.total})`,
    );
  }
  ui.outro("Reindex complete.");
}

export function registerReindexCommand(program: Command): void {
  program
    .command("reindex")
    .description("Rebuild the note index from files")
    .option("--workspace <name>", "reindex only this workspace")
    .action((options: { workspace?: string }) => runReindex({ workspace: options.workspace }));
}
