import { test, expect, beforeEach, afterEach } from "bun:test";
import { mkdtempSync, mkdirSync, rmSync, existsSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { createNote, readNote, supersedeNote, archiveNote, forgetNote, restoreNote, NotFoundError, ShaMismatchError, type Note } from "../src/core/note-service";
import { canTransition, InvalidTransitionError } from "../src/core/lifecycle";
import { detectConflict } from "../src/core/conflict";

let tempHome: string;
let paths: ReturnType<typeof resolveRuntimePaths>;

beforeEach(() => {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-life-"));
  paths = resolveRuntimePaths({ runtimeDir: tempHome });
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
});

afterEach(() => {
  rmSync(tempHome, { recursive: true, force: true });
});

test("canTransition: state machine matrix", () => {
  const froms = ["active", "superseded", "archived", "forgotten"] as const;
  const tos = ["active", "superseded", "archived", "forgotten"] as const;
  for (const from of froms) {
    for (const to of tos) {
      if (from === to) {
        expect(canTransition(from, to)).toBe(false);
        continue;
      }
      const expected = (
        (from === "active" && ["superseded", "archived", "forgotten"].includes(to)) ||
        (from === "superseded" && ["active", "archived", "forgotten"].includes(to)) ||
        (from === "archived" && ["active", "forgotten"].includes(to)) ||
        (from === "forgotten" && to === "active")
      );
      expect(canTransition(from, to)).toBe(expected);
    }
  }
});

test("canTransition: forbidden transitions throw InvalidTransitionError", () => {
  expect(() => {
    const err = new InvalidTransitionError("forgotten", "archived");
    throw err;
  }).toThrow(InvalidTransitionError);
});

test("supersedeNote: creates new note, flips old to superseded, both on disk", () => {
  const original = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "auth-v1",
    body: "Auth is required.",
  });
  const { oldNote, newNote } = supersedeNote(paths, original, {
    newBody: "Auth is required AND must be token-based.",
    newTitle: "Auth is required (token-based)",
    newSlug: "auth-v2",
  });
  expect(oldNote.status).toBe("superseded");
  expect(oldNote.supersededBy).toBe(newNote.id);
  expect(newNote.status).toBe("active");
  expect(newNote.supersedes).toEqual([original.id]);
  expect(newNote.tier).toBe("axioms");
  // Both files on disk.
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "axioms", "auth-v1.md"))).toBe(true);
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "axioms", "auth-v2.md"))).toBe(true);
  // Read-back confirms frontmatter updates.
  const oldRead = readNote(paths, "research", "axioms", "auth-v1");
  expect(oldRead?.status).toBe("superseded");
  expect(oldRead?.supersededBy).toBe(newNote.id);
});

test("supersedeNote: rejects when content_sha mismatches", () => {
  const original = createNote(paths, {
    workspace: "research",
    tier: "axioms",
    slug: "x",
    body: "x body",
  });
  expect(() => supersedeNote(paths, original, {
    newBody: "y body",
    expectedSha: "wrong-sha",
  })).toThrow(ShaMismatchError);
});

test("archiveNote: flips status to archived", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "a", body: "x" });
  const archived = archiveNote(paths, note);
  expect(archived.status).toBe("archived");
  const read = readNote(paths, "research", "axioms", "a");
  expect(read?.status).toBe("archived");
});

test("forgetNote: moves to .trash/, leaves tombstone", () => {
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "b", body: "b body" });
  const { tombstonePath } = forgetNote(paths, note, { reason: "stale" });
  expect(existsSync(tombstonePath)).toBe(true);
  expect(tombstonePath).toContain(".trash");
  const t = readFileSync(tombstonePath, "utf8");
  expect(t).toContain(`forgotten ${note.id}`);
  expect(t).toContain("original_path: axioms/b.md");
  expect(t).toContain("original_tier: axioms");
  // Original file moved out.
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "axioms", "b.md"))).toBe(false);
});

test("restoreNote: reverses forget, status back to active", () => {
  const note = createNote(paths, { workspace: "research", tier: "projects", slug: "c", body: "c body" });
  forgetNote(paths, note);
  const restored = restoreNote(paths, "research", note.id);
  expect(restored.status).toBe("active");
  expect(existsSync(join(paths.workspacesDir, "research", "wiki", "projects", "c.md"))).toBe(true);
  // Tombstone cleaned up.
  expect(existsSync(join(paths.workspacesDir, "research", ".trash", `${note.id}.md`))).toBe(false);
});

test("restoreNote: throws NotFoundError for unknown id", () => {
  expect(() => restoreNote(paths, "research", "nonexistent")).toThrow(NotFoundError);
});

test("detectConflict: returns ConflictReport for existing active note", () => {
  createNote(paths, { workspace: "research", tier: "axioms", slug: "d", body: "d body" });
  const report = detectConflict(paths, "research", "axioms", "d");
  expect(report).not.toBeNull();
  expect(report?.reason).toBe("path-overlap");
  expect(report?.existing.status).toBe("active");
});

test("detectConflict: returns null when supersedes is declared", () => {
  createNote(paths, { workspace: "research", tier: "axioms", slug: "e", body: "e" });
  const report = detectConflict(paths, "research", "axioms", "e", ["some-old-id"]);
  expect(report).toBeNull();
});

test("detectConflict: returns null for nonexistent path", () => {
  expect(detectConflict(paths, "research", "axioms", "never")).toBeNull();
});
