// V2 Note service: file-first CRUD for memory notes.
// Notes are markdown files under `<ws>/wiki/<tier>/<slug>.md` with YAML frontmatter.
// The DB is a derived cache, rebuildable from files via `indexer.rebuild()`.

import { createHash, randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { parseFrontmatter, serializeMarkdown } from "./frontmatter";
import { isWikiTier, type WikiTier } from "./workspace-layout";
import { wikiTierPath, type RuntimePaths } from "./runtime-paths";

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
    for (const entry of require("node:fs").readdirSync(tierDir, { withFileTypes: true })) {
      if (!entry.isFile() || !entry.name.endsWith(".md")) continue;
      const slug = entry.name.replace(/\.md$/, "");
      const note = readNote(paths, workspace, tier, slug);
      if (note) notes.push(note);
    }
  }
  return notes;
}
