import { describe, expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { resolveSecondaryWorkspaces } from "../../src/core/secondary-resolver";

function makeTempWorkspacesDir(names: string[]): { workspacesDir: string; cleanup: () => void } {
  const workspacesDir = mkdtempSync(join(tmpdir(), "zbrain-ws-"));
  for (const name of names) {
    mkdirSync(join(workspacesDir, name));
  }
  return {
    workspacesDir,
    cleanup: () => rmSync(workspacesDir, { recursive: true, force: true }),
  };
}

describe("resolveSecondaryWorkspaces", () => {
  test("returns all names when all workspaces exist", () => {
    const { workspacesDir, cleanup } = makeTempWorkspacesDir(["framework-core", "research"]);
    try {
      const result = resolveSecondaryWorkspaces(workspacesDir, ["framework-core", "research"]);
      expect(result.resolved).toEqual(["framework-core", "research"]);
      expect(result.warnings).toHaveLength(0);
    } finally {
      cleanup();
    }
  });

  test("excludes missing workspace and emits warning", () => {
    const { workspacesDir, cleanup } = makeTempWorkspacesDir(["framework-core"]);
    try {
      const result = resolveSecondaryWorkspaces(workspacesDir, ["framework-core", "missing-ws"]);
      expect(result.resolved).toEqual(["framework-core"]);
      expect(result.warnings).toHaveLength(1);
      expect(result.warnings[0]).toContain("missing-ws");
    } finally {
      cleanup();
    }
  });

  test("returns empty resolved when all workspaces are missing", () => {
    const { workspacesDir, cleanup } = makeTempWorkspacesDir([]);
    try {
      const result = resolveSecondaryWorkspaces(workspacesDir, ["a", "b"]);
      expect(result.resolved).toEqual([]);
      expect(result.warnings).toHaveLength(2);
    } finally {
      cleanup();
    }
  });

  test("returns empty resolved and no warnings for empty names array", () => {
    const { workspacesDir, cleanup } = makeTempWorkspacesDir(["framework-core"]);
    try {
      const result = resolveSecondaryWorkspaces(workspacesDir, []);
      expect(result.resolved).toEqual([]);
      expect(result.warnings).toHaveLength(0);
    } finally {
      cleanup();
    }
  });

  test("returns empty resolved and warnings when workspacesDir does not exist", () => {
    const result = resolveSecondaryWorkspaces("/nonexistent/path/zbrain/workspaces", ["research"]);
    expect(result.resolved).toEqual([]);
    expect(result.warnings).toHaveLength(1);
    expect(result.warnings[0]).toContain("research");
  });
});
