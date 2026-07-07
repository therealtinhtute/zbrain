import { spawnSync } from "node:child_process";
import { basename, relative, resolve } from "node:path";
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

// Parses the "Path:" line out of `qmd collection show <name>` output.
function parseCollectionShowPath(stdout: string): string | null {
  const match = stdout.match(/^\s*Path:\s*(.+)$/m);
  return match ? match[1].trim() : null;
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
      "-n",
      String(limit),
      "--format",
      "json",
      "--full-path",
    ]);

    if (result.exitCode !== 0) {
      throw new Error(result.stderr.trim() || "qmd search failed");
    }

    if (result.stdout.trim().length === 0) {
      return [];
    }

    const wikiPath = wikiRoot(this.paths, workspace);
    return parseSearchOutput(result.stdout).map((hit) => ({
      ...hit,
      // `--full-path` returns absolute on-disk paths; downstream tier
      // classification and citations expect paths relative to `wiki/`,
      // matching the FTS5 adapter's shape.
      path: relative(wikiPath, hit.path),
    }));
  }

  indexWorkspace({ workspace }: IndexWorkspaceOptions): void {
    const collection = workspaceCollectionName(workspace);
    // C1 fix: index only the `wiki/` subtree. `evidence/` and `.trash/` are
    // structurally unindexable. See SPEC §7 AD-2.
    const wikiPath = wikiRoot(this.paths, workspace);

    // qmd collection names default to the indexed folder's basename, which
    // would collide across workspaces (every workspace's wiki/ folder is
    // literally named "wiki") — pass --name explicitly. qmd has no scoped
    // per-collection reindex, so an already-registered collection is
    // refreshed via a full `qmd update` instead of re-adding.
    const show = this.runner(["collection", "show", collection]);
    if (show.exitCode === 0) {
      // A collection with this name already exists in qmd's global index —
      // it may predate the workspace (stale state, or created by another
      // tool) and point somewhere other than wiki/. Reusing it blindly
      // would search/index evidence/ or the workspace root, breaking the
      // isolation invariant (AC-P0-1). Refuse instead of silently leaking.
      const registeredPath = parseCollectionShowPath(show.stdout);
      if (registeredPath !== null && resolve(registeredPath) !== resolve(wikiPath)) {
        throw new Error(
          `qmd collection "${collection}" already exists but points at "${registeredPath}", ` +
            `not the expected wiki root "${wikiPath}". Remove it (\`qmd collection remove ${collection}\`) ` +
            `or rename it before indexing this workspace.`,
        );
      }
      const result = this.runner(["update"]);
      if (result.exitCode !== 0) {
        throw new Error(result.stderr.trim() || "qmd index failed");
      }
      return;
    }

    const result = this.runner(["collection", "add", wikiPath, "--name", collection]);
    if (result.exitCode !== 0) {
      throw new Error(result.stderr.trim() || "qmd index failed");
    }
  }
}
