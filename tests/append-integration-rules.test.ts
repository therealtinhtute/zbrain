import { test, expect } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { appendIntegrationRules } from "../src/commands/helpers";

test("appendIntegrationRules is idempotent for ## headings: re-running does not duplicate the section", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "zbrain-append-"));
  const claudeFile = join(tempDir, "CLAUDE.md");
  writeFileSync(claudeFile, "# Project notes\n\nSome existing content.\n", "utf8");

  const section = "## zbrain Integration\n\nSome integration rules.\n";

  const first = appendIntegrationRules(claudeFile, section, "zbrain Integration");
  expect(first.updated).toBe(true);

  const second = appendIntegrationRules(claudeFile, section, "zbrain Integration");
  expect(second.updated).toBe(false);

  const content = readFileSync(claudeFile, "utf8");
  const occurrences = content.split("## zbrain Integration").length - 1;
  expect(occurrences).toBe(1);

  rmSync(tempDir, { recursive: true, force: true });
});

test("appendIntegrationRules replaces stale section content with updated content instead of duplicating", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "zbrain-append-"));
  const claudeFile = join(tempDir, "CLAUDE.md");
  writeFileSync(
    claudeFile,
    "# Project notes\n\n## zbrain Integration\n\nOld rules.\n",
    "utf8",
  );

  const section = "## zbrain Integration\n\nNew rules, expanded.\n";
  const result = appendIntegrationRules(claudeFile, section, "zbrain Integration");
  expect(result.updated).toBe(true);

  const content = readFileSync(claudeFile, "utf8");
  const occurrences = content.split("## zbrain Integration").length - 1;
  expect(occurrences).toBe(1);
  expect(content).toContain("New rules, expanded.");
  expect(content).not.toContain("Old rules.");

  rmSync(tempDir, { recursive: true, force: true });
});
