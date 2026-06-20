import { describe, expect, test } from "bun:test";
import { classifyByTier, rankRetrievalResults } from "../../src/core/retrieval-ranking";

describe("retrieval ranking", () => {
  test("classifies paths by retrieval tier", () => {
    expect(classifyByTier("/ws/programming/axioms/fact.md")).toBe("P0");
    expect(classifyByTier("/ws/programming/mental-models/model.md")).toBe("P1");
    expect(classifyByTier("/ws/programming/projects/book.md")).toBe("P2");
    expect(classifyByTier("/ws/programming/decisions/adr.md")).toBe("P3");
    expect(classifyByTier("/ws/programming/random/note.md")).toBe("P2");
  });

  test("promotes higher tiers while preserving order within each tier", () => {
    const ranked = rankRetrievalResults([
      { path: "/ws/programming/projects/p2-a.md", score: 80, snippet: "project-a" },
      { path: "/ws/programming/axioms/p0-a.md", score: 10, snippet: "axiom-a" },
      { path: "/ws/programming/mental-models/p1-a.md", score: 15, snippet: "model-a" },
      { path: "/ws/programming/axioms/p0-b.md", score: 9, snippet: "axiom-b" },
      { path: "/ws/programming/decisions/p3-a.md", score: 1, snippet: "decision-a" },
    ]);

    expect(ranked.map((entry) => entry.path)).toEqual([
      "/ws/programming/axioms/p0-a.md",
      "/ws/programming/axioms/p0-b.md",
      "/ws/programming/mental-models/p1-a.md",
      "/ws/programming/projects/p2-a.md",
      "/ws/programming/decisions/p3-a.md",
    ]);
  });

  test("orders within a tier by BM25 score descending (ISSUE-008)", () => {
    const ranked = rankRetrievalResults([
      { path: "/ws/programming/axioms/low.md", score: 2, snippet: "low" },
      { path: "/ws/programming/axioms/high.md", score: 9, snippet: "high" },
      { path: "/ws/programming/axioms/mid.md", score: 5, snippet: "mid" },
    ]);

    expect(ranked.map((entry) => entry.path)).toEqual([
      "/ws/programming/axioms/high.md",
      "/ws/programming/axioms/mid.md",
      "/ws/programming/axioms/low.md",
    ]);
  });

  test("keeps qmd order for equal scores within a tier", () => {
    const ranked = rankRetrievalResults([
      { path: "/ws/programming/axioms/first.md", score: 5, snippet: "a" },
      { path: "/ws/programming/axioms/second.md", score: 5, snippet: "b" },
    ]);

    expect(ranked.map((entry) => entry.path)).toEqual([
      "/ws/programming/axioms/first.md",
      "/ws/programming/axioms/second.md",
    ]);
  });
});
