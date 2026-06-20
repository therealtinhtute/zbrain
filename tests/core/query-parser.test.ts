import { describe, expect, test } from "bun:test";
import { extractWorkspaceTags, matchKeywordWorkspaces, parseQuery } from "../../src/core/query-parser";
import { type SecondaryWorkspaceEntry } from "../../src/schemas/config";

const secondaries: SecondaryWorkspaceEntry[] = [
  { workspace: "framework-core", keywords: ["file-storage", "boilerplate", "starter-kit"], limit: 3 },
  { workspace: "research", keywords: ["paper", "study", "research"], limit: 2 },
];

const authSecondaries: SecondaryWorkspaceEntry[] = [
  { workspace: "auth-service", keywords: ["auth"], limit: 2 },
];

describe("extractWorkspaceTags", () => {
  test("extracts single tag and removes it from query", () => {
    const result = extractWorkspaceTags("how does @research work");
    expect(result.tags).toEqual(["research"]);
    expect(result.cleanQuery).toBe("how does work");
  });

  test("extracts multiple tags", () => {
    const result = extractWorkspaceTags("@framework-core pattern for @research");
    expect(result.tags).toContain("framework-core");
    expect(result.tags).toContain("research");
    expect(result.cleanQuery).not.toContain("@");
  });

  test("returns empty tags and unchanged query when no tags", () => {
    const result = extractWorkspaceTags("how does file-storage work");
    expect(result.tags).toEqual([]);
    expect(result.cleanQuery).toBe("how does file-storage work");
  });

  test("handles tag at start of query", () => {
    const result = extractWorkspaceTags("@research what is BM25");
    expect(result.tags).toEqual(["research"]);
    expect(result.cleanQuery).toBe("what is BM25");
  });

  test("handles tag at end of query", () => {
    const result = extractWorkspaceTags("file-storage starter @framework-core");
    expect(result.tags).toEqual(["framework-core"]);
    expect(result.cleanQuery).toBe("file-storage starter");
  });

  test("handles tag-only query", () => {
    const result = extractWorkspaceTags("@research");
    expect(result.tags).toEqual(["research"]);
    expect(result.cleanQuery).toBe("");
  });

  test("collapses multiple spaces left by tag removal", () => {
    const result = extractWorkspaceTags("how @research does this");
    expect(result.cleanQuery).toBe("how does this");
  });
});

describe("matchKeywordWorkspaces", () => {
  test("matches single keyword", () => {
    const result = matchKeywordWorkspaces("how to set up file-storage", secondaries);
    expect(result).toContain("framework-core");
    expect(result).not.toContain("research");
  });

  test("matches multiple keywords from different secondaries", () => {
    const result = matchKeywordWorkspaces("file-storage paper review", secondaries);
    expect(result).toContain("framework-core");
    expect(result).toContain("research");
  });

  test("returns empty when no keywords match", () => {
    const result = matchKeywordWorkspaces("how to write unit tests", secondaries);
    expect(result).toEqual([]);
  });

  test("is case-insensitive", () => {
    const result = matchKeywordWorkspaces("FILE-STORAGE pattern", secondaries);
    expect(result).toContain("framework-core");
  });

  test("deduplicates workspace when multiple keywords from same workspace match", () => {
    const result = matchKeywordWorkspaces("file-storage boilerplate starter-kit", secondaries);
    expect(result.filter((w) => w === "framework-core")).toHaveLength(1);
  });

  test("returns empty when secondaries array is empty", () => {
    const result = matchKeywordWorkspaces("file-storage", []);
    expect(result).toEqual([]);
  });

  test("does not match a keyword that is only a substring (ISSUE-023)", () => {
    const result = matchKeywordWorkspaces("who is the author of this", authSecondaries);
    expect(result).toEqual([]);
  });

  test("still matches a keyword on a word boundary", () => {
    const result = matchKeywordWorkspaces("how does auth work", authSecondaries);
    expect(result).toContain("auth-service");
  });
});

describe("parseQuery", () => {
  test("combines tag extraction and keyword matching", () => {
    const result = parseQuery("file-storage pattern @research", secondaries);
    expect(result.secondaryWorkspaces).toContain("framework-core");
    expect(result.secondaryWorkspaces).toContain("research");
    expect(result.cleanQuery).toBe("file-storage pattern");
  });

  test("deduplicates workspace triggered by both tag and keyword", () => {
    const result = parseQuery("file-storage @framework-core", secondaries);
    const fwCount = result.secondaryWorkspaces.filter((w) => w === "framework-core").length;
    expect(fwCount).toBe(1);
  });

  test("returns empty secondaryWorkspaces for query with no matches", () => {
    const result = parseQuery("how does clean architecture work", secondaries);
    expect(result.secondaryWorkspaces).toEqual([]);
    expect(result.cleanQuery).toBe("how does clean architecture work");
  });

  test("handles empty secondaries array", () => {
    const result = parseQuery("file-storage pattern", []);
    expect(result.secondaryWorkspaces).toEqual([]);
    expect(result.cleanQuery).toBe("file-storage pattern");
  });

  test("strips tags from cleanQuery even when no keyword matches", () => {
    const result = parseQuery("@research query with no keywords", []);
    expect(result.cleanQuery).toBe("query with no keywords");
    expect(result.secondaryWorkspaces).toEqual(["research"]);
  });

  test("exposes explicit @tags separately from keyword matches (ISSUE-009)", () => {
    const result = parseQuery("file-storage pattern @research", secondaries);
    expect(result.tags).toEqual(["research"]);
    expect(result.secondaryWorkspaces).toContain("framework-core");
  });

  test("tags is empty when only keywords match", () => {
    const result = parseQuery("file-storage pattern", secondaries);
    expect(result.tags).toEqual([]);
    expect(result.secondaryWorkspaces).toEqual(["framework-core"]);
  });
});
