import { spawnSync } from "node:child_process";
import { join, basename } from "node:path";
import { type RuntimePaths, wikiRoot } from "./runtime-paths";

export interface QmdSearchResult {
  path: string;
  score: number;
  snippet: string;
  body?: string;
}

export interface QmdRunnerResult {
  stdout: string;
  stderr: string;
  exitCode: number;
}

export type QmdRunner = (args: string[]) => QmdRunnerResult;

export interface SearchWorkspaceOptions {
  workspace: string;
  query: string;
  limit?: number;
}

export interface IndexWorkspaceOptions {
  workspace: string;
}

export function createProcessRunner(): QmdRunner {
  return (args) => {
    const result = spawnSync("qmd", args, { encoding: "utf8" });
    return {
      stdout: result.stdout ?? "",
      stderr: result.stderr ?? "",
      exitCode: result.status ?? 1,
    };
  };
}

function parseSearchOutput(stdout: string): QmdSearchResult[] {
  const parsed = JSON.parse(stdout) as unknown;

  if (Array.isArray(parsed)) {
    return parsed.map(normalizeSearchResult);
  }

  if (typeof parsed === "object" && parsed !== null && Array.isArray((parsed as { results?: unknown[] }).results)) {
    return ((parsed as { results: unknown[] }).results).map(normalizeSearchResult);
  }

  throw new Error("Unexpected qmd search output shape");
}

function normalizeSearchResult(input: unknown): QmdSearchResult {
  if (typeof input !== "object" || input === null) {
    throw new Error("Unexpected qmd result entry");
  }

  const entry = input as Record<string, unknown>;
  const path = typeof entry.path === "string" ? entry.path : typeof entry.file === "string" ? entry.file : null;
  const score = typeof entry.score === "number" ? entry.score : Number(entry.score ?? 0);
  const snippet = typeof entry.snippet === "string" ? entry.snippet : "";
  const body = typeof entry.body === "string" ? entry.body : undefined;

  if (!path) {
    throw new Error("Missing path in qmd search result");
  }

  return { path, score, snippet, body };
}

export function workspaceCollectionName(workspace: string): string {
  const trimmed = workspace.trim();
  if (trimmed.length === 0) {
    throw new Error("Workspace collection name cannot be empty");
  }

  // Reject path separators / traversal so a workspace name can never escape
  // workspacesDir or address an arbitrary qmd collection (isolation invariant).
  if (trimmed !== basename(trimmed) || /[\/\\]|\.\./.test(trimmed)) {
    throw new Error(`Illegal workspace name: ${workspace}`);
  }

  return trimmed;
}

export function workspaceCollectionPath(paths: RuntimePaths, workspace: string): string {
  return join(paths.workspacesDir, workspace);
}

export class QmdAdapter {
  constructor(
    private readonly paths: RuntimePaths,
    private readonly runner: QmdRunner = createProcessRunner(),
  ) {}

  searchWorkspace({ workspace, query, limit = 20 }: SearchWorkspaceOptions): QmdSearchResult[] {
    const collection = workspaceCollectionName(workspace);
    const result = this.runner([
      "search",
      query,
      "--collection",
      collection,
      "--limit",
      String(limit),
      "--json",
    ]);

    if (result.exitCode !== 0) {
      throw new Error(result.stderr.trim() || "qmd search failed");
    }

    if (result.stdout.trim().length === 0) {
      return [];
    }

    return parseSearchOutput(result.stdout);
  }

  indexWorkspace({ workspace }: IndexWorkspaceOptions): void {
    const collection = workspaceCollectionName(workspace);
    // C1 fix: index only the `wiki/` subtree. `evidence/` and `.trash/` are
    // structurally unindexable. See SPEC §7 AD-2.
    const wikiPath = wikiRoot(this.paths, workspace);
    const result = this.runner([
      "index",
      wikiPath,
      "--collection",
      collection,
    ]);

    if (result.exitCode !== 0) {
      throw new Error(result.stderr.trim() || "qmd index failed");
    }
  }
}
