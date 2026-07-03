// `zbrain ask` CLI — retrieve ranked workspace context for one question.

import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { readProjectBinding } from "../core/config";
import { openDb, initDb } from "../core/db";
import { retrieveMultiWorkspaceContext, retrieveWorkspaceContext, type RetrievalAdapter } from "../core/retrieval";
import { resolveActiveWorkspace } from "../core/workspace-resolver";
import { rebuildWorkspace } from "../core/indexer";
import { createRetrievalAdapter } from "../adapters/retrieval";
import { getSessionId, touchSession } from "../core/session";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface AskCommandOptions {
  workspace?: string;
  limit?: string | number;
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  adapter?: RetrievalAdapter;
  noLazyIndex?: boolean;
  sessionId?: string;
}

function isIndexStale(paths: ReturnType<typeof createCommandContext>["paths"], workspace: string): boolean {
  // Index is "stale" if any .md exists under wiki/ but the notes table has zero
  // rows for this workspace, OR if the row count is < file count.
  const db = openDb(paths.runtimeDir);
  const wiki = join(paths.workspacesDir, workspace, "wiki");
  if (!existsSync(wiki)) return false;
  let fileCount = 0;
  for (const tier of ["axioms", "mental-models", "projects", "decisions"]) {
    const tierDir = join(wiki, tier);
    if (!existsSync(tierDir)) continue;
    for (const entry of readdirSync(tierDir, { withFileTypes: true })) {
      if (entry.isFile() && entry.name.endsWith(".md")) fileCount += 1;
    }
  }
  const row = db.prepare(`SELECT COUNT(*) as c FROM notes WHERE workspace = ?`).get(workspace) as { c: number };
  db.close();
  return fileCount > 0 && row.c === 0;
}

export async function runAsk(query: string, options: AskCommandOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  let active: string;
  if (options.workspace) {
    if (!existsSync(join(context.paths.workspacesDir, options.workspace))) {
      throw new Error(`Workspace "${options.workspace}" does not exist.`);
    }
    active = options.workspace;
  } else {
    active = resolveActiveWorkspace(context.paths).name;
  }
  const db = initDb(context.paths.runtimeDir);

  // AC-P1-8: lazy/auto index. If the index is stale (files on disk but no
  // DB rows), rebuild before searching. Opt-out via --no-lazy-index.
  if (!options.noLazyIndex && isIndexStale(context.paths, active)) {
    rebuildWorkspace({ paths: context.paths, workspace: active, db });
  }

  const projectBinding = readProjectBinding(db, context.paths.cwd);
  const secondaries = options.workspace ? [] : projectBinding?.secondary_workspaces ?? [];
  const parsedLimit = typeof options.limit === "string" ? Number(options.limit) : options.limit;
  const limit = typeof parsedLimit === "number" && Number.isInteger(parsedLimit) && parsedLimit > 0 ? parsedLimit : 8;
  const adapter = options.adapter ?? createRetrievalAdapter(db, context.paths);
  // V2 multi-agent fix: thread session id through retrieval so per-session
  // context files don't clobber each other.
  const sessionId = options.sessionId ?? getSessionId();
  touchSession(db, { id: sessionId, projectRoot: context.paths.cwd, workspace: active });

  ui.intro("zbrain ask");
  const spinner = ui.spinner();
  spinner.start("Retrieving workspace context");
  const result = secondaries.length > 0
    ? retrieveMultiWorkspaceContext(
        context.paths,
        {
          primaryWorkspace: active,
          query,
          secondaries,
          workspacesDir: context.paths.workspacesDir,
          limit,
          sessionId,
        },
        adapter,
      )
    : retrieveWorkspaceContext(context.paths, { workspace: active, query, limit, sessionId }, adapter);
  spinner.stop("Context retrieved");

  ui.note(
    [
      `workspace: ${active}`,
      `session_id: ${result.sessionId}`,
      `results: ${result.results.length}`,
      `context_file: ${result.filePath}`,
    ].join("\n"),
    "Ask summary",
  );
  ui.outro("Context ready.");
}

export function registerAskCommand(program: Command): void {
  program
    .command("ask")
    .argument("<question>", "question to retrieve context for")
    .description("Retrieve ranked workspace context for a question")
    .option("--workspace <name>", "target workspace")
    .option("--limit <n>", "result limit")
    .option("--no-lazy-index", "skip auto-rebuild when index is stale")
    .action((question: string, options: AskCommandOptions) => runAsk(question, options));
}
