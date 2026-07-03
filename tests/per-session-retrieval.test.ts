// Regression test for M1 (per-session context files).
// Phase 04 introduced `writeSessionContext` but `retrieveWorkspaceContext`
// kept writing to the shared V1 `current-task.md`, so two parallel agents
// clobbered each other. This test pins the fix: distinct sessionIds
// produce distinct context files, and the same sessionId reuses one file.

import { test, expect, beforeEach, afterEach } from "bun:test";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { retrieveWorkspaceContext, retrieveMultiWorkspaceContext, type RetrievalAdapter } from "../src/core/retrieval";
import type { QmdSearchResult } from "../src/core/qmd-adapter";
import { resolveRuntimePaths } from "../src/core/runtime-paths";
import { writeSessionContext, readSessionContext, listSessionIds } from "../src/core/session";

// Mock adapter: returns a single fake hit, no shell-out to qmd.
function makeMockAdapter(): RetrievalAdapter {
  return {
    searchWorkspace: ({ workspace, query }): QmdSearchResult[] => [
      {
        path: `wiki/axioms/${workspace}.md`,
        score: 0.9,
        snippet: `mock hit for ${query}`,
        body: `body for ${query}`,
      },
    ],
  };
}

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-session-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("retrieveWorkspaceContext: two parallel sessions produce distinct files (no clobber)", () => {
  const adapter = makeMockAdapter();
  const r1 = retrieveWorkspaceContext(paths, { workspace: "research", query: "q1", sessionId: "agent-A" }, adapter);
  const r2 = retrieveWorkspaceContext(paths, { workspace: "research", query: "q2", sessionId: "agent-B" }, adapter);
  expect(r1.sessionId).toBe("agent-A");
  expect(r2.sessionId).toBe("agent-B");
  expect(r1.filePath).not.toBe(r2.filePath);
  expect(existsSync(r1.filePath)).toBe(true);
  expect(existsSync(r2.filePath)).toBe(true);
  // Neither file was clobbered: A still has q1's content, B has q2's.
  expect(readFileSync(r1.filePath, "utf8")).toContain("q1");
  expect(readFileSync(r2.filePath, "utf8")).toContain("q2");
  expect(readFileSync(r1.filePath, "utf8")).not.toContain("q2");
});

test("retrieveWorkspaceContext: same sessionId reuses the same file (subsequent writes are deterministic)", () => {
  const adapter = makeMockAdapter();
  const r1 = retrieveWorkspaceContext(paths, { workspace: "research", query: "first", sessionId: "agent-A" }, adapter);
  const r2 = retrieveWorkspaceContext(paths, { workspace: "research", query: "second", sessionId: "agent-A" }, adapter);
  expect(r1.filePath).toBe(r2.filePath);
  // Latest write wins within a session — the file shows the second query.
  expect(readFileSync(r2.filePath, "utf8")).toContain("second");
  // listSessionIds now shows agent-A under the project root.
  expect(listSessionIds(paths, paths.cwd)).toEqual(["agent-A"]);
});

test("retrieveWorkspaceContext: omitted sessionId is auto-generated (one per call)", () => {
  const adapter = makeMockAdapter();
  const r1 = retrieveWorkspaceContext(paths, { workspace: "research", query: "x" }, adapter);
  const r2 = retrieveWorkspaceContext(paths, { workspace: "research", query: "x" }, adapter);
  expect(r1.sessionId).not.toBe(r2.sessionId);
  expect(r1.filePath).not.toBe(r2.filePath);
  expect(listSessionIds(paths, paths.cwd).length).toBe(2);
});

test("retrieveMultiWorkspaceContext: respects sessionId and isolates from a parallel single-workspace call", () => {
  const adapter = makeMockAdapter();
  const multi = retrieveMultiWorkspaceContext(
    paths,
    {
      primaryWorkspace: "research",
      query: "x",
      secondaries: [],
      workspacesDir: paths.workspacesDir,
      sessionId: "multi-agent",
    },
    adapter,
  );
  const single = retrieveWorkspaceContext(paths, { workspace: "research", query: "x", sessionId: "single-agent" }, adapter);
  expect(multi.filePath).not.toBe(single.filePath);
  expect(readSessionContext(paths, paths.cwd, "multi-agent")).not.toBeNull();
  expect(readSessionContext(paths, paths.cwd, "single-agent")).not.toBeNull();
});
