import { readFileSync } from "node:fs";
import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { analyzeEvidence } from "../core/evidence-analyze";
import { applyEvidence, type ApplyMutation } from "../core/evidence-apply";
import { completeEvidenceQa } from "../core/evidence-qa";
import { listEvidenceItems } from "../core/evidence-list";
import { resolveActiveWorkspace } from "../core/workspace-resolver";
import type { RuntimePathOptions, RuntimePaths } from "../core/runtime-paths";
import type { EvidenceQuestion } from "../core/evidence-state";

interface BaseIngestOptions {
  workspace?: string;
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  nowIso?: string;
}

export interface IngestQaOptions extends BaseIngestOptions {
  fact?: string;
  questionId?: string;
  wikiPath?: string;
}

export interface IngestApplyOptions extends BaseIngestOptions {
  path?: string;
  content?: string;
  contentFile?: string;
}

function resolveWorkspaceName(paths: RuntimePaths, workspace?: string): string {
  if (workspace) {
    return workspace;
  }
  return resolveActiveWorkspace(paths).name;
}

export async function runIngestList(options: BaseIngestOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  const workspace = resolveWorkspaceName(context.paths, options.workspace);
  const items = listEvidenceItems(context.paths, workspace);

  ui.intro("zbrain ingest list");
  ui.note(
    items.length === 0
      ? `workspace: ${workspace}\nitems: 0`
      : [
          `workspace: ${workspace}`,
          "",
          ...items.map((item) =>
            `${item.state}  ${item.id}  ${item.label ?? "(untitled)"}  ->  ${item.nextCommand ?? "complete"}`,
          ),
        ].join("\n"),
    "Evidence",
  );
  ui.outro("Evidence listed.");
}

export async function runIngestAnalyze(evidenceId: string, options: BaseIngestOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const workspace = resolveWorkspaceName(context.paths, options.workspace);

  ui.intro("zbrain ingest analyze");
  const spinner = ui.spinner();
  spinner.start("Analyzing evidence");
  const files = analyzeEvidence(context.paths, { workspace, evidenceId, nowIso: options.nowIso });
  spinner.stop("Evidence analyzed");
  ui.note([`workspace: ${workspace}`, `evidence_id: ${evidenceId}`, `files: ${files.length}`].join("\n"), "Analyze summary");
  ui.outro("Analysis complete.");
}

export async function runIngestQa(evidenceId: string, options: IngestQaOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const workspace = resolveWorkspaceName(context.paths, options.workspace);
  const fact = options.fact ?? await ui.text({
    message: "Verified fact",
    placeholder: "Fact verified from this evidence",
    validate: (value) => value?.trim() ? undefined : "Verified fact is required.",
  });
  const questionId = options.questionId ?? "q-1";
  const wikiPath = options.wikiPath ?? await ui.text({
    message: "Target wiki path",
    placeholder: "projects/example.md",
    validate: (value) => value?.trim() ? undefined : "Target wiki path is required.",
  });

  ui.intro("zbrain ingest qa");
  const questions: EvidenceQuestion[] = [{ id: questionId, severity: "P0", status: "answered" }];
  const result = completeEvidenceQa(context.paths, {
    workspace,
    evidenceId,
    nowIso: options.nowIso,
    questions,
    facts: [{ statement: fact, questionId, wikiPath }],
  });
  ui.note([`workspace: ${workspace}`, `evidence_id: ${evidenceId}`, `state: ${result.state}`].join("\n"), "QA summary");
  ui.outro("QA recorded.");
}

export async function runIngestApply(evidenceId: string, options: IngestApplyOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const workspace = resolveWorkspaceName(context.paths, options.workspace);
  const relativePath = options.path ?? await ui.text({
    message: "Target wiki path",
    placeholder: "projects/example.md",
    validate: (value) => value?.trim() ? undefined : "Target wiki path is required.",
  });
  const content = options.contentFile
    ? readFileSync(options.contentFile, "utf8")
    : options.content ?? await ui.text({
        message: "Wiki file content",
        placeholder: "# Title",
        validate: (value) => value?.trim() ? undefined : "Wiki content is required.",
      });

  ui.intro("zbrain ingest apply");
  const mutation: ApplyMutation = {
    relativePath,
    content,
    citations: [{ questionId: "q-1", wikiPath: relativePath }],
  };
  const result = applyEvidence(context.paths, {
    workspace,
    evidenceId,
    nowIso: options.nowIso,
    questions: [{ id: "q-1", severity: "P0", status: "answered" }],
    mutations: [mutation],
  });
  ui.note([`workspace: ${workspace}`, `evidence_id: ${evidenceId}`, `applied: ${result.applied.length}`].join("\n"), "Apply summary");
  ui.outro("Evidence applied.");
}

export function registerIngestCommands(program: Command): void {
  const ingest = program
    .command("ingest")
    .description("Process learned evidence through analyze, qa, apply, and list");

  ingest
    .command("list")
    .description("List evidence items and next actions")
    .option("--workspace <name>", "target workspace")
    .action((options: BaseIngestOptions) => runIngestList(options));

  ingest
    .command("analyze")
    .argument("<id>", "evidence id")
    .description("Analyze an ingested evidence source")
    .option("--workspace <name>", "target workspace")
    .action((id: string, options: BaseIngestOptions) => runIngestAnalyze(id, options));

  ingest
    .command("qa")
    .argument("<id>", "evidence id")
    .description("Record verified facts for analyzed evidence")
    .option("--workspace <name>", "target workspace")
    .option("--fact <fact>", "verified fact")
    .option("--question-id <id>", "question id", "q-1")
    .option("--wiki-path <path>", "target wiki path for the fact")
    .action((id: string, options: IngestQaOptions) => runIngestQa(id, options));

  ingest
    .command("apply")
    .argument("<id>", "evidence id")
    .description("Apply verified evidence into workspace knowledge")
    .option("--workspace <name>", "target workspace")
    .option("--path <path>", "target wiki path")
    .option("--content <content>", "wiki file content")
    .option("--content-file <path>", "read wiki file content from a file")
    .action((id: string, options: IngestApplyOptions) => runIngestApply(id, options));
}
