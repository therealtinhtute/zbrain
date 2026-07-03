// `zbrain note` CLI — add / update / archive / forget / restore / show.

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import {
  archiveNote,
  createNote,
  forgetNote,
  readNote,
  restoreNote,
  slugify,
  supersedeNote,
} from "../core/note-service";
import { detectConflict } from "../core/conflict";
import { assertTransition } from "../core/lifecycle";
import { NotFoundError } from "../core/note-service";
import { isWikiTier, type WikiTier } from "../core/workspace-layout";
import { initDb } from "../core/db";
import { upsertNote, removeNote } from "../core/indexer";
import { resolveWorkspaceName } from "../core/workspace-resolver";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface NoteCommandOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
}

async function readStdin(): Promise<string> {
  let contents = "";
  for await (const chunk of process.stdin) {
    contents += chunk;
  }
  return contents;
}

async function resolveBody(ui: CommandUi, options: { body?: string; file?: string }): Promise<string> {
  if (typeof options.body === "string") {
    return options.body;
  }
  if (options.file) {
    return readFileSync(options.file, "utf8");
  }
  if (!process.stdin.isTTY) {
    return readStdin();
  }
  return ui.text({
    message: "Note body",
    placeholder: "Write the note body (markdown)",
    validate: (value) => (value?.trim() ? undefined : "Note body is required."),
  });
}

export async function runNoteAdd(
  options: {
    tier: string;
    title: string;
    workspace?: string;
    slug?: string;
    body?: string;
    file?: string;
    sources?: string[];
  } & NoteCommandOptions,
): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);

  if (!isWikiTier(options.tier)) {
    throw new Error(`Invalid tier "${options.tier}". Must be one of: axioms, mental-models, projects, decisions.`);
  }
  const tier: WikiTier = options.tier;
  const workspace = resolveWorkspaceName(context.paths, options.workspace);
  const slug = options.slug ?? slugify(options.title);
  const body = await resolveBody(ui, options);

  const conflict = detectConflict(context.paths, workspace, tier, slug, []);
  if (conflict) {
    throw new Error(
      `Conflict: ${conflict.proposedPath} already exists (id: ${conflict.existing.id}). ` +
        `Use \`zbrain note update ${conflict.existing.id}\` to supersede it, or pass --slug with a different value.`,
    );
  }

  const note = createNote(context.paths, {
    workspace,
    tier,
    slug,
    body,
    title: options.title,
    sources: options.sources ?? [],
  });
  upsertNote({ paths: context.paths, workspace, db }, note);

  ui.note(`id: ${note.id}\npath: ${note.relPath}\ntier: ${note.tier}\nworkspace: ${workspace}`, "Note added");
}

export async function runNoteShow(id: string, options: NoteCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const workspaces = readdirSync(context.paths.workspacesDir, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => e.name);
  for (const ws of workspaces) {
    for (const tier of ["axioms", "mental-models", "projects", "decisions"] as WikiTier[]) {
      const tierDir = `${context.paths.workspacesDir}/${ws}/wiki/${tier}`;
      if (!existsSync(tierDir)) continue;
      for (const entry of readdirSync(tierDir)) {
        if (!entry.endsWith(".md")) continue;
        const slug = entry.replace(/\.md$/, "");
        const note = readNote(context.paths, ws, tier, slug);
        if (note && note.id === id) {
          ui.note(
            `id: ${note.id}\nstatus: ${note.status}\npath: ${note.relPath}\n\n---\n\n${note.body}`,
            `Note: ${note.title}`,
          );
          return;
        }
      }
    }
  }
  throw new NotFoundError(id);
}

export async function runNoteUpdate(
  id: string,
  options: { body: string; title?: string; slug?: string; reason?: string } & NoteCommandOptions,
): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  // Locate the note.
  const found = locateNote(context.paths, id);
  if (!found) throw new NotFoundError(id);
  // Only check for a path conflict when an explicit --slug is given; the
  // auto-generated slug (id-derived) can never collide with an existing note.
  if (options.slug) {
    const conflict = detectConflict(context.paths, found.workspace, found.tier, options.slug, []);
    if (conflict) {
      throw new Error(
        `Conflict: ${conflict.proposedPath} already exists (id: ${conflict.existing.id}). ` +
          `Pass a different --slug or declare supersedes.`,
      );
    }
  }
  const { oldNote, newNote } = supersedeNote(context.paths, found, {
    newBody: options.body,
    newTitle: options.title,
    newSlug: options.slug,
  });
  // Persist to DB cache.
  upsertNote({ paths: context.paths, workspace: oldNote.workspace, db }, newNote);
  upsertNote({ paths: context.paths, workspace: oldNote.workspace, db }, oldNote);
  ui.note(`old: ${oldNote.id} -> superseded\nnew: ${newNote.id} (active)\npath: ${newNote.relPath}`, "Supersede");
}

