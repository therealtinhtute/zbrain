import { type RuntimePaths } from "./runtime-paths";
import { QmdAdapter, type QmdSearchResult } from "./qmd-adapter";
import { rankRetrievalResults, type RankedRetrievalResult } from "./retrieval-ranking";
import { generateCurrentTaskMarkdown, writeCurrentTask } from "./current-task";
import { parseQuery } from "./query-parser";
import { resolveSecondaryWorkspaces } from "./secondary-resolver";
import { type SecondaryWorkspaceEntry } from "../schemas/config";

const DEFAULT_SECONDARY_LIMIT = 3;

export interface RetrievalContext {
  results: RankedRetrievalResult[];
  markdown: string;
  filePath: string;
}

export interface RetrievalAdapter {
  searchWorkspace(options: { workspace: string; query: string; limit?: number }): QmdSearchResult[];
}

export interface MultiWorkspaceRetrievalOptions {
  primaryWorkspace: string;
  query: string;
  secondaries: SecondaryWorkspaceEntry[];
  workspacesDir: string;
  limit?: number;
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

export function retrieveMultiWorkspaceContext(
  paths: RuntimePaths,
  options: MultiWorkspaceRetrievalOptions,
  adapter: RetrievalAdapter = new QmdAdapter(paths),
): RetrievalContext {
  const totalLimit = options.limit ?? 8;

  const { cleanQuery, secondaryWorkspaces: secondaryNames } = parseQuery(options.query, options.secondaries);
  const { resolved, warnings } = resolveSecondaryWorkspaces(options.workspacesDir, secondaryNames);

  for (const warning of warnings) {
    console.warn(`[zbrain] ${warning}`);
  }

  // Build limit map: config entry limit takes precedence; default for @tag-only workspaces
  const limitMap = new Map<string, number>();
  for (const entry of options.secondaries) {
    limitMap.set(entry.workspace, entry.limit ?? DEFAULT_SECONDARY_LIMIT);
  }

  // Query primary workspace
  const primaryRaw = adapter.searchWorkspace({
    workspace: options.primaryWorkspace,
    query: cleanQuery,
    limit: totalLimit,
  });
  const primaryRanked = rankRetrievalResults(primaryRaw, totalLimit);

  const allResults: RankedRetrievalResult[] = [...primaryRanked];
  let remaining = totalLimit - primaryRanked.length;
  let remainingSecondaries = resolved.length;

  for (const name of resolved) {
    if (remaining <= 0) break;

    const entryLimit = limitMap.get(name) ?? DEFAULT_SECONDARY_LIMIT;
    const slots = Math.min(entryLimit, Math.floor(remaining / remainingSecondaries) || 1);

    const secondaryRaw = adapter.searchWorkspace({ workspace: name, query: cleanQuery, limit: slots });
    const secondaryRanked = rankRetrievalResults(secondaryRaw, slots).map((r) => ({ ...r, workspace: name }));

    allResults.push(...secondaryRanked);
    remaining -= secondaryRanked.length;
    remainingSecondaries -= 1;
  }

  const resolvedSecondaries = resolved.length > 0 ? resolved : undefined;

  const markdown = generateCurrentTaskMarkdown({
    query: options.query,
    workspace: options.primaryWorkspace,
    secondaryWorkspaces: resolvedSecondaries,
    results: allResults,
  });
  const filePath = writeCurrentTask(paths, markdown);

  return { results: allResults, markdown, filePath };
}
