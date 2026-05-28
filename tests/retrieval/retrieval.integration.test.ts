import { describe, expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { retrieveWorkspaceContext, retrieveMultiWorkspaceContext } from "../../src/core/retrieval";
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

describe("retrieveMultiWorkspaceContext", () => {
  function makeEnv(workspaceNames: string[]) {
    const root = mkdtempSync(join(tmpdir(), "zbrain-mw-"));
    const runtimeDir = join(root, "runtime");
    const workspacesDir = join(runtimeDir, "workspaces");
    for (const name of workspaceNames) {
      mkdirSync(join(workspacesDir, name), { recursive: true });
    }
    const paths = resolveRuntimePaths({ cwd: root, runtimeDir });
    return {
      root,
      paths,
      workspacesDir,
      cleanup: () => rmSync(root, { recursive: true, force: true }),
    };
  }

  test("no secondaries configured → identical to single-workspace", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh"]);
    const called: string[] = [];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "clean architecture", secondaries: [], workspacesDir },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return [{ path: "/ws/ttdvkh/axioms/arch.md", score: 5, snippet: "arch", body: "body" }];
          },
        },
      );
      expect(called).toEqual(["ttdvkh"]);
      expect(context.results).toHaveLength(1);
      expect(context.markdown).toContain("Workspace: ttdvkh");
      expect(context.markdown).not.toContain("Secondary Workspaces");
      expect(context.markdown).not.toContain("| Score | Workspace |");
    } finally {
      cleanup();
    }
  });

  test("keyword trigger → secondary workspace queried", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core"]);
    const called: string[] = [];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        {
          primaryWorkspace: "ttdvkh",
          query: "file-storage starter setup",
          secondaries: [{ workspace: "framework-core", keywords: ["file-storage"], limit: 3 }],
          workspacesDir,
        },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return [{ path: `/ws/${workspace}/projects/p.md`, score: 5, snippet: "s", body: "b" }];
          },
        },
      );
      expect(called).toContain("ttdvkh");
      expect(called).toContain("framework-core");
      const fwResult = context.results.find((r) => r.workspace === "framework-core");
      expect(fwResult).toBeDefined();
    } finally {
      cleanup();
    }
  });

  test("@tag → tag stripped from BM25 query, secondary workspace queried", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "research"]);
    const queriesSeen: string[] = [];
    try {
      retrieveMultiWorkspaceContext(
        paths,
        {
          primaryWorkspace: "ttdvkh",
          query: "design pattern @research",
          secondaries: [],
          workspacesDir,
        },
        {
          searchWorkspace: ({ workspace, query }) => {
            queriesSeen.push(`${workspace}:${query}`);
            return [];
          },
        },
      );
      expect(queriesSeen.some((q) => q.startsWith("research:"))).toBe(true);
      expect(queriesSeen.every((q) => !q.includes("@research"))).toBe(true);
    } finally {
      cleanup();
    }
  });

  test("primary fills all slots → no secondary results included", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core"]);
    const called: string[] = [];
    const primaryResults = Array.from({ length: 8 }, (_, i) => ({
      path: `/ws/ttdvkh/axioms/a${i}.md`,
      score: 10 - i,
      snippet: `s${i}`,
      body: `b${i}`,
    }));
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        {
          primaryWorkspace: "ttdvkh",
          query: "file-storage",
          secondaries: [{ workspace: "framework-core", keywords: ["file-storage"], limit: 3 }],
          workspacesDir,
          limit: 8,
        },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return workspace === "ttdvkh" ? primaryResults : [{ path: "/ws/fw/p.md", score: 3, snippet: "s", body: "b" }];
          },
        },
      );
      expect(context.results).toHaveLength(8);
      expect(context.results.every((r) => r.workspace === undefined)).toBe(true);
    } finally {
      cleanup();
    }
  });
});
