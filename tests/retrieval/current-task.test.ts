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

  test("single-workspace output has no Workspace column (backward compat)", () => {
    const markdown = generateCurrentTaskMarkdown({
      query: "clean arch",
      workspace: "programming",
      results: [{ path: "/ws/programming/axioms/solid.md", score: 5, snippet: "solid", tier: "P0" }],
    });
    expect(markdown).not.toContain("| Score | Workspace |");
    expect(markdown).toContain("| Score | File | Preview |");
    expect(markdown).toContain("### /ws/programming/axioms/solid.md (P0)");
    expect(markdown).not.toContain("[programming]");
  });

  test("multi-workspace output shows Workspace column and section labels", () => {
    const markdown = generateCurrentTaskMarkdown({
      query: "file-storage @research",
      workspace: "ttdvkh",
      secondaryWorkspaces: ["framework-core"],
      results: [
        { path: "/ws/ttdvkh/axioms/arch.md", score: 8, snippet: "arch", tier: "P0" },
        { path: "/ws/framework-core/projects/starter.md", score: 5, snippet: "starter", tier: "P2", workspace: "framework-core" },
      ],
    });
    expect(markdown).toContain("Secondary Workspaces: framework-core");
    expect(markdown).toContain("| Score | Workspace | File | Preview |");
    expect(markdown).toContain("### [ttdvkh] /ws/ttdvkh/axioms/arch.md (P0)");
    expect(markdown).toContain("### [framework-core] /ws/framework-core/projects/starter.md (P2)");
  });

  test("writes current-task.md into the central runtime project state directory", () => {
    const root = mkdtempSync(join(tmpdir(), "zbrain-current-task-"));

    try {
      const paths = resolveRuntimePaths({ cwd: root, runtimeDir: join(root, "runtime") });
      const filePath = writeCurrentTask(paths, "# Wiki Context");

      expect(filePath).toBe(currentTaskFilePath(paths));
      expect(filePath.startsWith(join(root, "runtime", "projects"))).toBe(true);
      expect(readFileSync(filePath, "utf8")).toBe("# Wiki Context");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
