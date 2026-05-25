import { describe, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { applyEvidence } from "../../src/core/evidence-apply";
import { analyzeEvidence } from "../../src/core/evidence-analyze";
import { ingestEvidence } from "../../src/core/evidence-ingest";
import { completeEvidenceQa } from "../../src/core/evidence-qa";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

function createWorkspaceFixture() {
  const root = mkdtempSync(join(tmpdir(), "zbrain-evidence-"));
  const runtimeDir = join(root, "runtime");
  const workspaceRoot = join(runtimeDir, "workspaces", "programming");
  mkdirSync(join(workspaceRoot, "axioms"), { recursive: true });
  mkdirSync(join(workspaceRoot, "mental-models"), { recursive: true });
  mkdirSync(join(workspaceRoot, "projects"), { recursive: true });
  mkdirSync(join(workspaceRoot, "decisions"), { recursive: true });
  mkdirSync(join(workspaceRoot, "evidence"), { recursive: true });
  return { root, paths: resolveRuntimePaths({ cwd: root, runtimeDir }) };
}

describe("evidence pipeline", () => {
  test("ingest creates immutable source files and an evidence index row", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "SOLID note",
        rawContent: "Single responsibility principle",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      expect(existsSync(ingested.rawFile)).toBe(true);
      expect(readFileSync(ingested.sourceFile, "utf8")).toContain("workspace_at_ingest: programming");
      expect(
        readFileSync(join(fixture.paths.workspacesDir, "programming", "evidence", "_index.md"), "utf8"),
      ).toContain("| 2026-05-25-paste-solid-note | ingested | 2026-05-25T02:00:00.000Z |");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("analyze and qa create deterministic artifacts and qa state", () => {
    const fixture = createWorkspaceFixture();

    try {
      const ingested = ingestEvidence(fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Refactoring note",
        rawContent: "Prefer small reversible changes.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      const analysisFiles = analyzeEvidence(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
      });
      const qa = completeEvidenceQa(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:10:00.000Z",
        questions: [
          { id: "q-1", severity: "P0", status: "answered" },
          { id: "q-2", severity: "P2", status: "answered" },
        ],
        facts: [
          {
            statement: "Prefer small reversible changes.",
            questionId: "q-1",
            wikiPath: "axioms/reversible-changes.md",
          },
        ],
      });

      expect(analysisFiles).toHaveLength(4);
      expect(qa.state).toBe("qa_done");
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
      const ingested = ingestEvidence(fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Resume note",
        rawContent: "Checkpoint every mutation.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });
      analyzeEvidence(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
      });
      completeEvidenceQa(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:10:00.000Z",
        questions: [{ id: "q-1", severity: "P0", status: "answered" }],
        facts: [{ statement: "Checkpoint every mutation.", questionId: "q-1", wikiPath: "axioms/checkpoints.md" }],
      });

      expect(() =>
        applyEvidence(fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          nowIso: "2026-05-25T02:15:00.000Z",
          questions: [{ id: "q-1", severity: "P0", status: "answered" }],
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

      const resumed = applyEvidence(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:20:00.000Z",
        questions: [{ id: "q-1", severity: "P0", status: "answered" }],
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
      const ingested = ingestEvidence(fixture.paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Guard note",
        rawContent: "Never bypass QA.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });
      analyzeEvidence(fixture.paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T02:05:00.000Z",
      });

      expect(() =>
        applyEvidence(fixture.paths, {
          workspace: "programming",
          evidenceId: ingested.evidenceId,
          questions: [{ id: "q-1", severity: "P0", status: "awaiting_external" }],
          mutations: [
            {
              relativePath: "axioms/guard.md",
              content: "# Guard\n",
              citations: [{ questionId: "q-1", wikiPath: "axioms/guard.md" }],
            },
          ],
        }),
      ).toThrow();

      expect(() =>
        applyEvidence(fixture.paths, {
          workspace: "finance",
          evidenceId: ingested.evidenceId,
          questions: [{ id: "q-1", severity: "P0", status: "answered" }],
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
});
