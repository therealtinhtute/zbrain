import { test, expect } from "bun:test";
import { parseFrontmatter, serializeMarkdown } from "../src/core/frontmatter";

test("parseFrontmatter: extracts YAML frontmatter from markdown", () => {
  const md = `---
id: abc-123
tier: axioms
status: active
title: Hello
---
# Body
hello world`;
  const { frontmatter, body } = parseFrontmatter(md);
  expect(frontmatter.id).toBe("abc-123");
  expect(frontmatter.tier).toBe("axioms");
  expect(frontmatter.status).toBe("active");
  expect(frontmatter.title).toBe("Hello");
  expect(body).toContain("# Body");
  expect(body).toContain("hello world");
});

test("parseFrontmatter: no frontmatter returns empty object", () => {
  const md = "# Just a body\nNo frontmatter here.";
  const { frontmatter, body } = parseFrontmatter(md);
  expect(frontmatter).toEqual({});
  expect(body).toBe(md);
});

test("parseFrontmatter: malformed YAML passes through with empty frontmatter", () => {
  const md = `---
: invalid yaml
:::
---
body`;
  const { frontmatter, body } = parseFrontmatter(md);
  expect(frontmatter).toEqual({});
  expect(body).toContain("body");
});

test("serializeMarkdown: writes frontmatter + body", () => {
  const fm = { id: "x", tier: "axioms", status: "active" };
  const body = "# Title\n\ncontent";
  const md = serializeMarkdown(fm, body);
  expect(md.startsWith("---\n")).toBe(true);
  expect(md).toContain("id: x");
  expect(md).toContain("tier: axioms");
  expect(md).toContain("status: active");
  expect(md).toContain("\n---\n");
  expect(md).toContain("# Title");
});

test("roundtrip: serialize then parse yields the same frontmatter", () => {
  const fm = { id: "rt", tier: "projects", status: "active", sources: ["s1", "s2"] };
  const body = "hello body";
  const md = serializeMarkdown(fm, body);
  const { frontmatter, body: parsedBody } = parseFrontmatter(md);
  expect(frontmatter.id).toBe("rt");
  expect(frontmatter.tier).toBe("projects");
  expect(frontmatter.status).toBe("active");
  expect(frontmatter.sources).toEqual(["s1", "s2"]);
  expect(parsedBody).toBe(body);
});
