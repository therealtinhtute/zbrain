import { describe, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { applyEvidence } from "../../src/core/evidence-apply";
import { reviewEvidence } from "../../src/core/evidence-review";
import { ingestEvidence } from "../../src/core/evidence-ingest";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";
import { initDb } from "../../src/core/db";
import { readEvidence } from "../../src/core/db-evidence";

function createWorkspaceFixture() {
  const root = mkdtempSync(join(tmpdir(), "zbrain-evidence-"));
  const runtimeDir = join(root, "runtime");
  const workspaceRoot = join(runtimeDir, "workspaces", "programming");
  mkdirSync(join(workspaceRoot, "axioms"), { recursive: true });
  mkdirSync(join(workspaceRoot, "mental-models"), { recursive: true });
  mkdirSync(join(workspaceRoot, "projects"), { recursive: true });
  mkdirSync(join(workspaceRoot, "decisions"), { recursive: true });
  mkdirSync(join(workspaceRoot, "evidence"), { recursive: true });
  const db = initDb(runtimeDir);
  return { root, paths: resolveRuntimePaths({ cwd: root, runtimeDir }), db };
}

describe("evidence pipeline", () => {
  test("ingest creates immutable source files and an evidence index row", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "SOLID note",
        rawContent: "Single responsibility principle",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      expect(existsSync(ingested.rawFile)).toBe(true);
      const row = readEvidence(fixture.db, "programming", ingested.evidenceId);
      expect(row?.workspace_at_ingest).toBe("programming");
      expect(row?.state).toBe("ingested");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("review writes verified-facts.md and transitions to reviewed", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Refactoring note",
        rawContent: "Prefer small reversible changes.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      reviewEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
        facts: [
          {
            statement: "Prefer small reversible changes.",
            questionId: "q-1",
            wikiPath: "axioms/reversible-changes.md",
          },
        ],
      });

      const row = readEvidence(fixture.db, "programming", ingested.evidenceId);
      expect(row?.state).toBe("reviewed");
      expect(
        readFileSync(join(fixture.paths.workspacesDir, "programming", "evidence", "qa", ingested.evidenceId, "verified-facts.md"), "utf8"),
      ).toContain("question_id: q-1");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("apply resumes after interruption and triggers reindex", () => {
    const fixture = createWorkspaceFixture();
    const reindexed: string[] = [];

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Resume note",
        rawContent: "Checkpoint every mutation.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      reviewEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
        facts: [{ statement: "Checkpoint every mutation.", questionId: "q-1", wikiPath: "axioms/checkpoints.md" }],
      });

      expect(() =>
        applyEvidence(fixture.db, fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          nowIso: "2026-05-25T02:10:00.000Z",
          mutations: [
            {
              relativePath: "axioms/checkpoints.md",
              content: "# Checkpoints\n",
              citations: [{ questionId: "q-1", wikiPath: "axioms/checkpoints.md" }],
            },
            {
              relativePath: "projects/resume.md",
              content: "# Resume\n",
              citations: [{ questionId: "q-1", wikiPath: "projects/resume.md" }],
            },
          ],
          failAfterMutations: 1,
        }),
      ).toThrow("Simulated apply interruption");

      const resumed = applyEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:15:00.000Z",
        mutations: [
          {
            relativePath: "axioms/checkpoints.md",
            content: "# Checkpoints\n",
            citations: [{ questionId: "q-1", wikiPath: "axioms/checkpoints.md" }],
          },
          {
            relativePath: "projects/resume.md",
            content: "# Resume\n",
            citations: [{ questionId: "q-1", wikiPath: "projects/resume.md" }],
          },
        ],
        reindex: (workspace) => reindexed.push(workspace),
      });

      expect(resumed.applied).toEqual(["axioms/checkpoints.md", "projects/resume.md"]);
      expect(reindexed).toEqual(["programming"]);
      expect(readFileSync(join(fixture.paths.workspacesDir, "programming", "projects", "resume.md"), "utf8")).toBe("# Resume\n");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("apply rejects blocked QA and wrong workspace", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Guard note",
        rawContent: "Never bypass QA.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      // Review persists a blocking P0 (awaiting_external); the gate must read it from
      // disk at apply time — no caller can fabricate an "answered" override anymore.
      reviewEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
        facts: [{ statement: "Never bypass QA.", questionId: "q-1", wikiPath: "axioms/guard.md" }],
        questions: [{ id: "q-1", severity: "P0", status: "awaiting_external" }],
      });

      expect(() =>
        applyEvidence(fixture.db, fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          mutations: [
            {
              relativePath: "axioms/guard.md",
              content: "# Guard\n",
              citations: [{ questionId: "q-1", wikiPath: "axioms/guard.md" }],
            },
          ],
        }),
      ).toThrow("QA gate blocked");

      expect(() =>
        applyEvidence(fixture.db, fixture.paths, {
          workspace: "finance",
          evidenceId: ingested.evidenceId,
          mutations: [
            {
              relativePath: "axioms/guard.md",
              content: "# Guard\n",
              citations: [{ questionId: "q-1", wikiPath: "axioms/guard.md" }],
            },
          ],
        }),
      ).toThrow();
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("re-review of a reviewed item is rejected before clobbering verified facts (ISSUE-006)", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Reentry note",
        rawContent: "Review runs once.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      reviewEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
        facts: [{ statement: "First fact.", questionId: "q-1", wikiPath: "axioms/reentry.md" }],
      });

      // State is now "reviewed"; the state machine forbids reviewed -> reviewed.
      // The guard must fire BEFORE verified-facts.md is overwritten.
      expect(() =>
        reviewEvidence(fixture.db, fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          nowIso: "2026-05-25T02:06:00.000Z",
          facts: [{ statement: "Second fact.", questionId: "q-1", wikiPath: "axioms/reentry.md" }],
        }),
      ).toThrow("Invalid evidence transition");

      const verified = readFileSync(
        join(fixture.paths.workspacesDir, "programming", "evidence", "qa", ingested.evidenceId, "verified-facts.md"),
        "utf8",
      );
      expect(verified).toContain("First fact.");
      expect(verified).not.toContain("Second fact.");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("apply rejects evidence that was never reviewed without writing wiki files (ISSUE-006)", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.db, fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Unreviewed note",
        rawContent: "Apply must follow review.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      // No review — state is still "ingested"; apply must refuse before any write.
      expect(() =>
        applyEvidence(fixture.db, fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          mutations: [
            {
              relativePath: "axioms/unreviewed.md",
              content: "# Unreviewed\n",
              citations: [{ questionId: "q-1", wikiPath: "axioms/unreviewed.md" }],
            },
          ],
        }),
      ).toThrow("Invalid evidence transition");

      expect(
        existsSync(join(fixture.paths.workspacesDir, "programming", "axioms", "unreviewed.md")),
      ).toBe(false);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });
});
