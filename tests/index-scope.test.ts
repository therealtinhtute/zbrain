import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { QmdAdapter, type QmdRunner } from "../src/core/qmd-adapter";
import { resolveRuntimePaths } from "../src/core/runtime-paths";
import { workspaceRoot, wikiRoot, wikiTierPath } from "../src/core/runtime-paths";
import { WikiTiers } from "../src/core/workspace-layout";

let tempHome: string;
let capturedArgs: string[][];

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-scope-"));
  capturedArgs = [];
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

function makeRunner(): QmdRunner {
  return (args) => {
    capturedArgs.push(args);
    if (args[0] === "search") return { stdout: "[]\n", stderr: "", exitCode: 0 };
    // "collection show" simulates a not-yet-registered collection so
    // indexWorkspace takes the "collection add" branch.
    if (args[0] === "collection" && args[1] === "show") return { stdout: "", stderr: "", exitCode: 1 };
    return { stdout: "", stderr: "", exitCode: 0 };
  };
}

test("AC-P0-1 (structural): indexWorkspace points qmd at wiki/ subtree, not workspace root", () => {
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  const workspace = "research";

  new QmdAdapter(paths, makeRunner()).indexWorkspace({ workspace });

  const addCall = capturedArgs.find((args) => args[0] === "collection" && args[1] === "add");
  expect(addCall).toBeDefined();
  expect(addCall?.[0]).toBe("collection");
  expect(addCall?.[1]).toBe("add");
  // Path must end with the workspace's wiki/ subtree, not the workspace root.
  expect(addCall?.[2]).toBe(wikiRoot(paths, workspace));
  expect(addCall?.[2]).not.toBe(workspaceRoot(paths, workspace));
});

test("wikiRoot returns <runtimeDir>/workspaces/<name>/wiki", () => {
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  expect(wikiRoot(paths, "finance")).toBe(join(tempHome, "workspaces", "finance", "wiki"));
});

test("wikiTierPath returns wiki/<tier>/ for every WikiTier", () => {
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  for (const tier of WikiTiers) {
    expect(wikiTierPath(paths, "health", tier)).toBe(
      join(tempHome, "workspaces", "health", "wiki", tier),
    );
  }
});

test("AC-P0-1 (end-to-end with poisoned fixture): qmd is never asked to index evidence/", () => {
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  const ws = "research";

  // Seed the V2 layout including a poisoned evidence file. We never call
  // qmd ourselves — we only assert that the adapter's args would never give
  // qmd a chance to see the evidence tree.
  mkdirSync(wikiTierPath(paths, ws, "axioms"), { recursive: true });
  mkdirSync(join(paths.workspacesDir, ws, "evidence", "sources", "20260623-poison"), { recursive: true });
  writeFileSync(
    join(paths.workspacesDir, ws, "evidence", "sources", "20260623-poison", "raw.md"),
    "INJECTED_PROMPT_POISON",
  );

  new QmdAdapter(paths, makeRunner()).indexWorkspace({ workspace: ws });

  const addCall = capturedArgs.find((args) => args[0] === "collection" && args[1] === "add");
  const indexedPath = addCall?.[2];
  // The structural guarantee: qmd is pointed at wiki/ ONLY. No path that
  // contains "evidence" was ever passed. A poisoned raw.md cannot be indexed.
  expect(indexedPath).toBeDefined();
  expect(indexedPath).not.toContain("evidence");
  expect(indexedPath?.endsWith("/wiki")).toBe(true);
});
