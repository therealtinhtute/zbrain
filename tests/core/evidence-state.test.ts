import { describe, expect, test } from "bun:test";
import {
  assertCitationCoverage,
  assertImmutableSourceSnapshot,
  assertValidEvidenceTransition,
  assertWorkspaceLock,
  validateQAGate,
} from "../../src/core/evidence-state";

describe("evidence state guards", () => {
  test("allows valid evidence transitions", () => {
    expect(() => assertValidEvidenceTransition("ingested", "analyzed")).not.toThrow();
    expect(() => assertValidEvidenceTransition("qa_done", "applied")).not.toThrow();
  });

  test("rejects invalid evidence transitions", () => {
    expect(() => assertValidEvidenceTransition("ingested", "applied")).toThrow();
  });

  test("enforces the workspace lock", () => {
    expect(() => assertWorkspaceLock("programming", "programming")).not.toThrow();
    expect(() => assertWorkspaceLock("programming", "finance")).toThrow();
  });

  test("blocks QA when P0 or P1 questions remain unresolved", () => {
    expect(() =>
      validateQAGate([
        { id: "q-1", severity: "P0", status: "awaiting_external" },
        { id: "q-2", severity: "P2", status: "deferred" },
      ]),
    ).toThrow();

    expect(() =>
      validateQAGate([
        { id: "q-3", severity: "P2", status: "deferred" },
        { id: "q-4", severity: "P3", status: "answered" },
      ]),
    ).not.toThrow();
  });

  test("requires citations and immutable source snapshots", () => {
    expect(() =>
      assertCitationCoverage([{ questionId: "q-1", wikiPath: "axioms/fact.md" }]),
    ).not.toThrow();
    expect(() =>
      assertCitationCoverage([{ questionId: "", wikiPath: "axioms/fact.md" }]),
    ).toThrow();

    expect(() =>
      assertImmutableSourceSnapshot(
        { rawContent: "source", sourceYaml: "workspace: programming" },
        { rawContent: "source", sourceYaml: "workspace: programming" },
      ),
    ).not.toThrow();
    expect(() =>
      assertImmutableSourceSnapshot(
        { rawContent: "source", sourceYaml: "workspace: programming" },
        { rawContent: "edited", sourceYaml: "workspace: programming" },
      ),
    ).toThrow();
  });
});
