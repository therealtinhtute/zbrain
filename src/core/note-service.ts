// V2 Note service: file-first CRUD for memory notes.
// Notes are markdown files under `<ws>/wiki/<tier>/<slug>.md` with YAML frontmatter.
// The DB is a derived cache, rebuildable from files via `indexer.rebuild()`.

import { createHash, randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { parseFrontmatter, serializeMarkdown } from "./frontmatter";
import { assertTransition, InvalidTransitionError } from "./lifecycle";
import { isWikiTier, type WikiTier } from "./workspace-layout";
import { wikiTierPath, workspaceRoot, type RuntimePaths } from "./runtime-paths";

export type NoteStatus = "active" | "superseded" | "archived" | "forgotten";
export const NoteStatuses: NoteStatus[] = ["active", "superseded", "archived", "forgotten"];

export interface Note {
  id: string;
  workspace: string;
  tier: WikiTier;
  status: NoteStatus;
  path: string;
  relPath: string;
  title: string;
  body: string;
  contentSha: string;
  createdAt: string;
  updatedAt: string;
  sources: string[];
  supersedes: string[];
  supersededBy: string | null;
  reviewBy: string | null;
}

export interface CreateNoteInput {
  workspace: string;
  tier: WikiTier;
  slug: string;
  body: string;
  title?: string;
  sources?: string[];
  supersedes?: string[];
  reviewBy?: string;
  nowIso?: string;
}

export class ShaMismatchError extends Error {
  constructor(public readonly current: string, public readonly expected: string) {
    super(`content_sha mismatch: expected ${expected}, current ${current}`);
    this.name = "ShaMismatchError";
  }
}

export class NotFoundError extends Error {
  constructor(public readonly noteId: string) {
    super(`Note not found: ${noteId}`);
    this.name = "NotFoundError";
  }
}

export function generateNoteId(): string {
  return randomUUID();
}

export function computeContentSha(body: string): string {
  return createHash("sha256").update(body).digest("hex");
}

export function slugify(input: string): string {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60) || "note";
}

export function notePath(paths: RuntimePaths, workspace: string, tier: WikiTier, slug: string): string {
  return join(wikiTierPath(paths, workspace, tier), `${slug}.md`);
}

export function relNotePath(tier: WikiTier, slug: string): string {
  return `${tier}/${slug}.md`;
}

function asString(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function asStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((x): x is string => typeof x === "string");
}

export function readNote(
  paths: RuntimePaths,
  workspace: string,
  tier: WikiTier,
  slug: string,
): Note | null {
  const path = notePath(paths, workspace, tier, slug);
  if (!existsSync(path)) return null;
  const raw = readFileSync(path, "utf8");
  const { frontmatter, body } = parseFrontmatter(raw);
  const id = asString(frontmatter.id) ?? generateNoteId();
  const fmTier = asString(frontmatter.tier);
  const tierFm: WikiTier = fmTier && isWikiTier(fmTier) ? fmTier : tier;
  const fmStatus = asString(frontmatter.status);
  const status: NoteStatus = fmStatus && (NoteStatuses as string[]).includes(fmStatus)
    ? (fmStatus as NoteStatus)
    : "active";
  return {
    id,
    workspace,
    tier: tierFm,
    status,
    path,
    relPath: relNotePath(tierFm, slug),
    title: asString(frontmatter.title) ?? "",
    body,
    contentSha: computeContentSha(body),
    createdAt: asString(frontmatter.created_at) ?? "",
    updatedAt: asString(frontmatter.updated_at) ?? "",
    sources: asStringArray(frontmatter.sources),
    supersedes: asStringArray(frontmatter.supersedes),
    supersededBy: asString(frontmatter.superseded_by),
    reviewBy: asString(frontmatter.review_by),
  };
}

function writeNoteToFile(note: Note, frontmatterOverride: Record<string, unknown>): void {
  const fm = {
    id: note.id,
    tier: note.tier,
    status: frontmatterOverride.status ?? note.status,
    title: note.title,
    created_at: note.createdAt,
    updated_at: frontmatterOverride.updated_at ?? note.updatedAt,
    content_sha: frontmatterOverride.content_sha ?? note.contentSha,
    sources: frontmatterOverride.sources ?? note.sources,
    supersedes: frontmatterOverride.supersedes ?? note.supersedes,
    superseded_by: frontmatterOverride.superseded_by ?? note.supersededBy,
    review_by: frontmatterOverride.review_by ?? note.reviewBy,
  };
  const markdown = serializeMarkdown(fm, note.body);
  writeFileSync(note.path, markdown, "utf8");
}

