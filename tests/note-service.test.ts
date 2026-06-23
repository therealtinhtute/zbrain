import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths } from "../src/core/runtime-paths";
import { wikiTierPath } from "../src/core/runtime-paths";
import { createNote, readNote, listNotes, computeContentSha, slugify } from "../src/core/note-service";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-notes-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  // Create a V2 workspace skeleton.
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("createNote: writes a frontmatter-wrapped markdown file", () => {
  const note = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "auth-is-required",
    body: "All endpoints require authentication.",
    title: "Auth is required",
    sources: ["src-1"],
  });
  expect(note.id).toBeTruthy();
  expect(note.tier).toBe("axioms");
  expect(note.status).toBe("active");
  expect(note.contentSha).toBeTruthy();
  expect(note.relPath).toBe("axioms/auth-is-required.md");
});

test("createNote: refuses to overwrite existing note", () => {
  createNote(paths, { workspace: "research", tier: "axioms", slug: "alpha", body: "x" });
  expect(() =>
    createNote(paths, { workspace: "research", tier: "axioms", slug: "alpha", body: "y" }),
  ).toThrow(/already exists/);
});

test("createNote: rejects invalid tier", () => {
  expect(() =>
    // @ts-expect-error - testing runtime validation
    createNote(paths, { workspace: "research", tier: "invalid", slug: "x", body: "y" }),
  ).toThrow(/Invalid tier/);
});

test("readNote: roundtrips a created note", () => {
  const created = createNote(paths, {
    workspace: "research",
    tier: "projects",
    slug: "oauth-rotation",
    body: "Rotate OAuth tokens every 90 days.",
    sources: ["s1", "s2"],
  });
  const read = readNote(paths, "research", "projects", "oauth-rotation");
  expect(read).not.toBeNull();
  expect(read?.id).toBe(created.id);
  expect(read?.tier).toBe("projects");
  expect(read?.status).toBe("active");
  expect(read?.sources).toEqual(["s1", "s2"]);
  expect(read?.contentSha).toBe(created.contentSha);
  expect(read?.body).toContain("Rotate OAuth tokens");
});

test("readNote: returns null for nonexistent note", () => {
  expect(readNote(paths, "research", "axioms", "missing")).toBeNull();
});

test("listNotes: returns all notes across tiers", () => {
  createNote(paths, { workspace: "research", tier: "axioms", slug: "a1", body: "x" });
  createNote(paths, { workspace: "research", tier: "projects", slug: "p1", body: "y" });
  createNote(paths, { workspace: "research", tier: "decisions", slug: "d1", body: "z" });
  const notes = listNotes(paths, "research");
  expect(notes.length).toBe(3);
  const tiers = new Set(notes.map((n) => n.tier));
  expect(tiers.has("axioms")).toBe(true);
  expect(tiers.has("projects")).toBe(true);
  expect(tiers.has("decisions")).toBe(true);
});

test("computeContentSha: stable hash of body", () => {
  const sha1 = computeContentSha("hello");
  const sha2 = computeContentSha("hello");
  const sha3 = computeContentSha("world");
  expect(sha1).toBe(sha2);
  expect(sha1).not.toBe(sha3);
  expect(sha1).toMatch(/^[0-9a-f]{64}$/);
});

test("slugify: kebab-case, alphanumeric only", () => {
  expect(slugify("Hello World!")).toBe("hello-world");
  expect(slugify("Auth — is required!")).toBe("auth-is-required");
  expect(slugify("---")).toBe("note");
});

test("createNote: persists supersedes list", () => {
  const note = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "v2",
    body: "new version",
    supersedes: ["old-note-id-1", "old-note-id-2"],
  });
  const read = readNote(paths, "research", "axioms", "v2");
  expect(read?.supersedes).toEqual(["old-note-id-1", "old-note-id-2"]);
});
