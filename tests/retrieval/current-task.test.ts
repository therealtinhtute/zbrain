import { describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { currentTaskFilePath, generateCurrentTaskMarkdown, writeCurrentTask } from "../../src/core/current-task";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

describe("current-task generation", () => {
  test("generates markdown with omitted empty tiers and explicit gaps", () => {
    const markdown = generateCurrentTaskMarkdown({
      query: "solid design",
      workspace: "programming",
      results: [
        {
          path: "/ws/programming/axioms/solid.md",
          score: 12,
          snippet: "solid",
          body: "Axiom body",
          tier: "P0",
        },
        {
          path: "/ws/programming/projects/book.md",
          score: 4,
          snippet: "book",
          body: "Project body",
          tier: "P2",
        },
      ],
    });

    expect(markdown).toContain("### P0 - Axioms");
    expect(markdown).not.toContain("### P1 - Mental Models\n|");
    expect(markdown).toContain("### /ws/programming/axioms/solid.md (P0)");
    expect(markdown).toContain("No Mental Models results (P1) for this query.");
  });

  test("writes current-task.md into the project .claude/context directory", () => {
    const root = mkdtempSync(join(tmpdir(), "zbrain-current-task-"));

    try {
      const paths = resolveRuntimePaths({ cwd: root, runtimeDir: join(root, "runtime") });
      const filePath = writeCurrentTask(paths, "# Wiki Context");

      expect(filePath).toBe(currentTaskFilePath(paths));
      expect(readFileSync(filePath, "utf8")).toBe("# Wiki Context");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
