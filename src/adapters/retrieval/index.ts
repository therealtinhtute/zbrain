// RetrievalAdapter registry (AC-P3-1).
// Selects between FTS5 (default) and QMD via the active config.
// The adapter contract is preserved across both backends so callers don't change.

import type { Database } from "bun:sqlite";
import type { RuntimePaths } from "../../core/runtime-paths";
import { Fts5Adapter } from "./fts5-adapter";
import { QmdAdapter, type QmdSearchResult } from "../../core/qmd-adapter";

export type { QmdSearchResult };

export interface RetrievalAdapter {
  searchWorkspace(options: { workspace: string; query: string; limit?: number }): QmdSearchResult[];
}

export type RetrievalEngine = "fts5" | "qmd";

export interface AdapterFactory {
  create(db: Database, paths: RuntimePaths): RetrievalAdapter;
}

class Fts5Factory implements AdapterFactory {
  create(db: Database, paths: RuntimePaths): RetrievalAdapter {
    const fts = new Fts5Adapter(db, paths);
    return {
      searchWorkspace: ({ workspace, query, limit }) => {
        const result = fts.search({ workspace, query, limit: limit ?? 20 });
        // Convert Fts5SearchHit -> QmdSearchResult shape (downstream consumers expect it).
        return result.hits.map((h) => ({
          path: h.path,
          score: h.score,
          snippet: h.body.slice(0, 240),
          body: h.body,
        }));
      },
    };
  }
}

class QmdFactory implements AdapterFactory {
  create(_db: Database, paths: RuntimePaths): RetrievalAdapter {
    return new QmdAdapter(paths);
  }
}

const REGISTRY: Record<RetrievalEngine, AdapterFactory> = {
  fts5: new Fts5Factory(),
  qmd: new QmdFactory(),
};

export function createRetrievalAdapter(db: Database, paths: RuntimePaths, engine: RetrievalEngine = "fts5"): RetrievalAdapter {
  const factory = REGISTRY[engine] ?? REGISTRY.fts5;
  return factory.create(db, paths);
}
