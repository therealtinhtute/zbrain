import { describe, expect, test } from "bun:test";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const assetRoot = join(process.cwd(), "assets");

describe("bundled asset tree", () => {
  test("creates the expected top-level directories", () => {
    const entries = readdirSync(assetRoot).sort();

    expect(entries).toEqual([
      "README.md",
      "agents",
      "commands",
      "engine",
      "templates",
      "workspaces",
    ]);
  });

  test("ships starter files in each asset group", () => {
    const requiredFiles = [
      "engine/system-prompt.md",
      "templates/workspace.md",
      "commands/ask.md",
      "agents/wiki-planner.md",
      "workspaces/README.md",
    ];

    for (const file of requiredFiles) {
      expect(existsSync(join(assetRoot, file))).toBe(true);
      expect(readFileSync(join(assetRoot, file), "utf8").trim().length).toBeGreaterThan(0);
    }
  });
});
