#!/usr/bin/env bun

import { Command } from "commander";
import { runInit } from "./commands/init";
import { runSetup } from "./commands/setup";
import { runUpdate } from "./commands/update";
import { registerWorkspaceCommands } from "./commands/workspace";
import { runInteractive } from "./commands/interactive";
import { registerLearnCommand } from "./commands/learn";
import { registerIngestCommands } from "./commands/ingest";
import { registerAskCommand } from "./commands/ask";
import { registerReindexCommand } from "./commands/reindex";
import { registerNoteCommand } from "./commands/note";
import { registerLeaseCommand } from "./commands/lease";
import { registerDoctorCommand } from "./commands/doctor";
import { registerMcpCommand } from "./commands/mcp";

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
  registerLearnCommand(program);
  registerIngestCommands(program);
  registerAskCommand(program);
  registerReindexCommand(program);
  registerNoteCommand(program);
  registerLeaseCommand(program);
  registerDoctorCommand(program);
  registerMcpCommand(program);

  program
    .command("update")
    .description("Refresh bundled runtime assets")
    .action(runUpdate);

  return program;
}

if (import.meta.main) {
  const userArgs = Bun.argv.slice(2);
  try {
    if (userArgs.length === 0 && process.stdin.isTTY) {
      await runInteractive();
    } else {
      await createProgram().parseAsync(Bun.argv);
    }
  } catch (err) {
    if (err instanceof Error && err.message === "Command cancelled") {
      process.exit(0);
    }
    throw err;
  }
}
