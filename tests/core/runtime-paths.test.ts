import { describe, expect, test } from "bun:test";
import { mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

describe("resolveRuntimePaths", () => {
  test("uses legacy ~/.zwiki when ~/.zbrain is missing", () => {
    const root = join(tmpdir(), `zbrain-runtime-paths-${Date.now()}-${Math.random()}`);
    const homeDir = join(root, "home");
    const cwd = join(root, "project");

    try {
      mkdirSync(join(homeDir, ".zwiki"), { recursive: true });
      mkdirSync(cwd, { recursive: true });

      const paths = resolveRuntimePaths({ homeDir, cwd });
      expect(paths.runtimeDir).toBe(join(homeDir, ".zwiki"));
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  test("prefers ~/.zbrain when it exists", () => {
    const root = join(tmpdir(), `zbrain-runtime-paths-${Date.now()}-${Math.random()}`);
    const homeDir = join(root, "home");
    const cwd = join(root, "project");

    try {
      mkdirSync(join(homeDir, ".zbrain"), { recursive: true });
      mkdirSync(join(homeDir, ".zwiki"), { recursive: true });
      mkdirSync(cwd, { recursive: true });

      const paths = resolveRuntimePaths({ homeDir, cwd });
      expect(paths.runtimeDir).toBe(join(homeDir, ".zbrain"));
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
