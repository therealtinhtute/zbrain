import { type SecondaryWorkspaceEntry } from "../schemas/config";

const TAG_REGEX = /@([a-zA-Z0-9_-]+)/g;

export interface ParsedQuery {
  cleanQuery: string;
  secondaryWorkspaces: string[];
  tags: string[];
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
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
  const matched: string[] = [];
  for (const entry of secondaries) {
    const hits = entry.keywords.some((kw) =>
      new RegExp("\\b" + escapeRegExp(kw) + "\\b", "i").test(query),
    );
    if (hits && !matched.includes(entry.workspace)) {
      matched.push(entry.workspace);
    }
  }
  return matched;
}

export function parseQuery(query: string, secondaries: SecondaryWorkspaceEntry[]): ParsedQuery {
  const { tags, cleanQuery } = extractWorkspaceTags(query);
  const keywordMatches = matchKeywordWorkspaces(cleanQuery, secondaries);
  const secondaryWorkspaces = [...new Set([...tags, ...keywordMatches])];
  return { cleanQuery, secondaryWorkspaces, tags };
}
