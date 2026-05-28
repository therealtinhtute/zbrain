import { type SecondaryWorkspaceEntry } from "../schemas/config";

const TAG_REGEX = /@([a-zA-Z0-9_-]+)/g;

export interface ParsedQuery {
  cleanQuery: string;
  secondaryWorkspaces: string[];
}

export function extractWorkspaceTags(query: string): { tags: string[]; cleanQuery: string } {
  const tags: string[] = [];
  const cleanQuery = query
    .replace(TAG_REGEX, (_, name: string) => {
      tags.push(name);
      return " ";
    })
    .replace(/\s+/g, " ")
    .trim();
  return { tags, cleanQuery };
}

export function matchKeywordWorkspaces(query: string, secondaries: SecondaryWorkspaceEntry[]): string[] {
  const lower = query.toLowerCase();
  const matched: string[] = [];
  for (const entry of secondaries) {
    const hits = entry.keywords.some((kw) => lower.includes(kw.toLowerCase()));
    if (hits && !matched.includes(entry.workspace)) {
      matched.push(entry.workspace);
    }
  }
  return matched;
}

export function parseQuery(query: string, secondaries: SecondaryWorkspaceEntry[]): ParsedQuery {
  const { tags, cleanQuery } = extractWorkspaceTags(query);
  const keywordMatches = matchKeywordWorkspaces(query, secondaries);
  const secondaryWorkspaces = [...new Set([...tags, ...keywordMatches])];
  return { cleanQuery, secondaryWorkspaces };
}
