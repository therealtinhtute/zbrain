import { Command } from "commander";
import { assertRuntimeReady, createCommandContext, createWorkspaceScaffold, validateWorkspaceName } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { initDb } from "../core/db";
import { readProjectBinding } from "../core/config";
import { resolveActiveWorkspace, WorkspaceResolutionError } from "../core/workspace-resolver";
import type { RuntimePaths, RuntimePathOptions } from "../core/runtime-paths";

export interface WorkspaceCurrentOptions {
  pathOptions?: RuntimePathOptions;
}

export interface WorkspaceCurrentResult {
  project_root: string;
  workspace: string;
  secondary_workspaces: unknown[];
  context_file: string | null;
}

// Replacement read path for AC-P1-9: agents resolve the active workspace binding
// via this command instead of reading `~/.zbrain/projects.json` directly —
// SQLite (via `readProjectBinding`) is now the only source of truth.
export function resolveWorkspaceCurrent(paths: RuntimePaths): WorkspaceCurrentResult | { error: string } {
  const db = initDb(paths.runtimeDir);
  const binding = readProjectBinding(db, paths.cwd);
  if (binding) {
    return {
      project_root: binding.project_root,
      workspace: binding.workspace,
      secondary_workspaces: binding.secondary_workspaces ?? [],
      context_file: binding.context_file,
    };
  }

  try {
    const resolved = resolveActiveWorkspace(paths);
    return {
      project_root: paths.cwd,
      workspace: resolved.name,
      secondary_workspaces: [],
      context_file: null,
    };
  } catch (err) {
    if (err instanceof WorkspaceResolutionError) {
      return { error: err.message };
    }
    throw err;
  }
}

export function runWorkspaceCurrent(options: WorkspaceCurrentOptions = {}): void {
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  console.log(JSON.stringify(resolveWorkspaceCurrent(context.paths), null, 2));
}

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

  workspace
    .command("current")
    .description("Print the resolved workspace binding for this project as JSON")
    .action(() => runWorkspaceCurrent());
}
