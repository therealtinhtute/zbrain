import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

describe("engine and starter workspace assets", () => {
  test("engine assets encode workspace isolation and evidence rules", () => {
    const constraints = readFileSync(join(process.cwd(), "assets", "engine", "constraints.md"), "utf8");
    const retrieval = readFileSync(join(process.cwd(), "assets", "engine", "retrieval-rules.md"), "utf8");
    const evidence = readFileSync(join(process.cwd(), "assets", "engine", "evidence-rules.md"), "utf8");
    const codexRules = readFileSync(join(process.cwd(), "assets", "engine", "codex-rules.md"), "utf8");

    expect(constraints.includes("Never mix knowledge across workspaces.")).toBe(true);
    expect(retrieval.includes("current-task.md")).toBe(true);
    expect(evidence.includes("zbrain:learn")).toBe(true);
    expect(evidence.includes("zbrain:ingest")).toBe(true);
    expect(evidence.includes("zbrain:ask")).toBe(true);
    expect(codexRules.includes("projects.json")).toBe(true);
  });

  test("workspace template exists for scaffold generation", () => {
    expect(existsSync(join(process.cwd(), "assets", "templates", "workspace.md"))).toBe(true);
  });
});
