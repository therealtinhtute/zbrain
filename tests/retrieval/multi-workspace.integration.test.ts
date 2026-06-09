import { describe, expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { retrieveMultiWorkspaceContext } from "../../src/core/retrieval";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";
import { type SecondaryWorkspaceEntry } from "../../src/schemas/config";

function makeEnv(workspaceNames: string[]) {
  const root = mkdtempSync(join(tmpdir(), "zbrain-mw-int-"));
  const runtimeDir = join(root, "runtime");
  const workspacesDir = join(runtimeDir, "workspaces");
  for (const name of workspaceNames) {
    mkdirSync(join(workspacesDir, name), { recursive: true });
  }
  return {
    root,
    paths: resolveRuntimePaths({ cwd: root, runtimeDir }),
    workspacesDir,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  };
}

const primaryResult = (ws: string, tier: "axioms" | "projects") => ({
  path: `/ws/${ws}/${tier}/doc.md`,
  score: 5,
  snippet: "snippet",
  body: "body",
});

describe("V1 — no secondaries configured: single workspace only", () => {
  test("identical to single-workspace when no secondary_workspaces", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh"]);
    const called: string[] = [];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "architecture", secondaries: [], workspacesDir },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return [primaryResult("ttdvkh", "axioms")];
          },
        },
      );
      expect(called).toEqual(["ttdvkh"]);
      expect(context.markdown).not.toContain("Secondary Workspaces");
      expect(context.markdown).not.toContain("| Score | Workspace |");
    } finally {
      cleanup();
    }
  });
});

describe("V2 — keyword triggers secondary workspace", () => {
  test("primary + up to secondary limit results, total ≤ 8", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core"]);
    const secondaries: SecondaryWorkspaceEntry[] = [
      { workspace: "framework-core", keywords: ["file-storage"], limit: 3 },
    ];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "file-storage starter", secondaries, workspacesDir, limit: 8 },
        {
          searchWorkspace: ({ workspace, limit }) => {
            return Array.from({ length: Math.min(3, limit ?? 3) }, (_, i) => ({
              path: `/ws/${workspace}/projects/p${i}.md`,
              score: 5 - i,
              snippet: `s${i}`,
              body: `b${i}`,
            }));
          },
        },
      );
      expect(context.results.length).toBeLessThanOrEqual(8);
      const fwResults = context.results.filter((r) => r.workspace === "framework-core");
      expect(fwResults.length).toBeGreaterThan(0);
      expect(fwResults.length).toBeLessThanOrEqual(3);
    } finally {
      cleanup();
    }
  });
});

describe("V3 — @tag includes secondary workspace", () => {
  test("tag stripped from query, research workspace queried", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "research"]);
    const querySeen = new Map<string, string>();
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "design pattern @research", secondaries: [], workspacesDir },
        {
          searchWorkspace: ({ workspace, query }) => {
            querySeen.set(workspace, query ?? "");
            return [{ path: `/ws/${workspace}/axioms/a.md`, score: 3, snippet: "s", body: "b" }];
          },
        },
      );
      expect(querySeen.has("research")).toBe(true);
      expect(querySeen.get("ttdvkh")).not.toContain("@research");
      expect(querySeen.get("research")).not.toContain("@research");
      const researchResult = context.results.find((r) => r.workspace === "research");
      expect(researchResult).toBeDefined();
    } finally {
      cleanup();
    }
  });
});

describe("V4 — same workspace from keyword AND @tag → queried once", () => {
  test("framework-core workspace queried exactly once when triggered both ways", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core"]);
    const callCount = new Map<string, number>();
    const secondaries: SecondaryWorkspaceEntry[] = [
      { workspace: "framework-core", keywords: ["file-storage"], limit: 3 },
    ];
    try {
      retrieveMultiWorkspaceContext(
        paths,
        {
          primaryWorkspace: "ttdvkh",
          query: "file-storage @framework-core",
          secondaries,
          workspacesDir,
        },
        {
          searchWorkspace: ({ workspace }) => {
            callCount.set(workspace, (callCount.get(workspace) ?? 0) + 1);
            return [];
          },
        },
      );
      expect(callCount.get("framework-core")).toBe(1);
    } finally {
      cleanup();
    }
  });
});

