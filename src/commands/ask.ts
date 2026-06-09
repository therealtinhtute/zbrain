import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { readProjectBinding } from "../core/config";
import { retrieveMultiWorkspaceContext, retrieveWorkspaceContext, type RetrievalAdapter } from "../core/retrieval";
import { resolveActiveWorkspace } from "../core/workspace-resolver";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface AskCommandOptions {
  workspace?: string;
  limit?: string | number;
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  adapter?: RetrievalAdapter;
}

export async function runAsk(query: string, options: AskCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  const active = options.workspace ?? resolveActiveWorkspace(context.paths).name;
  const projectBinding = readProjectBinding(context.paths.projectRegistryFile, context.paths.cwd);
  const secondaries = options.workspace ? [] : projectBinding?.secondary_workspaces ?? [];
  const limit = typeof options.limit === "string" ? Number(options.limit) : options.limit;

  ui.intro("zbrain ask");
  const spinner = ui.spinner();
  spinner.start("Retrieving workspace context");
  const result = secondaries.length > 0
    ? retrieveMultiWorkspaceContext(
        context.paths,
        {
          primaryWorkspace: active,
          query,
          secondaries,
          workspacesDir: context.paths.workspacesDir,
          limit,
        },
        options.adapter,
      )
    : retrieveWorkspaceContext(context.paths, { workspace: active, query, limit }, options.adapter);
  spinner.stop("Context retrieved");

  ui.note(
    [
      `workspace: ${active}`,
      `results: ${result.results.length}`,
      `context_file: ${result.filePath}`,
    ].join("\n"),
    "Ask summary",
  );
  ui.outro("Context ready.");
}

export function registerAskCommand(program: Command): void {
  program
    .command("ask")
    .argument("<question>", "question to retrieve context for")
    .description("Retrieve ranked workspace context for a question")
    .option("--workspace <name>", "target workspace")
    .option("--limit <n>", "result limit")
    .action((question: string, options: AskCommandOptions) => runAsk(question, options));
}
