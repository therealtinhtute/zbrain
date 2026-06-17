import { describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runInit } from "../../src/commands/init";
import { runSetup } from "../../src/commands/setup";
import { runWorkspaceCreate } from "../../src/commands/workspace";
import { runLearn } from "../../src/commands/learn";
import type { CommandUi } from "../../src/commands/ui";
import { applyEvidence } from "../../src/core/evidence-apply";
import { reviewEvidence } from "../../src/core/evidence-review";
import { retrieveWorkspaceContext } from "../../src/core/retrieval";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";
import { openDb } from "../../src/core/db";
import { readProject } from "../../src/core/db-projects";

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
      await runWorkspaceCreate("programming", {
        ui: new FakeUi(),
        pathOptions: { cwd: projectDir, homeDir },
        nowIso: "2026-05-25T03:00:00.000Z",
      });

      const initUi = new FakeUi();
      initUi.selects = ["programming"];
      initUi.multiselects = [["claude_rules", "skills", "agents", "mcp"]];
      await runInit({ ui: initUi, pathOptions: { cwd: projectDir, homeDir } });

      const paths = resolveRuntimePaths({ cwd: projectDir, homeDir });
      await runLearn({
        ui: new FakeUi(),
        pathOptions: { cwd: projectDir, homeDir },
        workspace: "programming",
        type: "paste",
        origin: "inline",
        label: "Acceptance note",
        rawContent: "Prefer small reversible changes when editing production systems.",
        nowIso: "2026-05-25T03:10:00.000Z",
      });
      const evidenceId = "2026-05-25-paste-acceptance-note";
      const db = openDb(paths.runtimeDir);
      reviewEvidence(db, paths, {
        workspace: "programming",
        evidenceId,
        nowIso: "2026-05-25T03:11:00.000Z",
        facts: [
          {
            statement: "Prefer small reversible changes when editing production systems.",
            questionId: "q-1",
            wikiPath: "axioms/reversible-changes.md",
          },
        ],
      });

      applyEvidence(db, paths, {
        workspace: "programming",
        evidenceId,
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

      expect(readProject(db, projectDir)?.workspace).toBe("programming");
      expect(readFileSync(join(paths.workspacesDir, "programming", "axioms", "reversible-changes.md"), "utf8")).toContain("Reversible Changes");
      expect(retrieval.markdown).toContain("Prefer small reversible changes");
      expect(readFileSync(retrieval.filePath, "utf8")).toContain("Workspace: programming");
      expect(retrieval.filePath.startsWith(join(homeDir, ".zbrain", "projects"))).toBe(true);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
