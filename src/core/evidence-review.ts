import type { Database } from "bun:sqlite";
import { readTextFile, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import { evidenceLocations, verifiedFactsMarkdown, type VerifiedFactRecord } from "./evidence-store";
import { readEvidence, verifyEvidenceIntegrity, updateEvidenceState } from "./db-evidence";
import { assertCitationCoverage, assertValidEvidenceTransition, assertWorkspaceLock } from "./evidence-state";

export interface ReviewEvidenceOptions {
  workspace: string;
  evidenceId: string;
  facts: VerifiedFactRecord[];
  nowIso?: string;
}

export function reviewEvidence(
  db: Database,
  paths: RuntimePaths,
  options: ReviewEvidenceOptions,
): void {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const locations = evidenceLocations(paths, options.workspace, options.evidenceId);
  const rawContent = readTextFile(locations.rawFile);
  const row = readEvidence(db, options.workspace, options.evidenceId);

  if (!row) {
    throw new Error(`Evidence not found: ${options.evidenceId} in workspace ${options.workspace}`);
  }

  verifyEvidenceIntegrity(db, options.workspace, options.evidenceId, rawContent);
  assertWorkspaceLock(row.workspace_at_ingest, options.workspace);
  assertValidEvidenceTransition("ingested", "reviewed");
  assertCitationCoverage(
    options.facts.map((fact) => ({ questionId: fact.questionId, wikiPath: fact.wikiPath })),
  );

  writeTextFile(locations.verifiedFactsFile, verifiedFactsMarkdown(options.facts), { overwrite: true });
  updateEvidenceState(db, options.workspace, options.evidenceId, "reviewed", nowIso);
}