export function createNote(paths: RuntimePaths, input: CreateNoteInput): Note {
  if (!isWikiTier(input.tier)) {
    throw new Error(`Invalid tier: ${input.tier}`);
  }
  const path = notePath(paths, input.workspace, input.tier, input.slug);
  if (existsSync(path)) {
    throw new Error(`Note already exists: ${path}`);
  }
  const nowIso = input.nowIso ?? new Date().toISOString();
  const id = generateNoteId();
  const title = input.title ?? input.slug;
  const body = `# ${title}\n\n${input.body}\n`;
  const contentSha = computeContentSha(body);
  const frontmatter = {
    id,
    tier: input.tier,
    status: "active" as NoteStatus,
    title,
    created_at: nowIso,
    updated_at: nowIso,
    content_sha: contentSha,
    sources: input.sources ?? [],
    supersedes: input.supersedes ?? [],
    superseded_by: null,
    review_by: input.reviewBy ?? null,
  };
  const markdown = serializeMarkdown(frontmatter, body);
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, markdown, "utf8");
  return {
    id,
    workspace: input.workspace,
    tier: input.tier,
    status: "active",
    path,
    relPath: relNotePath(input.tier, input.slug),
    title,
    body,
    contentSha,
    createdAt: nowIso,
    updatedAt: nowIso,
    sources: input.sources ?? [],
    supersedes: input.supersedes ?? [],
    supersededBy: null,
    reviewBy: input.reviewBy ?? null,
  };
}

export function listNotes(paths: RuntimePaths, workspace: string): Note[] {
  const notes: Note[] = [];
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as WikiTier[]) {
    const tierDir = wikiTierPath(paths, workspace, tier);
    if (!existsSync(tierDir)) continue;
    for (const entry of readdirSyncSafe(tierDir)) {
      if (!entry.isFile() || !entry.name.endsWith(".md")) continue;
      const slug = entry.name.replace(/\.md$/, "");
      const note = readNote(paths, workspace, tier, slug);
      if (note) notes.push(note);
    }
  }
  return notes;
}

import { readdirSync } from "node:fs";
function readdirSyncSafe(dir: string) {
  return readdirSync(dir, { withFileTypes: true });
}

export interface SupersedeOptions {
  newBody: string;
  newTitle?: string;
  newSlug?: string;
  nowIso?: string;
  expectedSha?: string;
}

export function supersedeNote(
  paths: RuntimePaths,
  oldNote: Note,
  options: SupersedeOptions,
): { oldNote: Note; newNote: Note } {
  assertTransition(oldNote.status, "superseded");
  if (options.expectedSha && options.expectedSha !== oldNote.contentSha) {
    throw new ShaMismatchError(oldNote.contentSha, options.expectedSha);
  }
  const nowIso = options.nowIso ?? new Date().toISOString();
  const newId = generateNoteId();
  const newSlug = options.newSlug ?? `${oldNote.tier === "axioms" ? "axiom" : "note"}-${newId.slice(0, 8)}`;
  const newTitle = options.newTitle ?? oldNote.title;
  const newBody = `# ${newTitle}\n\n${options.newBody}\n`;
  const newContentSha = computeContentSha(newBody);

  // 1. Create the new note.
  const newPath = notePath(paths, oldNote.workspace, oldNote.tier, newSlug);
  if (existsSync(newPath)) {
    throw new Error(`Note already exists: ${newPath}`);
  }
  mkdirSync(dirname(newPath), { recursive: true });
  const newFrontmatter = {
    id: newId,
    tier: oldNote.tier,
    status: "active" as NoteStatus,
    title: newTitle,
    created_at: nowIso,
    updated_at: nowIso,
    content_sha: newContentSha,
    sources: oldNote.sources,
    supersedes: [oldNote.id],
    superseded_by: null,
    review_by: null,
  };
  writeFileSync(newPath, serializeMarkdown(newFrontmatter, newBody), "utf8");

  // 2. Flip the old note.
  const flipped: Note = {
    ...oldNote,
    status: "superseded",
    supersededBy: newId,
    updatedAt: nowIso,
  };
  writeNoteToFile(flipped, {
    status: "superseded",
    superseded_by: newId,
    updated_at: nowIso,
  });

  return {
    oldNote: flipped,
    newNote: {
      id: newId,
      workspace: oldNote.workspace,
      tier: oldNote.tier,
      status: "active",
      path: newPath,
      relPath: relNotePath(oldNote.tier, newSlug),
      title: newTitle,
      body: newBody,
      contentSha: newContentSha,
      createdAt: nowIso,
      updatedAt: nowIso,
      sources: oldNote.sources,
      supersedes: [oldNote.id],
      supersededBy: null,
      reviewBy: null,
    },
  };
}

