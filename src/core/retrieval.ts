import { type RuntimePaths } from "./runtime-paths";
import { QmdAdapter, type QmdSearchResult } from "./qmd-adapter";
import { rankRetrievalResults, type RankedRetrievalResult } from "./retrieval-ranking";
import { generateCurrentTaskMarkdown } from "./current-task";
import { writeSessionContext, getSessionId } from "./session";
import { parseQuery } from "./query-parser";
import { resolveSecondaryWorkspaces } from "./secondary-resolver";
import { type SecondaryWorkspaceEntry } from "../schemas/config";

const DEFAULT_SECONDARY_LIMIT = 3;

export interface RetrievalContext {
  results: RankedRetrievalResult[];
  markdown: string;
  filePath: string;
  sessionId: string;
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
  nowIso?: string;
  sessionId?: string;
}

export interface RetrieveWorkspaceContextOptions {
  workspace: string;
  query: string;
  limit?: number;
  nowIso?: string;
  sessionId?: string;
}

export function retrieveWorkspaceContext(
  paths: RuntimePaths,
  options: RetrieveWorkspaceContextOptions,
  adapter: RetrievalAdapter = new QmdAdapter(paths),
): RetrievalContext {
  const sessionId = options.sessionId ?? getSessionId();
  const rawResults = adapter.searchWorkspace(options);
  const ranked = rankRetrievalResults(rawResults, options.limit ?? 8);
  const markdown = generateCurrentTaskMarkdown({
    query: options.query,
    workspace: options.workspace,
    results: ranked,
    nowIso: options.nowIso,
  });
  // V2 multi-agent fix: write to per-session file instead of shared
  // current-task.md (which clobbered between parallel agents).
  const filePath = writeSessionContext(paths, paths.cwd, sessionId, markdown);

  return {
    results: ranked,
    markdown,
    filePath,
    sessionId,
  };
}

export function retrieveMultiWorkspaceContext(
  paths: RuntimePaths,
  options: MultiWorkspaceRetrievalOptions,
  adapter: RetrievalAdapter = new QmdAdapter(paths),
): RetrievalContext {
  const totalLimit = options.limit ?? 8;

  const { cleanQuery, secondaryWorkspaces: secondaryNames, tags } = parseQuery(options.query, options.secondaries);
  const { resolved, warnings } = resolveSecondaryWorkspaces(options.workspacesDir, secondaryNames);
  const taggedSet = new Set(tags);

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

  for (let i = 0; i < resolved.length; i += 1) {
    const name = resolved[i];
    const remainingSecondaries = resolved.length - i;
    const isTagged = taggedSet.has(name);

    // Keyword-only secondaries yield when the primary saturated the limit;
    // explicit @tag secondaries always keep a floor of one slot.
    if (remaining <= 0 && !isTagged) continue;

    const entryLimit = limitMap.get(name) ?? DEFAULT_SECONDARY_LIMIT;
    let slots = Math.min(entryLimit, remaining, Math.ceil(remaining / remainingSecondaries));
    if (isTagged) slots = Math.max(slots, 1);
    if (slots <= 0) continue;

    const secondaryRaw = adapter.searchWorkspace({ workspace: name, query: cleanQuery, limit: slots });
    const secondaryRanked = rankRetrievalResults(secondaryRaw, slots).map((r) => ({ ...r, workspace: name }));

    allResults.push(...secondaryRanked);
    remaining -= secondaryRanked.length;
  }

  // Dedup merged results by workspace:path (primary rows key under the primary workspace).
  const seenKeys = new Set<string>();
  const dedupedResults = allResults.filter((result) => {
    const key = `${result.workspace ?? options.primaryWorkspace}:${result.path}`;
    if (seenKeys.has(key)) return false;
    seenKeys.add(key);
    return true;
  });

  const resolvedSecondaries = resolved.length > 0 ? resolved : undefined;

  const markdown = generateCurrentTaskMarkdown({
    query: options.query,
    workspace: options.primaryWorkspace,
    secondaryWorkspaces: resolvedSecondaries,
    results: dedupedResults,
    nowIso: options.nowIso,
  });
  const sessionId = options.sessionId ?? getSessionId();
  // V2 multi-agent fix: per-session file (not shared current-task.md).
  const filePath = writeSessionContext(paths, paths.cwd, sessionId, markdown);

  return { results: dedupedResults, markdown, filePath, sessionId };
}
