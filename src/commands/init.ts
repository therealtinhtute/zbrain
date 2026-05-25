import { assertRuntimeReady, createCommandContext, initProject, listWorkspaceNames, updateRuntime } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface InitCommandOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

const initTargetOptions = [
  { value: "claude_rules", label: "CLAUDE.md rules", hint: "append non-destructively" },
  { value: "commands", label: "Slash commands", hint: ".claude/commands" },
  { value: "agents", label: "Subagents", hint: ".claude/agents" },
  { value: "mcp", label: "MCP config", hint: ".claude/settings.local.json" },
] as const;

export async function runInit(options: InitCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  updateRuntime(context.paths);

  const workspaces = listWorkspaceNames(context.paths);
  if (workspaces.length === 0) {
    throw new Error("No workspaces available. Run `zbrain workspace create <name>` first.");
  }

  ui.intro("zbrain init");
  const workspace = await ui.select({
    message: "Workspace for this project?",
    options: workspaces.map((name) => ({ value: name, label: name })),
    initialValue: workspaces[0],
  });
  const injectTargets = await ui.multiselect({
    message: "What should be injected into this project?",
    options: initTargetOptions.map((option) => ({ ...option })),
    initialValues: ["claude_rules", "commands", "agents", "mcp"],
  });

  const spinner = ui.spinner();
  spinner.start("Integrating project");
  const result = initProject(context.paths, { workspace, injectTargets });
  spinner.stop("Project integrated");

  ui.note(
    [
      `workspace: ${workspace}`,
      `created: ${result.created.length}`,
      `updated: ${result.updated.length}`,
    ].join("\n"),
    "Init summary",
  );
  ui.outro("Project initialized.");
}
