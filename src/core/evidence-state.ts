export const evidenceStates = [
  "ingested",
  "reviewed",
  "applied",
  "archived",
] as const;

export type EvidenceState = (typeof evidenceStates)[number];
export type QuestionSeverity = "P0" | "P1" | "P2" | "P3";
export type QuestionStatus = "open" | "answered" | "awaiting_external" | "deferred";

export interface EvidenceQuestion {
  id: string;
  severity: QuestionSeverity;
  status: QuestionStatus;
}

export interface EvidenceCitation {
  questionId: string;
  wikiPath: string;
}

export interface SourceSnapshot {
  rawContent: string;
  sourceYaml: string;
}

const transitionMap: Record<EvidenceState, EvidenceState[]> = {
  ingested: ["reviewed"],
  reviewed: ["applied"],
  applied: ["archived"],
  archived: [],
};

export function assertValidEvidenceTransition(from: EvidenceState, to: EvidenceState): void {
  if (!transitionMap[from].includes(to)) {
    throw new Error(`Invalid evidence transition: ${from} -> ${to}`);
  }
}

export function assertWorkspaceLock(expectedWorkspace: string, activeWorkspace: string): void {
  if (expectedWorkspace !== activeWorkspace) {
    throw new Error(
      `Workspace lock violation: evidence belongs to "${expectedWorkspace}" but active workspace is "${activeWorkspace}"`,
    );
  }
}

export function validateQAGate(questions: EvidenceQuestion[]): void {
  const blocking = questions.filter(
    (question) =>
      (question.severity === "P0" || question.severity === "P1") &&
      (question.status === "awaiting_external" || question.status === "deferred"),
  );

  if (blocking.length > 0) {
    throw new Error(
      `QA gate blocked by unresolved questions: ${blocking.map((question) => question.id).join(", ")}`,
    );
  }
}

export function assertCitationCoverage(citations: EvidenceCitation[]): void {
  for (const citation of citations) {
    if (citation.questionId.trim().length === 0 || citation.wikiPath.trim().length === 0) {
      throw new Error("Citation coverage violation: every citation needs questionId and wikiPath");
    }
  }
}

export function assertImmutableSourceSnapshot(
  originalSnapshot: SourceSnapshot,
  currentSnapshot: SourceSnapshot,
): void {
  if (
    originalSnapshot.rawContent !== currentSnapshot.rawContent ||
    originalSnapshot.sourceYaml !== currentSnapshot.sourceYaml
  ) {
    throw new Error("Immutable source violation: raw.md and source.yaml cannot change after ingest");
  }
}
