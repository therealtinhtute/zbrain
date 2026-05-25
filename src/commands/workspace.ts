import { Command } from "commander";
import { assertRuntimeReady, createCommandContext, createWorkspaceScaffold, validateWorkspaceName } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface WorkspaceCreateOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  nowIso?: string;
}

export async function runWorkspaceCreate(
  name: string,
  options: WorkspaceCreateOptions = {},
): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  const nowIso = options.nowIso ?? new Date().toISOString();
  const workspaceName = validateWorkspaceName(name);
  assertRuntimeReady(context.paths);

  ui.intro("zbrain workspace create");
  const confirmed = await ui.confirm({ message: `Create workspace "${workspaceName}"?`, initialValue: true });
  if (!confirmed) {
    ui.outro("Workspace creation skipped.");
    return;
  }

  const spinner = ui.spinner();
  spinner.start("Scaffolding workspace");
  const result = createWorkspaceScaffold(context.paths, workspaceName, nowIso);
  spinner.stop("Workspace scaffolded");

  ui.note(
    [
      `workspace: ${workspaceName}`,
      `created_dirs: ${result.createdPaths.length}`,
      `default_workspace_set: ${result.defaultWorkspaceSet ? "yes" : "no"}`,
    ].join("\n"),
    "Workspace summary",
  );
  ui.outro(`Workspace "${workspaceName}" created.`);
}

export function registerWorkspaceCommands(program: Command): void {
  const workspace = program
    .command("workspace")
    .description("Manage zbrain workspaces");

  workspace
    .command("create")
    .argument("<name>", "workspace name")
    .description("Create a workspace scaffold")
    .action((name: string) => runWorkspaceCreate(name));
}
