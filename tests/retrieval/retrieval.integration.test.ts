import { describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { retrieveWorkspaceContext } from "../../src/core/retrieval";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

describe("retrieval integration", () => {
  test("writes ranked context for a single workspace query", () => {
    const root = mkdtempSync(join(tmpdir(), "zbrain-retrieval-"));

    try {
      const paths = resolveRuntimePaths({ cwd: root, runtimeDir: join(root, "runtime") });
      const context = retrieveWorkspaceContext(
        paths,
        { workspace: "programming", query: "solid design" },
        {
          searchWorkspace: ({ workspace }) => {
            expect(workspace).toBe("programming");
            return [
              {
                path: "/ws/programming/projects/refactoring.md",
                score: 7,
                snippet: "refactoring",
                body: "Project context",
              },
              {
                path: "/ws/programming/axioms/solid.md",
                score: 2,
                snippet: "solid",
                body: "Axiom context",
              },
            ];
          },
        },
      );

      expect(context.results[0]?.path).toContain("/axioms/");
      expect(readFileSync(context.filePath, "utf8")).toContain("Workspace: programming");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("reports knowledge gaps when the workspace query returns nothing", () => {
    const root = mkdtempSync(join(tmpdir(), "zbrain-retrieval-"));

    try {
      const paths = resolveRuntimePaths({ cwd: root, runtimeDir: join(root, "runtime") });
      const context = retrieveWorkspaceContext(
        paths,
        { workspace: "finance", query: "yield curve" },
        { searchWorkspace: () => [] },
      );

      expect(context.markdown).toContain("No documents matched the active workspace query.");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
