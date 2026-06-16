import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import YAML from "js-yaml";

const skillsRoot = join(process.cwd(), "assets", "skills");

const expectedSkills: Record<string, string> = {
  "zbrain-ask": "zbrain:ask",
  "zbrain-ingest": "zbrain:ingest",
  "zbrain-learn": "zbrain:learn",
  "zbrain-research": "zbrain:research",
};

describe("skill assets", () => {
  test("ships only the four public zbrain skill directories", () => {
    const dirs = readdirSync(skillsRoot).sort();
    expect(dirs).toEqual(Object.keys(expectedSkills).sort());
  });

  test("each skill directory contains SKILL.md", () => {
    for (const dir of Object.keys(expectedSkills)) {
      const skillFile = join(skillsRoot, dir, "SKILL.md");
      const contents = readFileSync(skillFile, "utf8");
      expect(contents.trim().length).toBeGreaterThan(0);
    }
  });

  test("each SKILL.md exposes the correct zbrain:* name in frontmatter", () => {
    for (const [dir, expectedName] of Object.entries(expectedSkills)) {
      const contents = readFileSync(join(skillsRoot, dir, "SKILL.md"), "utf8");
      const match = contents.match(/^---\n([\s\S]*?)\n---/);
      expect(match).not.toBeNull();

      const frontmatter = YAML.load(match![1]) as Record<string, unknown>;
      expect(frontmatter.name).toBe(expectedName);
      expect(typeof frontmatter.description).toBe("string");
      expect((frontmatter.description as string).length).toBeGreaterThan(0);
    }
  });

  test("skill names do not reference legacy wiki-template commands", () => {
    for (const dir of Object.keys(expectedSkills)) {
      const contents = readFileSync(join(skillsRoot, dir, "SKILL.md"), "utf8");
      expect(contents.includes("/use-wiki")).toBe(false);
      expect(contents.includes("/switch-workspace")).toBe(false);
      expect(contents.includes("/update-wiki")).toBe(false);
    }
  });

  test("zbrain-ask documents the central context_file output path", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-ask", "SKILL.md"), "utf8");
    expect(contents.includes("context_file")).toBe(true);
  });

  test("zbrain-ingest has a references directory with pipeline.md", () => {
    const pipelineRef = join(skillsRoot, "zbrain-ingest", "references", "pipeline.md");
    const contents = readFileSync(pipelineRef, "utf8");
    expect(contents.includes("workspace_at_ingest")).toBe(true);
    expect(contents.includes("verified-facts.md")).toBe(true);
  });

  test("zbrain-ingest SKILL.md references pipeline.md", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-ingest", "SKILL.md"), "utf8");
    expect(contents.includes("pipeline.md")).toBe(true);
  });

  test("zbrain-learn documents evidence source creation", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-learn", "SKILL.md"), "utf8");
    expect(contents.includes("evidence/sources")).toBe(true);
  });
});
