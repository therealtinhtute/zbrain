import { describe, expect, test } from "bun:test";
import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { resolveActiveWorkspace, WorkspaceResolutionError } from "../../src/core/workspace-resolver";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

function createRuntimeFixture() {
  const rootDir = join(tmpdir(), `zbrain-resolver-${Date.now()}-${Math.random()}`);
  mkdirSync(rootDir, { recursive: true });
  mkdirSync(join(rootDir, "project", ".claude"), { recursive: true });
  mkdirSync(join(rootDir, "runtime", "workspaces"), { recursive: true });

  return {
    rootDir,
    runtimeDir: join(rootDir, "runtime"),
    projectDir: join(rootDir, "project"),
  };
}

describe("resolveActiveWorkspace", () => {
  test("prefers the project pointer over global config", () => {
    const fixture = createRuntimeFixture();

    try {
      mkdirSync(join(fixture.runtimeDir, "workspaces", "programming"));
      mkdirSync(join(fixture.runtimeDir, "workspaces", "finance"));
      writeFileSync(join(fixture.runtimeDir, "config.yml"), "default_workspace: programming\n");
      writeFileSync(
        join(fixture.projectDir, ".claude", "zbrain.json"),
        JSON.stringify({ workspace: "finance" }),
      );

      const resolved = resolveActiveWorkspace(
        resolveRuntimePaths({ cwd: fixture.projectDir, runtimeDir: fixture.runtimeDir }),
      );

      expect(resolved.name).toBe("finance");
      expect(resolved.source).toBe("project_pointer");
    } finally {
      rmSync(fixture.rootDir, { recursive: true, force: true });
    }
  });

  test("falls back to the global default workspace", () => {
    const fixture = createRuntimeFixture();

    try {
      mkdirSync(join(fixture.runtimeDir, "workspaces", "health"));
      writeFileSync(join(fixture.runtimeDir, "config.yml"), "default_workspace: health\n");

      const resolved = resolveActiveWorkspace(
        resolveRuntimePaths({ cwd: fixture.projectDir, runtimeDir: fixture.runtimeDir }),
      );

      expect(resolved.name).toBe("health");
      expect(resolved.source).toBe("global_config");
    } finally {
      rmSync(fixture.rootDir, { recursive: true, force: true });
    }
  });

  test("auto-detects a single workspace", () => {
    const fixture = createRuntimeFixture();

    try {
      mkdirSync(join(fixture.runtimeDir, "workspaces", "philosophy"));

      const resolved = resolveActiveWorkspace(
        resolveRuntimePaths({ cwd: fixture.projectDir, runtimeDir: fixture.runtimeDir }),
      );

      expect(resolved.name).toBe("philosophy");
      expect(resolved.source).toBe("single_workspace_auto");
    } finally {
      rmSync(fixture.rootDir, { recursive: true, force: true });
    }
  });

  test("fails with an actionable error when no workspace can be resolved", () => {
    const fixture = createRuntimeFixture();

    try {
      expect(() =>
        resolveActiveWorkspace(resolveRuntimePaths({ cwd: fixture.projectDir, runtimeDir: fixture.runtimeDir })),
      ).toThrow(WorkspaceResolutionError);
    } finally {
      rmSync(fixture.rootDir, { recursive: true, force: true });
    }
  });
});
