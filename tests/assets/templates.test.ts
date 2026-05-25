import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import YAML from "js-yaml";

function extractFrontmatter(markdown: string): string | null {
  const match = markdown.match(/^---\n([\s\S]*?)\n---/);
  return match ? match[1] : null;
}

describe("template assets", () => {
  test("markdown templates with frontmatter parse cleanly", () => {
    const templateFiles = [
      "workspace.md",
      "axiom.md",
      "mental-model.md",
      "project.md",
    ];

    for (const file of templateFiles) {
      const contents = readFileSync(join(process.cwd(), "assets", "templates", file), "utf8");
      const frontmatter = extractFrontmatter(contents);

      expect(frontmatter).not.toBeNull();
      expect(YAML.load(frontmatter!)).toBeTruthy();
    }
  });

  test("yaml and json templates parse cleanly", () => {
    const yamlTemplate = readFileSync(
      join(process.cwd(), "assets", "templates", "evidence-source.yaml"),
      "utf8",
    );
    const manifestTemplate = readFileSync(
      join(process.cwd(), "assets", "templates", "evidence-manifest.yaml"),
      "utf8",
    );
    const qaTodoTemplate = readFileSync(
      join(process.cwd(), "assets", "templates", "evidence-qa-todo.json"),
      "utf8",
    );
    const checkpointTemplate = readFileSync(
      join(process.cwd(), "assets", "templates", "evidence-apply-checkpoint.json"),
      "utf8",
    );

    expect(YAML.load(yamlTemplate)).toBeTruthy();
    expect(YAML.load(manifestTemplate)).toBeTruthy();
    expect(JSON.parse(qaTodoTemplate)).toBeTruthy();
    expect(JSON.parse(checkpointTemplate)).toBeTruthy();
  });
});
