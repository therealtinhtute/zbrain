import { writeTextFile, readTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import {
  evidenceLocations,
  parseSourceRecord,
  updateEvidenceIndex,
  verifiedFactsMarkdown,
  type VerifiedFactRecord,
} from "./evidence-store";
import {
  assertCitationCoverage,
  assertValidEvidenceTransition,
  assertWorkspaceLock,
  type EvidenceQuestion,
} from "./evidence-state";

export interface CompleteEvidenceQaOptions {
  workspace: string;
  evidenceId: string;
  questions: EvidenceQuestion[];
  facts: VerifiedFactRecord[];
  nowIso?: string;
}

function qaStateFromQuestions(questions: EvidenceQuestion[]): "qa_in_progress" | "qa_awaiting_external" | "qa_done" {
  if (questions.some((question) => question.status === "awaiting_external" || question.status === "deferred")) {
    return "qa_awaiting_external";
  }
  if (questions.some((question) => question.status === "open")) {
    return "qa_in_progress";
  }
  return "qa_done";
}

export function completeEvidenceQa(paths: RuntimePaths, options: CompleteEvidenceQaOptions): { state: string } {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const locations = evidenceLocations(paths, options.workspace, options.evidenceId);
  const sourceRecord = parseSourceRecord(readTextFile(locations.sourceFile));

  assertWorkspaceLock(sourceRecord.workspace_at_ingest, options.workspace);
  assertValidEvidenceTransition("analyzed", qaStateFromQuestions(options.questions));
  assertCitationCoverage(
    options.facts.map((fact) => ({ questionId: fact.questionId, wikiPath: fact.wikiPath })),
  );

  const answersLines = ["# Evidence QA Answers", "", "| question_id | severity | status | answer |", "| --- | --- | --- | --- |"];
  for (const question of options.questions) {
    answersLines.push(`| ${question.id} | ${question.severity} | ${question.status} | recorded |`);
  }
  answersLines.push("");

  writeTextFile(locations.qaAnswersFile, answersLines.join("\n"), { overwrite: true });
  writeTextFile(locations.verifiedFactsFile, verifiedFactsMarkdown(options.facts), { overwrite: true });

  const state = qaStateFromQuestions(options.questions);
  updateEvidenceIndex(locations.indexFile, options.evidenceId, state, nowIso);
  return { state };
}