describe("V5 — missing secondary workspace → skip with warning, retrieval continues", () => {
  test("retrieval succeeds and only existing workspaces are queried", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh"]);
    const called: string[] = [];
    const secondaries: SecondaryWorkspaceEntry[] = [
      { workspace: "nonexistent-ws", keywords: ["missing-keyword"], limit: 2 },
    ];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "missing-keyword topic", secondaries, workspacesDir },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return [{ path: `/ws/${workspace}/axioms/a.md`, score: 4, snippet: "s", body: "b" }];
          },
        },
      );
      expect(called).toEqual(["ttdvkh"]);
      expect(context.results).toHaveLength(1);
    } finally {
      cleanup();
    }
  });
});

describe("V6 — primary fills all slots → no secondary results", () => {
  test("secondary not queried when primary fills limit", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core"]);
    const called: string[] = [];
    const primaryResults = Array.from({ length: 8 }, (_, i) => ({
      path: `/ws/ttdvkh/axioms/a${i}.md`,
      score: 10 - i,
      snippet: `s${i}`,
      body: `b${i}`,
    }));
    const secondaries: SecondaryWorkspaceEntry[] = [
      { workspace: "framework-core", keywords: ["file-storage"], limit: 3 },
    ];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "ttdvkh", query: "file-storage architecture", secondaries, workspacesDir, limit: 8 },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return workspace === "ttdvkh"
              ? primaryResults
              : [{ path: "/ws/fw/p.md", score: 3, snippet: "s", body: "b" }];
          },
        },
      );
      expect(context.results).toHaveLength(8);
      expect(called).not.toContain("framework-core");
      expect(context.results.every((r) => r.workspace === undefined)).toBe(true);
    } finally {
      cleanup();
    }
  });
});

describe("V7 — primary returns 3, two secondaries → remaining 5 slots split", () => {
  test("secondary slots allocated fairly, total ≤ 8", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["ttdvkh", "framework-core", "research"]);
    const secondaries: SecondaryWorkspaceEntry[] = [
      { workspace: "framework-core", keywords: ["file-storage"], limit: 3 },
      { workspace: "research", keywords: ["paper"], limit: 2 },
    ];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        {
          primaryWorkspace: "ttdvkh",
          query: "file-storage paper architecture",
          secondaries,
          workspacesDir,
          limit: 8,
        },
        {
          searchWorkspace: ({ workspace, limit }) => {
            const count = workspace === "ttdvkh" ? 3 : Math.min(3, limit ?? 3);
            return Array.from({ length: count }, (_, i) => ({
              path: `/ws/${workspace}/projects/p${i}.md`,
              score: 5 - i,
              snippet: `s${i}`,
              body: `b${i}`,
            }));
          },
        },
      );
      expect(context.results.length).toBeLessThanOrEqual(8);
      const fwCount = context.results.filter((r) => r.workspace === "framework-core").length;
      const researchCount = context.results.filter((r) => r.workspace === "research").length;
      expect(fwCount).toBeGreaterThan(0);
      expect(researchCount).toBeGreaterThan(0);
    } finally {
      cleanup();
    }
  });
});

describe("V8 — evidence pipeline unaffected", () => {
  test("evidence-related source files do not import secondary workspace types", () => {
    const { readdirSync, readFileSync } = require("node:fs");
    const { join } = require("node:path");
    const evidenceFiles = readdirSync(join(process.cwd(), "src", "core"))
      .filter((file: string) => file.startsWith("evidence-") && file.endsWith(".ts"));

    for (const file of evidenceFiles) {
      expect(readFileSync(join(process.cwd(), "src", "core", file), "utf8")).not.toContain("secondary_workspaces");
    }
  });
});

describe("V9 — project without secondary_workspaces behaves as before", () => {
  test("pointer without secondary_workspaces field works identically to today", () => {
    const { paths, workspacesDir, cleanup } = makeEnv(["programming"]);
    const called: string[] = [];
    try {
      const context = retrieveMultiWorkspaceContext(
        paths,
        { primaryWorkspace: "programming", query: "solid design", secondaries: [], workspacesDir },
        {
          searchWorkspace: ({ workspace }) => {
            called.push(workspace);
            return [{ path: `/ws/${workspace}/axioms/solid.md`, score: 8, snippet: "solid", body: "Axiom body" }];
          },
        },
      );
      expect(called).toEqual(["programming"]);
      expect(context.markdown).toContain("Workspace: programming");
      expect(context.markdown).not.toContain("Secondary Workspaces");
      expect(context.results[0]?.path).toContain("/axioms/");
    } finally {
      cleanup();
    }
  });
});
