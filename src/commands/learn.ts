import { readFileSync } from "node:fs";
import { basename } from "node:path";
import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { ingestEvidence } from "../core/evidence-ingest";
import { openDb } from "../core/db";
import { resolveWorkspaceName } from "../core/workspace-resolver";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface LearnCommandOptions {
  workspace?: string;
  type?: string;
  origin?: string;
  label?: string;
  file?: string;
  rawContent?: string;
  nowIso?: string;
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

async function readStdin(): Promise<string> {
  let contents = "";
  for await (const chunk of process.stdin) {
    contents += chunk;
  }
  return contents;
}

async function resolveRawContent(ui: CommandUi, options: LearnCommandOptions): Promise<string> {
  if (typeof options.rawContent === "string") {
    return options.rawContent;
  }

  if (options.file) {
    return readFileSync(options.file, "utf8");
  }

  if (!process.stdin.isTTY) {
    return readStdin();
  }

  return ui.text({
    message: "Source text",
    placeholder: "Paste the source material to learn",
    validate: (value) => value?.trim() ? undefined : "Source text is required.",
  });
}

export async function runLearn(options: LearnCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  const workspace = resolveWorkspaceName(context.paths, options.workspace);
  const rawContent = await resolveRawContent(ui, options);
  const label = options.label ?? (options.file ? basename(options.file) : "learning note");
  const sourceType = options.type ?? "paste";
  const origin = options.origin ?? "cli";

  ui.intro("zbrain learn");
  const spinner = ui.spinner();
  spinner.start("Recording source");
  const db = openDb(context.paths.runtimeDir);
  const result = ingestEvidence(db, context.paths, {
    workspace,
    sourceType,
    origin,
    label,
    rawContent,
    nowIso: options.nowIso,
  });
  spinner.stop("Source recorded");

  ui.note(
    [
      `workspace: ${workspace}`,
      `evidence_id: ${result.evidenceId}`,
      `source: ${result.rawFile}`,
      `next: zbrain ingest review ${result.evidenceId}`,
    ].join("\n"),
    "Learn summary",
  );
  ui.outro("Learning source added.");
}

export function registerLearnCommand(program: Command): void {
  program
    .command("learn")
    .description("Record new source material into the active workspace evidence sources")
    .option("--workspace <name>", "target workspace")
    .option("--type <type>", "source type", "paste")
    .option("--origin <origin>", "source origin", "cli")
    .option("--label <label>", "source label")
    .option("--file <path>", "read source material from a file")
    .action((options: LearnCommandOptions) => runLearn(options));
}
