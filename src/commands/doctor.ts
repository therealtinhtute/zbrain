// `zbrain doctor` CLI — health + reconciliation.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext, listWorkspaceNames } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { initDb } from "../core/db";
import { runDoctor, fixIdleSessions } from "../core/doctor";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface DoctorOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  workspace?: string;
  json?: boolean;
  fix?: boolean;
}

export async function runDoctorCommand(options: DoctorOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);

  const targets = options.workspace
    ? [options.workspace]
    : listWorkspaceNames(context.paths);

  if (targets.length === 0) {
    ui.note("No workspaces to check.", "zbrain doctor");
    return;
  }

  ui.intro("zbrain doctor");
  if (options.fix) {
    for (const workspace of targets) {
      const fixed = fixIdleSessions(db, context.paths, workspace);
      if (fixed > 0) ui.note(`Removed ${fixed} idle session(s).`, `workspace: ${workspace} --fix`);
    }
  }
  let allOk = true;
  for (const workspace of targets) {
    const report = runDoctor(context.paths, workspace, db);
    if (options.json) {
      console.log(JSON.stringify({ workspace, ...report }, null, 2));
      continue;
    }
    const lines: string[] = [];
    for (const r of report.results) {
      const icon = r.status === "ok" ? "✓" : r.status === "warn" ? "!" : "✗";
      lines.push(`${icon} ${r.name}: ${r.status}`);
      for (const f of r.findings) {
        lines.push(`    - ${f.message}${f.detail ? ` (${f.detail})` : ""}`);
      }
    }
    ui.note(lines.join("\n"), `workspace: ${workspace}`);
    if (!report.ok) allOk = false;
  }
  ui.outro(allOk ? "All clean." : "Issues found.");
}

export function registerDoctorCommand(program: Command): void {
  program
    .command("doctor")
    .description("Check workspace health + reconciliation")
    .option("--workspace <name>", "check a single workspace")
    .option("--json", "machine-readable output")
    .option("--fix", "GC idle sessions (30+ days inactive)")
    .action((options: { workspace?: string; json?: boolean; fix?: boolean }) =>
      runDoctorCommand({ workspace: options.workspace, json: options.json, fix: options.fix }),
    );
}
