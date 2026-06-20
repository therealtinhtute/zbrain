import type { Database } from "bun:sqlite";
import { readTextFile, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import { evidenceLocations, qaAnswersMarkdown, verifiedFactsMarkdown, type VerifiedFactRecord } from "./evidence-store";
import { readEvidence, verifyEvidenceIntegrity, updateEvidenceState } from "./db-evidence";
import { assertCitationCoverage, assertValidEvidenceTransition, assertWorkspaceLock, type EvidenceQuestion, type EvidenceState } from "./evidence-state";

export interface ReviewEvidenceOptions {
  workspace: string;
  evidenceId: string;
  facts: VerifiedFactRecord[];
  questions?: EvidenceQuestion[];
  nowIso?: string;
}

function answeredQuestionsFromFacts(facts: VerifiedFactRecord[]): EvidenceQuestion[] {
  const byId = new Map<string, EvidenceQuestion>();
  for (const fact of facts) {
    if (!byId.has(fact.questionId)) {
      byId.set(fact.questionId, { id: fact.questionId, severity: "P0", status: "answered" });
    }
  }
  return [...byId.values()];
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
  assertValidEvidenceTransition(row.state as EvidenceState, "reviewed");
  assertCitationCoverage(
    options.facts.map((fact) => ({ questionId: fact.questionId, wikiPath: fact.wikiPath })),
  );

  const questions = options.questions ?? answeredQuestionsFromFacts(options.facts);
  writeTextFile(locations.verifiedFactsFile, verifiedFactsMarkdown(options.facts), { overwrite: true });
  writeTextFile(locations.qaAnswersFile, qaAnswersMarkdown(questions), { overwrite: true });
  updateEvidenceState(db, options.workspace, options.evidenceId, "reviewed", nowIso);
}
