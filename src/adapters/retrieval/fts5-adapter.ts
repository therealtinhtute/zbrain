// FTS5 (BM25) retrieval adapter. Default retrieval engine for V2.
// `qmd` adapter stays available via `RetrievalAdapter` selection.

import type { Database } from "bun:sqlite";
import type { RuntimePaths } from "../../core/runtime-paths";

export interface Fts5SearchHit {
  id: string;
  workspace: string;
  path: string;
  tier: string;
  status: string;
  title: string | null;
  body: string;
  score: number; // higher = better
}

export interface Fts5SearchOptions {
  workspace: string;
  query: string;
  limit?: number;
  statusFilter?: string[]; // defaults to ['active']
}

export interface Fts5SearchResult {
  hits: Fts5SearchHit[];
}

export class Fts5Adapter {
  constructor(private db: Database, private paths: RuntimePaths) {}

  search(options: Fts5SearchOptions): Fts5SearchResult {
    const limit = options.limit && options.limit > 0 ? options.limit : 20;
    const statusFilter = options.statusFilter ?? ["active"];
    const placeholders = statusFilter.map(() => "?").join(",");

    // FTS5 BM25: `rank` is negative; lower rank = better match.
    // We normalize so higher score = better (multiplier of -1) and apply
    // tier weight to match the legacy classifier order.
    const sql = `
      SELECT
        n.id            AS id,
        n.workspace     AS workspace,
        n.path          AS path,
        n.tier          AS tier,
        n.status        AS status,
        n.title         AS title,
        fts.body        AS body,
        fts.rank        AS fts_rank
      FROM note_fts fts
      JOIN note_fts_map m ON fts.rowid = m.rowid
      JOIN notes n ON n.id = m.note_id AND n.workspace = m.workspace
      WHERE fts.note_fts MATCH ?
        AND n.workspace = ?
        AND n.status IN (${placeholders})
      ORDER BY fts.rank ASC
      LIMIT ?
    `;
    const params: (string | number)[] = [escapeFtsQuery(options.query), options.workspace, ...statusFilter, limit];
    const rows = this.db.prepare(sql).all(...params) as Array<{
      id: string;
      workspace: string;
      path: string;
      tier: string;
      status: string;
      title: string | null;
      body: string;
      fts_rank: number;
    }>;
    const hits: Fts5SearchHit[] = rows.map((row) => ({
      id: row.id,
      workspace: row.workspace,
      path: row.path,
      tier: row.tier,
      status: row.status,
      title: row.title,
      body: row.body,
      // FTS5 rank is negative (more negative = better); flip sign.
      // Score is then weighted by tier at ranking time.
      score: -row.fts_rank,
    }));
    return { hits };
  }
}

// Escape user input for FTS5 MATCH: wrap in double quotes, escape any
// embedded double quotes. This is a conservative sanitizer; richer query
// syntax (NEAR, column filters) is not exposed.
function escapeFtsQuery(input: string): string {
  const trimmed = input.trim();
  if (trimmed.length === 0) return '""';
  return `"${trimmed.replace(/"/g, '""')}"`;
}

export const TIER_WEIGHTS: Record<string, number> = {
  axioms: 1.5,
  "mental-models": 1.3,
  projects: 1.1,
  decisions: 1.0,
};
