import { type RuntimePaths } from "./runtime-paths";
import { QmdAdapter, type QmdSearchResult } from "./qmd-adapter";
import { rankRetrievalResults, type RankedRetrievalResult } from "./retrieval-ranking";
import { generateCurrentTaskMarkdown, writeCurrentTask } from "./current-task";

export interface RetrievalContext {
  results: RankedRetrievalResult[];
  markdown: string;
  filePath: string;
}

export interface RetrievalAdapter {
  searchWorkspace(options: { workspace: string; query: string; limit?: number }): QmdSearchResult[];
}

export function retrieveWorkspaceContext(
  paths: RuntimePaths,
  options: { workspace: string; query: string; limit?: number },
  adapter: RetrievalAdapter = new QmdAdapter(paths),
): RetrievalContext {
  const rawResults = adapter.searchWorkspace(options);
  const ranked = rankRetrievalResults(rawResults, options.limit ?? 8);
  const markdown = generateCurrentTaskMarkdown({
    query: options.query,
    workspace: options.workspace,
    results: ranked,
  });
  const filePath = writeCurrentTask(paths, markdown);

  return {
    results: ranked,
    markdown,
    filePath,
  };
}
