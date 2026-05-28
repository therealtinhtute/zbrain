import { describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runInit } from "../../src/commands/init";
import { runSetup } from "../../src/commands/setup";
import type { CommandUi } from "../../src/commands/ui";
import { analyzeEvidence } from "../../src/core/evidence-analyze";
import { applyEvidence } from "../../src/core/evidence-apply";
import { ingestEvidence } from "../../src/core/evidence-ingest";
import { completeEvidenceQa } from "../../src/core/evidence-qa";
import { retrieveWorkspaceContext } from "../../src/core/retrieval";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

class FakeUi implements CommandUi {
  confirms: boolean[] = [];
  selects: string[] = [];
  multiselects: string[][] = [];
  intro(): void {}
  outro(): void {}
  info(): void {}
  note(): void {}
  spinner() {
    return { start() {}, stop() {} };
  }
  async confirm(): Promise<boolean> {
    return this.confirms.shift() ?? true;
  }
  async text(): Promise<string> {
    throw new Error("text() not expected in acceptance tests");
  }
  async select(): Promise<string> {
    const value = this.selects.shift();
    if (!value) throw new Error("Missing fake select value");
    return value;
  }
  async multiselect(): Promise<string[]> {
    return this.multiselects.shift() ?? [];
  }
}

describe("release acceptance path", () => {
  test("covers setup, init, learn, and ask for a seeded workspace", async () => {
    const root = mkdtempSync(join(tmpdir(), "zbrain-release-"));
    const homeDir = join(root, "home");
    const projectDir = join(root, "project");
    mkdirSync(homeDir, { recursive: true });
    mkdirSync(projectDir, { recursive: true });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: projectDir, homeDir } });

      const initUi = new FakeUi();
      initUi.selects = ["programming"];
      initUi.multiselects = [["claude_rules", "commands", "agents", "mcp"]];
      await runInit({ ui: initUi, pathOptions: { cwd: projectDir, homeDir } });

      const paths = resolveRuntimePaths({ cwd: projectDir, homeDir });
      const ingested = ingestEvidence(paths, {
        workspace: "programming",
        sourceType: "paste",
        origin: "inline",
        label: "Acceptance note",
        rawContent: "Prefer small reversible changes when editing production systems.",
        nowIso: "2026-05-25T03:10:00.000Z",
      });
      analyzeEvidence(paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T03:11:00.000Z",
      });
      completeEvidenceQa(paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T03:12:00.000Z",
        questions: [{ id: "q-1", severity: "P0", status: "answered" }],
        facts: [
          {
            statement: "Prefer small reversible changes when editing production systems.",
            questionId: "q-1",
            wikiPath: "axioms/reversible-changes.md",
          },
        ],
      });

      applyEvidence(paths, {
        workspace: "programming",
        evidenceId: ingested.evidenceId,
        nowIso: "2026-05-25T03:13:00.000Z",
        questions: [{ id: "q-1", severity: "P0", status: "answered" }],
        mutations: [
          {
            relativePath: "axioms/reversible-changes.md",
            content: "# Reversible Changes\n\nPrefer small reversible changes when editing production systems.\n",
            citations: [{ questionId: "q-1", wikiPath: "axioms/reversible-changes.md" }],
          },
        ],
      });

      const retrieval = retrieveWorkspaceContext(
        paths,
        { workspace: "programming", query: "reversible changes" },
        {
          searchWorkspace: () => [
            {
              path: join(paths.workspacesDir, "programming", "axioms", "reversible-changes.md"),
              score: 12,
              snippet: "Prefer small reversible changes",
              body: readFileSync(
                join(paths.workspacesDir, "programming", "axioms", "reversible-changes.md"),
                "utf8",
              ),
            },
          ],
        },
      );

      expect(readFileSync(join(projectDir, ".claude", "zbrain.json"), "utf8")).toContain("\"workspace\": \"programming\"");
      expect(readFileSync(join(paths.workspacesDir, "programming", "axioms", "reversible-changes.md"), "utf8")).toContain("Reversible Changes");
      expect(retrieval.markdown).toContain("Prefer small reversible changes");
      expect(readFileSync(join(projectDir, ".claude", "context", "current-task.md"), "utf8")).toContain("Workspace: programming");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
