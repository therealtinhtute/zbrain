// V2 indexer: walks `wiki/`, parses notes, upserts to DB + FTS5 + links.
// File-first invariant: every mutation begins with a successful file write
// before the DB transaction. DB is the cache; files are truth.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import type { Database } from "bun:sqlite";
import { listNotes, readNote, relNotePath, type Note } from "./note-service";
import { isWikiTier, WikiTiers, type WikiTier } from "./workspace-layout";
import { wikiRoot, type RuntimePaths } from "./runtime-paths";

export interface IndexerOptions {
  paths: RuntimePaths;
  workspace: string;
  db: Database;
  nowIso?: string;
}

export interface RebuildResult {
  added: number;
  updated: number;
  removed: number;
  total: number;
}

export function upsertNote(options: IndexerOptions, note: Note): void {
  const nowIso = options.nowIso ?? new Date().toISOString();
  const tx = options.db.transaction(() => {
    options.db.prepare(`
      INSERT INTO notes (id, workspace, path, tier, status, title, content_sha, created_at, updated_at, review_by)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT (id, workspace) DO UPDATE SET
        path = excluded.path,
        tier = excluded.tier,
        status = excluded.status,
        title = excluded.title,
        content_sha = excluded.content_sha,
        updated_at = excluded.updated_at,
        review_by = excluded.review_by
    `).run(
      note.id,
      note.workspace,
      note.relPath,
      note.tier,
      note.status,
      note.title,
      note.contentSha,
      note.createdAt,
      nowIso,
      note.reviewBy,
    );

    // Rebuild FTS5 entry: delete existing map row + FTS rowid, then insert.
    const existingMap = options.db.prepare(`
      SELECT rowid FROM note_fts_map WHERE workspace = ? AND note_id = ?
    `).get(note.workspace, note.id) as { rowid: number } | null;
    if (existingMap) {
      options.db.prepare(`DELETE FROM note_fts WHERE rowid = ?`).run(existingMap.rowid);
      options.db.prepare(`DELETE FROM note_fts_map WHERE rowid = ?`).run(existingMap.rowid);
    }
    const ftsResult = options.db.prepare(`
      INSERT INTO note_fts (title, body) VALUES (?, ?)
    `).run(note.title, note.body);
    const rowid = Number(ftsResult.lastInsertRowid);
    options.db.prepare(`
      INSERT INTO note_fts_map (rowid, note_id, workspace) VALUES (?, ?, ?)
    `).run(rowid, note.id, note.workspace);

    // Rebuild link rows for this note's `supersedes`.
    options.db.prepare(`DELETE FROM links WHERE from_id = ? AND workspace = ? AND type = 'supersedes'`).run(note.id, note.workspace);
    for (const target of note.supersedes) {
      options.db.prepare(`
        INSERT OR IGNORE INTO links (from_id, workspace, type, to_id) VALUES (?, ?, 'supersedes', ?)
      `).run(note.id, note.workspace, target);
    }
  });
  tx();
}

export function removeNote(options: IndexerOptions, noteId: string): void {
  const tx = options.db.transaction(() => {
    const existingMap = options.db.prepare(`
      SELECT rowid FROM note_fts_map WHERE workspace = ? AND note_id = ?
    `).get(options.workspace, noteId) as { rowid: number } | null;
    if (existingMap) {
      options.db.prepare(`DELETE FROM note_fts WHERE rowid = ?`).run(existingMap.rowid);
      options.db.prepare(`DELETE FROM note_fts_map WHERE rowid = ?`).run(existingMap.rowid);
    }
    options.db.prepare(`DELETE FROM notes WHERE id = ? AND workspace = ?`).run(noteId, options.workspace);
    options.db.prepare(`DELETE FROM links WHERE from_id = ? AND workspace = ?`).run(noteId, options.workspace);
    options.db.prepare(`DELETE FROM links WHERE to_id = ? AND workspace = ?`).run(noteId, options.workspace);
  });
  tx();
}

export function rebuildWorkspace(options: IndexerOptions): RebuildResult {
  const wsRoot = wikiRoot(options.paths, options.workspace);
  if (!existsSync(wsRoot)) {
    return { added: 0, updated: 0, removed: 0, total: 0 };
  }

  const filesOnDisk = new Set<string>();
  for (const tier of WikiTiers) {
    const tierDir = join(wsRoot, tier);
    if (!existsSync(tierDir)) continue;
    for (const entry of readdirSync(tierDir, { withFileTypes: true })) {
      if (!entry.isFile() || !entry.name.endsWith(".md")) continue;
      const slug = entry.name.replace(/\.md$/, "");
      filesOnDisk.add(`${tier}/${slug}.md`);
    }
  }

  const dbRows = options.db.prepare(`
    SELECT path FROM notes WHERE workspace = ?
  `).all(options.workspace) as Array<{ path: string }>;
  const filesInDb = new Set(dbRows.map((r) => r.path));

  let added = 0;
  let updated = 0;
  let removed = 0;

  // Upsert every file on disk.
  for (const tier of WikiTiers) {
    const tierDir = join(wsRoot, tier);
    if (!existsSync(tierDir)) continue;
    for (const entry of readdirSync(tierDir, { withFileTypes: true })) {
      if (!entry.isFile() || !entry.name.endsWith(".md")) continue;
      const slug = entry.name.replace(/\.md$/, "");
      const note = readNote(options.paths, options.workspace, tier, slug);
      if (!note) continue;
      const wasInDb = filesInDb.has(note.relPath);
      upsertNote(options, note);
      if (wasInDb) updated += 1;
      else added += 1;
    }
  }

  // Remove DB rows for files that vanished.
  for (const relPath of filesInDb) {
    if (!filesOnDisk.has(relPath)) {
      const row = options.db.prepare(`
        SELECT id FROM notes WHERE workspace = ? AND path = ?
      `).get(options.workspace, relPath) as { id: string } | null;
      if (row) {
        removeNote(options, row.id);
        removed += 1;
      }
    }
  }

  return { added, updated, removed, total: filesOnDisk.size };
}