export async function runNoteArchive(id: string, options: NoteCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const found = locateNote(context.paths, id);
  if (!found) throw new NotFoundError(id);
  assertTransition(found.status, "archived");
  const archived = archiveNote(context.paths, found);
  upsertNote({ paths: context.paths, workspace: archived.workspace, db }, archived);
  ui.note(`archived: ${archived.id}\npath: ${archived.relPath}`, "Archive");
}

export async function runNoteForget(
  id: string,
  options: { reason?: string; purge?: boolean } & NoteCommandOptions,
): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const found = locateNote(context.paths, id);
  if (!found) throw new NotFoundError(id);
  const { tombstonePath, forgotten } = forgetNote(context.paths, found, { reason: options.reason });
  removeNote({ paths: context.paths, workspace: forgotten.workspace, db }, forgotten.id);
  ui.note(`tombstone: ${tombstonePath}\nid: ${forgotten.id}\nuse \`zbrain note restore ${forgotten.id}\` to undo.`, "Forget");
}

export async function runNoteRestore(
  id: string,
  options: NoteCommandOptions & { workspace?: string } = {},
): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const db = initDb(context.paths.runtimeDir);
  const workspace = options.workspace ?? firstWorkspace(context.paths);
  if (!workspace) throw new Error("No workspace specified and none found.");
  const restored = restoreNote(context.paths, workspace, id);
  upsertNote({ paths: context.paths, workspace: restored.workspace, db }, restored);
  ui.note(`restored: ${restored.id}\npath: ${restored.relPath}\nstatus: active`, "Restore");
}

function firstWorkspace(paths: any): string | null {
  if (!existsSync(paths.workspacesDir)) return null;
  for (const e of readdirSync(paths.workspacesDir, { withFileTypes: true })) {
    if (e.isDirectory()) return e.name;
  }
  return null;
}

function locateNote(paths: any, noteId: string) {
  if (!existsSync(paths.workspacesDir)) return null;
  for (const wsEntry of readdirSync(paths.workspacesDir, { withFileTypes: true })) {
    if (!wsEntry.isDirectory()) continue;
    for (const tier of ["axioms", "mental-models", "projects", "decisions"] as WikiTier[]) {
      const tierDir = `${paths.workspacesDir}/${wsEntry.name}/wiki/${tier}`;
      if (!existsSync(tierDir)) continue;
      for (const entry of readdirSync(tierDir)) {
        if (!entry.endsWith(".md")) continue;
        const slug = entry.replace(/\.md$/, "");
        const note = readNote(paths, wsEntry.name, tier, slug);
        if (note && note.id === noteId) return note;
      }
    }
  }
  return null;
}

export function registerNoteCommand(program: Command): void {
  const note = program
    .command("note")
    .description("Manage notes (add, update, archive, forget, restore, show)");

  note
    .command("add")
    .description("Add a note directly to the wiki (trusted fast path, conflict-checked)")
    .requiredOption("--tier <tier>", "axioms | mental-models | projects | decisions")
    .requiredOption("--title <text>", "note title")
    .option("--workspace <name>", "target workspace (default: resolved active workspace)")
    .option("--slug <text>", "slug (defaults to a slugified title)")
    .option("--body <text>", "note body (else --file, else stdin, else prompt)")
    .option("--file <path>", "read note body from a file")
    .option("--source <id...>", "evidence source id(s) this note is derived from")
    .action((options: any) => runNoteAdd({ ...options, sources: options.source }));

  note
    .command("show <id>")
    .description("Print a note by id")
    .action((id: string) => runNoteShow(id));

  note
    .command("update <id>")
    .description("Supersede a note with a new body (creates a new note, flips old)")
    .requiredOption("--body <text>", "new note body")
    .option("--title <text>", "new title")
    .option("--slug <text>", "new slug (defaults to a generated one)")
    .option("--reason <text>", "reason for supersede")
    .action((id: string, options: any) => runNoteUpdate(id, options));

  note
    .command("archive <id>")
    .description("Archive a note (out of retrieval, on disk)")
    .action((id: string) => runNoteArchive(id));

  note
    .command("forget <id>")
    .description("Forget a note (move to .trash/, recoverable via restore)")
    .option("--reason <text>", "reason for forgetting")
    .option("--purge", "hard delete (default: recoverable via restore)")
    .action((id: string, options: any) => runNoteForget(id, options));

  note
    .command("restore <id>")
    .description("Restore a forgotten note from .trash/")
    .option("--workspace <name>", "workspace to restore into (default: first workspace)")
    .action((id: string, options: any) => runNoteRestore(id, options));
}
