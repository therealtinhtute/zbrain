import { describe, expect, test } from "bun:test";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { extractBundledAssets, getBundledAssets } from "../../src/core/assets";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

describe("asset extraction", () => {
  test("discovers bundled asset files", () => {
    const assets = getBundledAssets();
    const relativePaths = assets.map((asset) => asset.relativePath);

    expect(relativePaths).toContain("engine/system-prompt.md");
    expect(relativePaths).toContain("skills/zbrain-ask/SKILL.md");
  });

  test("extracts assets into the runtime tree and preserves workspace content", () => {
    const baseDir = join(tmpdir(), `zbrain-assets-${Date.now()}-${Math.random()}`);
    const runtimeDir = join(baseDir, "runtime");
    const workspacesDir = join(runtimeDir, "workspaces");
    mkdirSync(join(workspacesDir, "programming"), { recursive: true });
    writeFileSync(join(workspacesDir, "README.md"), "user workspace content\n");

    try {
      const result = extractBundledAssets(
        resolveRuntimePaths({ cwd: baseDir, runtimeDir }),
      );

      expect(result.copied.length).toBeGreaterThan(0);
      expect(result.copied).toContain("engine/system-prompt.md");
      expect(existsSync(join(runtimeDir, "engine", "system-prompt.md"))).toBe(true);
      expect(readFileSync(join(workspacesDir, "README.md"), "utf8")).toBe("user workspace content\n");
    } finally {
      rmSync(baseDir, { recursive: true, force: true });
    }
  });
});