export function archiveNote(
  paths: RuntimePaths,
  note: Note,
  options: { nowIso?: string } = {},
): Note {
  assertTransition(note.status, "archived");
  const nowIso = options.nowIso ?? new Date().toISOString();
  const next: Note = { ...note, status: "archived", updatedAt: nowIso };
  writeNoteToFile(next, { status: "archived", updated_at: nowIso });
  return next;
}

export interface ForgetResult {
  tombstonePath: string;
  forgotten: Note;
}

export function forgetNote(
  paths: RuntimePaths,
  note: Note,
  options: { reason?: string; nowIso?: string } = {},
): ForgetResult {
  // Allow forgetting from any non-forgotten status.
  if (note.status === "forgotten") {
    throw new InvalidTransitionError("forgotten", "forgotten");
  }
  const nowIso = options.nowIso ?? new Date().toISOString();
  const wsRoot = workspaceRoot(paths, note.workspace);
  const trashDir = join(wsRoot, ".trash");
  mkdirSync(trashDir, { recursive: true });
  const tombstonePath = join(trashDir, `${note.id}.md`);
  const tombstone = [
    `# forgotten ${note.id} at ${nowIso}`,
    `reason: ${options.reason ?? ""}`,
    `original_path: ${note.relPath}`,
    `original_tier: ${note.tier}`,
    "",
  ].join("\n");
  writeFileSync(tombstonePath, tombstone, "utf8");
  if (existsSync(note.path)) {
    renameSync(note.path, join(trashDir, `${note.id}.md.bak`));
  }
  return {
    tombstonePath,
    forgotten: { ...note, status: "forgotten", updatedAt: nowIso },
  };
}

export function restoreNote(
  paths: RuntimePaths,
  workspace: string,
  noteId: string,
  options: { nowIso?: string } = {},
): Note {
  const wsRoot = workspaceRoot(paths, workspace);
  const trashDir = join(wsRoot, ".trash");
  const tombstonePath = join(trashDir, `${noteId}.md`);
  const backupPath = join(trashDir, `${noteId}.md.bak`);
  if (!existsSync(tombstonePath) || !existsSync(backupPath)) {
    throw new NotFoundError(noteId);
  }
  const tombstone = readFileSync(tombstonePath, "utf8");
  const originalTierMatch = tombstone.match(/^original_tier:\s*(.+)$/m);
  const originalPathMatch = tombstone.match(/^original_path:\s*(.+)$/m);
  if (!originalTierMatch || !originalPathMatch) {
    throw new Error(`Tombstone missing original_tier or original_path: ${noteId}`);
  }
  const originalTier = originalTierMatch[1].trim();
  const originalRelPath = originalPathMatch[1].trim();
  if (!isWikiTier(originalTier)) {
    throw new Error(`Invalid original_tier: ${originalTier}`);
  }
  const slug = originalRelPath.replace(/^.*\//, "").replace(/\.md$/, "");
  const targetPath = notePath(paths, workspace, originalTier, slug);
  mkdirSync(dirname(targetPath), { recursive: true });
  renameSync(backupPath, targetPath);
  // Remove tombstone.
  const { unlinkSync } = require("node:fs");
  try { unlinkSync(tombstonePath); } catch {}
  const restored = readNote(paths, workspace, originalTier, slug);
  if (!restored) throw new Error(`Restoration failed: ${targetPath}`);
  // Flip status to active.
  const nowIso = options.nowIso ?? new Date().toISOString();
  const next: Note = { ...restored, status: "active", updatedAt: nowIso };
  writeNoteToFile(next, { status: "active", updated_at: nowIso });
  return next;
}

