import { createHash } from "node:crypto";
import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import YAML from "js-yaml";
import { ensureDir, pathInside } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  questionSeverities,
  questionStatuses,
  type EvidenceQuestion,
  type QuestionSeverity,
  type QuestionStatus,
} from "./evidence-state";

export interface EvidenceLocationSet {
  workspaceRoot: string;
  evidenceRoot: string;
  sourcesRoot: string;
  sourceDir: string;
  analysisDir: string;
  qaDir: string;
  appliedDir: string;
  archiveDir: string;
  rawFile: string;
  sourceFile: string;
  indexFile: string;
  verifiedFactsFile: string;
  qaAnswersFile: string;
  checkpointFile: string;
  manifestFile: string;
}

export interface EvidenceSourceRecord {
  id: string;
  type: string;
  origin: string;
  label: string;
  workspace_at_ingest: string;
  ingested_at: string;
  state: string;
  raw_filename: string;
  raw_sha256: string;
  source_sha256: string;
}

function slugify(input: string): string {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 30) || "evidence";
}

export function sha256(input: string): string {
  return createHash("sha256").update(input).digest("hex");
}

export function evidenceLocations(paths: RuntimePaths, workspace: string, evidenceId: string): EvidenceLocationSet {
  const workspaceRoot = join(paths.workspacesDir, workspace);
  const evidenceRoot = join(workspaceRoot, "evidence");
  const sourcesRoot = join(evidenceRoot, "sources");
  const sourceDir = join(sourcesRoot, evidenceId);
  const analysisDir = join(evidenceRoot, "analysis", evidenceId);
  const qaDir = join(evidenceRoot, "qa", evidenceId);
  const appliedDir = join(evidenceRoot, "applied", evidenceId);
  const archiveDir = join(evidenceRoot, "archive", evidenceId);

  return {
    workspaceRoot,
    evidenceRoot,
    sourcesRoot,
    sourceDir,
    analysisDir,
    qaDir,
    appliedDir,
    archiveDir,
    rawFile: join(sourceDir, "raw.md"),
    sourceFile: join(sourceDir, "source.yaml"),
    indexFile: join(evidenceRoot, "_index.md"),
    verifiedFactsFile: join(qaDir, "verified-facts.md"),
    qaAnswersFile: join(qaDir, "answers.md"),
    checkpointFile: join(appliedDir, "checkpoint.json"),
    manifestFile: join(appliedDir, "manifest.yaml"),
  };
}

export function createEvidenceId(nowIso: string, sourceType: string, label: string, existingIds: string[]): string {
  const date = nowIso.slice(0, 10);
  const base = `${date}-${sourceType}-${slugify(label)}`;
  let candidate = base;
  let counter = 2;
  while (existingIds.includes(candidate)) {
    candidate = `${base}-${counter}`;
    counter += 1;
  }
  return candidate;
}

export function listEvidenceIds(paths: RuntimePaths, workspace: string): string[] {
  const sourcesRoot = join(paths.workspacesDir, workspace, "evidence", "sources");
  if (!existsSync(sourcesRoot)) {
    return [];
  }

  return readdirSync(sourcesRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function fingerprintSourceRecord(record: Omit<EvidenceSourceRecord, "source_sha256">): string {
  return sha256(YAML.dump(record, { noRefs: true }));
}

export function buildSourceRecord(input: Omit<EvidenceSourceRecord, "raw_sha256" | "source_sha256"> & { rawContent: string }): EvidenceSourceRecord {
  const rawSha = sha256(input.rawContent);
  const baseRecord = {
    id: input.id,
    type: input.type,
    origin: input.origin,
    label: input.label,
    workspace_at_ingest: input.workspace_at_ingest,
    ingested_at: input.ingested_at,
    state: input.state,
    raw_filename: input.raw_filename,
    raw_sha256: rawSha,
  };

  return {
    ...baseRecord,
    source_sha256: fingerprintSourceRecord(baseRecord),
  };
}

export function ensureEvidenceDirectories(locations: EvidenceLocationSet): void {
  ensureDir(locations.sourceDir);
  ensureDir(locations.analysisDir);
  ensureDir(locations.qaDir);
  ensureDir(locations.appliedDir);
  ensureDir(locations.archiveDir);
}

export interface VerifiedFactRecord {
  statement: string;
  questionId: string;
  wikiPath: string;
}

export function verifiedFactsMarkdown(facts: VerifiedFactRecord[]): string {
  const lines = ["# Verified Facts", ""];
  for (const fact of facts) {
    lines.push(`- ${fact.statement}`);
    lines.push(`  - question_id: ${fact.questionId}`);
    lines.push(`  - wiki_path: ${fact.wikiPath}`);
  }
  lines.push("");
  return lines.join("\n");
}

export function qaAnswersMarkdown(questions: EvidenceQuestion[]): string {
  const lines = [
    "# Evidence QA Answers",
    "",
    "| question_id | severity | status |",
    "| --- | --- | --- |",
  ];
  for (const question of questions) {
    lines.push(`| ${question.id} | ${question.severity} | ${question.status} |`);
  }
  lines.push("");
  return lines.join("\n");
}

export function parseQaAnswers(markdown: string): EvidenceQuestion[] {
  const questions: EvidenceQuestion[] = [];
  for (const line of markdown.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("|")) continue;
    const cells = trimmed.split("|").slice(1, -1).map((cell) => cell.trim());
    if (cells.length < 3) continue;
    const [id, severity, status] = cells;
    if (!questionSeverities.includes(severity as QuestionSeverity)) continue;
    if (!questionStatuses.includes(status as QuestionStatus)) continue;
    questions.push({ id, severity: severity as QuestionSeverity, status: status as QuestionStatus });
  }
  return questions;
}

export function assertWorkspaceTarget(workspaceRoot: string, relativePath: string): string {
  const target = join(workspaceRoot, relativePath);
  if (!pathInside(workspaceRoot, target)) {
    throw new Error(`Workspace isolation violation: ${relativePath}`);
  }
  return target;
}
