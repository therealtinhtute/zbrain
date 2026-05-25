import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import YAML from "js-yaml";

describe("command assets", () => {
  test("use only the locked command names", () => {
    const commandFiles = [
      "ask.md",
      "learn.md",
      "reflect.md",
      "workspace.md",
      "reindex.md",
    ];

    for (const file of commandFiles) {
      const contents = readFileSync(join(process.cwd(), "assets", "commands", file), "utf8");

      expect(contents.includes("/use-wiki")).toBe(false);
      expect(contents.includes("/switch-workspace")).toBe(false);
      expect(contents.includes("/update-wiki")).toBe(false);
    }
  });

  test("document runtime paths and invariants where relevant", () => {
    const ask = readFileSync(join(process.cwd(), "assets", "commands", "ask.md"), "utf8");
    const learn = readFileSync(join(process.cwd(), "assets", "commands", "learn.md"), "utf8");
    const workspace = readFileSync(join(process.cwd(), "assets", "commands", "workspace.md"), "utf8");

    expect(ask.includes("current-task.md")).toBe(true);
    expect(learn.includes("workspace_at_ingest")).toBe(true);
    expect(workspace.includes(".claude/zbrain.json")).toBe(true);
  });

  test("command files expose frontmatter skill names", () => {
    const expectedNames: Record<string, string> = {
      "ask.md": "zbrain:ask",
      "learn.md": "zbrain:learn",
      "reflect.md": "zbrain:reflect",
      "workspace.md": "zbrain:workspace",
      "reindex.md": "zbrain:reindex",
    };

    for (const [file, expectedName] of Object.entries(expectedNames)) {
      const contents = readFileSync(join(process.cwd(), "assets", "commands", file), "utf8");
      const match = contents.match(/^---\n([\s\S]*?)\n---/);
      expect(match).not.toBeNull();

      const frontmatter = YAML.load(match![1]) as Record<string, unknown>;
      expect(frontmatter.name).toBe(expectedName);
      expect(typeof frontmatter.description).toBe("string");
    }
  });
});
