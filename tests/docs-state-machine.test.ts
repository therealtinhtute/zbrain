import { describe, expect, test } from "bun:test";
import { bundledAssets } from "../src/generated/bundled-assets";
import { evidenceStates } from "../src/core/evidence-state";

function parseStateLegend(markdown: string): string[] {
  const lines = markdown.split("\n");
  const start = lines.findIndex((line) => line.trim() === "## State Legend");
  if (start === -1) return [];

  const states: string[] = [];
  for (const line of lines.slice(start + 1)) {
    const trimmed = line.trim();
    if (trimmed.startsWith("## ")) break;
    const match = trimmed.match(/^-\s+`([^`]+)`/);
    if (match) states.push(match[1]);
  }
  return states;
}

describe("evidence state docs match code", () => {
  test("bundled evidence-index.md State Legend equals evidenceStates (ISSUE-010)", () => {
    const asset = bundledAssets.find((entry) => entry.relativePath === "templates/evidence-index.md");
    expect(asset).toBeDefined();

    const states = parseStateLegend(asset!.contents);
    expect(states).toEqual([...evidenceStates]);
  });
});
