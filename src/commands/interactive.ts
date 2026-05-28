import { existsSync } from "node:fs";
import { clackUi, type CommandUi } from "./ui";
import { runSetup } from "./setup";
import { runInit } from "./init";
import { runUpdate } from "./update";
import { runWorkspaceCreate } from "./workspace";
import { createCommandContext, listWorkspaceNames } from "./helpers";
import type { RuntimePathOptions } from "../core/runtime-paths";

const BANNER = `
░▀█▀░██▄▒█▀▄▒▄▀▄░█░█▄░█░░
░█▄▄▒█▄█░█▀▄░█▀█░█░█▒▀█▒░
`;

const PRESET_WORKSPACES = ["research"];

export interface InteractiveOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

export async function runInteractive(options: InteractiveOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  const runtimeExists = existsSync(context.paths.runtimeDir);
  const workspaces = runtimeExists ? listWorkspaceNames(context.paths) : [];

  ui.intro(BANNER);

  type ActionKey = "setup" | "workspace_create" | "init" | "update";

  const menuOptions: { value: ActionKey; label: string; hint: string }[] = [];

  if (!runtimeExists) {
    menuOptions.push({ value: "setup", label: "Setup runtime", hint: "first-time setup" });
  } else {
    menuOptions.push({ value: "workspace_create", label: "Create workspace", hint: "add a new knowledge domain" });
    if (workspaces.length > 0) {
      menuOptions.push({ value: "init", label: "Init this project", hint: "link current directory to a workspace" });
    }
    menuOptions.push({ value: "update", label: "Update runtime", hint: "refresh bundled assets" });
  }

  const action = await ui.select({
    message: "What would you like to do?",
    options: menuOptions,
    initialValue: menuOptions[0]?.value,
  });

  if (action === "setup") {
    await runSetup({ ui, pathOptions: options.pathOptions });
  } else if (action === "workspace_create") {
    const workspaceName = await promptWorkspaceName(ui, workspaces);
    await runWorkspaceCreate(workspaceName, { ui, pathOptions: options.pathOptions });
  } else if (action === "init") {
    await runInit({ ui, pathOptions: options.pathOptions });
  } else if (action === "update") {
    await runUpdate({ ui, pathOptions: options.pathOptions });
  }
}

async function promptWorkspaceName(ui: CommandUi, existing: string[]): Promise<string> {
  const presets = PRESET_WORKSPACES.filter((p) => !existing.includes(p));
  const choices = [
    ...presets.map((p) => ({ value: p, label: p })),
    { value: "__custom__", label: "custom name..." },
  ];

  const picked = await ui.select({
    message: "Workspace name?",
    options: choices,
    initialValue: choices[0]?.value,
  });

  if (picked !== "__custom__") {
    return picked;
  }

  return ui.text({
    message: "Workspace name (lowercase, hyphens only):",
    placeholder: "my-workspace",
    validate: (v) => {
      if (!v || !/^[a-z0-9-]+$/.test(v.trim())) {
        return "Use lowercase letters, numbers, and hyphens only.";
      }
    },
  });
}
