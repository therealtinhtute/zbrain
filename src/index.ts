#!/usr/bin/env bun

import { Command } from "commander";
import { runInit } from "./commands/init";
import { runSetup } from "./commands/setup";
import { runUpdate } from "./commands/update";
import { registerWorkspaceCommands } from "./commands/workspace";

export function createProgram(): Command {
  const program = new Command();

  program
    .name("zbrain")
    .description("CLI for workspace-isolated personal LLM wiki workflows")
    .showHelpAfterError();

  program
    .command("setup")
    .description("Prepare the ~/.zbrain runtime")
    .action(runSetup);

  program
    .command("init")
    .description("Integrate zbrain into the current project")
    .action(runInit);

  registerWorkspaceCommands(program);

  program
    .command("update")
    .description("Refresh bundled runtime assets")
    .action(runUpdate);

  return program;
}

if (import.meta.main) {
  await createProgram().parseAsync(Bun.argv);
}
