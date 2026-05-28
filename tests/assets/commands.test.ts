import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import YAML from "js-yaml";

const skillsRoot = join(process.cwd(), "assets", "skills");

const expectedSkills: Record<string, string> = {
  "zbrain-ask": "zbrain:ask",
  "zbrain-ingest": "zbrain:ingest",
  "zbrain-learn": "zbrain:learn",
  "zbrain-reflect": "zbrain:reflect",
  "zbrain-reindex": "zbrain:reindex",
  "zbrain-state": "zbrain:state",
  "zbrain-workspace": "zbrain:workspace",
};

describe("skill assets", () => {
  test("ships all seven zbrain skill directories", () => {
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

  test("zbrain-ask documents current-task.md output path", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-ask", "SKILL.md"), "utf8");
    expect(contents.includes("current-task.md")).toBe(true);
  });

  test("zbrain-workspace documents zbrain.json pointer path", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-workspace", "SKILL.md"), "utf8");
    expect(contents.includes(".claude/zbrain.json")).toBe(true);
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

  test("zbrain-learn has a references directory with fetch.md", () => {
    const fetchRef = join(skillsRoot, "zbrain-learn", "references", "fetch.md");
    const contents = readFileSync(fetchRef, "utf8");
    expect(contents.includes("defuddle.md")).toBe(true);
    expect(contents.includes("r.jina.ai")).toBe(true);
  });

  test("zbrain-learn SKILL.md references fetch.md", () => {
    const contents = readFileSync(join(skillsRoot, "zbrain-learn", "SKILL.md"), "utf8");
    expect(contents.includes("fetch.md")).toBe(true);
  });
});
