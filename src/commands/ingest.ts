import { readFileSync } from "node:fs";
import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { reviewEvidence } from "../core/evidence-review";
import { applyEvidence, type ApplyMutation } from "../core/evidence-apply";
import type { VerifiedFactRecord } from "../core/evidence-store";
import { QmdAdapter, type QmdRunner } from "../core/qmd-adapter";
import { listEvidenceItems } from "../core/evidence-list";
import { resolveActiveWorkspace } from "../core/workspace-resolver";
import {
  questionSeverities,
  questionStatuses,
  type QuestionSeverity,
  type QuestionStatus,
} from "../core/evidence-state";
import { openDb } from "../core/db";
import type { RuntimePathOptions, RuntimePaths } from "../core/runtime-paths";

interface BaseIngestOptions {
  workspace?: string;
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  nowIso?: string;
}

export interface IngestReviewOptions extends BaseIngestOptions {
  fact?: string;
  questionId?: string;
  wikiPath?: string;
  severity?: QuestionSeverity;
  status?: QuestionStatus;
}

export interface IngestApplyOptions extends BaseIngestOptions {
  path?: string;
  content?: string;
  contentFile?: string;
  qmdRunner?: QmdRunner;
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
  const db = openDb(context.paths.runtimeDir);
  const items = listEvidenceItems(db, workspace);

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

export async function runIngestReview(evidenceId: string, options: IngestReviewOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const workspace = resolveWorkspaceName(context.paths, options.workspace);

  const severity = options.severity ?? "P0";
  const status = options.status ?? "answered";
  if (!questionSeverities.includes(severity)) {
    throw new Error(`Invalid question severity: ${severity} (expected one of ${questionSeverities.join(", ")})`);
  }
  if (!questionStatuses.includes(status)) {
    throw new Error(`Invalid question status: ${status} (expected one of ${questionStatuses.join(", ")})`);
  }
  const questionId = options.questionId ?? "q-1";

  // A verified fact is only required when the question is actually answered.
  // Blocking statuses (awaiting_external/deferred/open) record the question with no fact.
  const facts: VerifiedFactRecord[] = [];
  if (status === "answered") {
    const fact = options.fact ?? await ui.text({
      message: "Verified fact",
      placeholder: "Fact verified from this evidence",
      validate: (value) => value?.trim() ? undefined : "Verified fact is required.",
    });
    const wikiPath = options.wikiPath ?? await ui.text({
      message: "Target wiki path",
      placeholder: "projects/example.md",
      validate: (value) => value?.trim() ? undefined : "Target wiki path is required.",
    });
    facts.push({ statement: fact, questionId, wikiPath });
  }

  ui.intro("zbrain ingest review");
  const db = openDb(context.paths.runtimeDir);
  reviewEvidence(db, context.paths, {
    workspace,
    evidenceId,
    nowIso: options.nowIso,
    facts,
    questions: [{ id: questionId, severity, status }],
  });
  ui.note([`workspace: ${workspace}`, `evidence_id: ${evidenceId}`, `state: reviewed`, `${questionId}: ${severity}/${status}`].join("\n"), "Review summary");
  ui.outro("Review recorded.");
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
  const db = openDb(context.paths.runtimeDir);
  const qmd = new QmdAdapter(context.paths, options.qmdRunner);
  const result = applyEvidence(db, context.paths, {
    workspace,
    evidenceId,
    nowIso: options.nowIso,
    mutations: [mutation],
    reindex: (targetWorkspace) => {
      try {
        qmd.indexWorkspace({ workspace: targetWorkspace });
      } catch (error) {
        ui.note(`qmd reindex skipped: ${(error as Error).message}`, "Warning");
      }
    },
  });
  ui.note([`workspace: ${workspace}`, `evidence_id: ${evidenceId}`, `applied: ${result.applied.length}`].join("\n"), "Apply summary");
  ui.outro("Evidence applied.");
}

export function registerIngestCommands(program: Command): void {
  const ingest = program
    .command("ingest")
    .description("Process learned evidence through review, apply, and list");

  ingest
    .command("list")
    .description("List evidence items and next actions")
    .option("--workspace <name>", "target workspace")
    .action((options: BaseIngestOptions) => runIngestList(options));

  ingest
    .command("review")
    .argument("<id>", "evidence id")
    .description("Extract and record verified facts from an ingested evidence source")
    .option("--workspace <name>", "target workspace")
    .option("--fact <fact>", "verified fact")
    .option("--question-id <id>", "question id", "q-1")
    .option("--wiki-path <path>", "target wiki path for the fact")
    .option("--severity <severity>", "question severity (P0-P3)", "P0")
    .option("--status <status>", "question status (open|answered|awaiting_external|deferred)", "answered")
    .action((id: string, options: IngestReviewOptions) => runIngestReview(id, options));

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
