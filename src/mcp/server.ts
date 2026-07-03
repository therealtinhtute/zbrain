// Minimal MCP server (JSON-RPC 2.0 over stdio).
// Per SPEC §5 AC-P3-4: recall, remember, list_pending, get_note.
// v2.1 adds `add_note` — a trusted fast path straight to the wiki, conflict-checked.
// `remember` still writes to the evidence pipeline (NOT directly to notes);
// review -> apply is still required for unverified external material (the moat).

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import type { Database } from "bun:sqlite";
import { initDb } from "../core/db";
import { Fts5Adapter } from "../adapters/retrieval/fts5-adapter";
import { createNote, listNotes, readNote, slugify } from "../core/note-service";
import { detectConflict } from "../core/conflict";
import { upsertNote } from "../core/indexer";
import { isWikiTier, type WikiTier } from "../core/workspace-layout";
import { createEvidenceRecord, insertEvidence } from "../core/evidence-service";
import { getSessionId, touchSession } from "../core/session";
import type { RuntimePaths } from "../core/runtime-paths";
import { resolveRuntimePaths } from "../core/runtime-paths";

interface JsonRpcRequest {
  jsonrpc: "2.0";
  id?: number | string;
  method: string;
  params?: unknown;
}

interface JsonRpcResponse {
  jsonrpc: "2.0";
  id: number | string | null;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

const PROTOCOL_VERSION = "2024-11-05";

export const MCP_TOOLS = [
  {
    name: "recall",
    description: "Retrieve ranked memory context for a question. Returns active notes only.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string" },
        limit: { type: "number", default: 8 },
        workspace: { type: "string" },
      },
      required: ["query"],
    },
  },
  {
    name: "remember",
    description: "Propose a fact for human review. Writes to the evidence pipeline (ingested state). Does NOT auto-apply.",
    inputSchema: {
      type: "object",
      properties: {
        fact: { type: "string" },
        source: {
          type: "object",
          properties: {
            type: { type: "string", enum: ["paste", "file", "url"] },
            value: { type: "string" },
          },
          required: ["type", "value"],
        },
        tier: { type: "string", enum: ["axioms", "mental-models", "projects", "decisions"] },
        labels: { type: "array", items: { type: "string" } },
      },
      required: ["fact", "source"],
    },
  },
  {
    name: "add_note",
    description: "Write a note directly to the wiki (trusted fast path, conflict-checked). Use `remember` instead for unverified external material, which still requires human review.",
    inputSchema: {
      type: "object",
      properties: {
        title: { type: "string" },
        body: { type: "string" },
        tier: { type: "string", enum: ["axioms", "mental-models", "projects", "decisions"] },
        workspace: { type: "string" },
        slug: { type: "string" },
        sources: { type: "array", items: { type: "string" } },
      },
      required: ["title", "body", "tier"],
    },
  },
  {
    name: "list_pending",
    description: "List evidence items awaiting human review or apply.",
    inputSchema: {
      type: "object",
      properties: {
        workspace: { type: "string" },
      },
    },
  },
  {
    name: "get_note",
    description: "Get a note by id, including sources + supersedes chain.",
    inputSchema: {
      type: "object",
      properties: {
        id: { type: "string" },
      },
      required: ["id"],
    },
  },
] as const;

export interface McpServerOptions {
  paths?: RuntimePaths;
  db?: Database;
}

export class McpServer {
  private paths: RuntimePaths;
  private db: Database;
  private buffer = "";
  // One session id per running MCP server process — a Claude Code session
  // talks to one server instance for its whole lifetime.
  private sessionId = getSessionId();

  constructor(options: McpServerOptions = {}) {
    this.paths = options.paths ?? resolveRuntimePaths();
    this.db = options.db ?? initDb(this.paths.runtimeDir);
  }

  async handleLine(line: string): Promise<string | null> {
    let request: JsonRpcRequest;
    try {
      request = JSON.parse(line) as JsonRpcRequest;
    } catch {
      return this.respond(null, undefined, { code: -32700, message: "Parse error" });
    }
    if (request.jsonrpc !== "2.0" || typeof request.method !== "string") {
      return this.respond(request.id ?? null, undefined, { code: -32600, message: "Invalid Request" });
    }
    try {
      const result = await this.dispatch(request.method, request.params);
      return this.respond(request.id ?? null, result, undefined);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      return this.respond(request.id ?? null, undefined, { code: -32603, message: "Internal error", data: { message } });
    }
  }

  private async dispatch(method: string, params: unknown): Promise<unknown> {
    switch (method) {
      case "initialize":
        return {
          protocolVersion: PROTOCOL_VERSION,
          serverInfo: { name: "zbrain", version: "2.0.0" },
          capabilities: { tools: {} },
        };
      case "notifications/initialized":
        return {};
      case "tools/list":
        return { tools: MCP_TOOLS };
      case "tools/call":
        return this.callTool((params ?? {}) as { name: string; arguments?: Record<string, unknown> });
      case "ping":
        return {};
      default:
        throw new Error(`Method not found: ${method}`);
    }
  }

  private callTool(params: { name: string; arguments?: Record<string, unknown> }): unknown {
    const args = params.arguments ?? {};
    switch (params.name) {
      case "recall":
        return this.toolRecall(args);
      case "remember":
        return this.toolRemember(args);
      case "add_note":
        return this.toolAddNote(args);
      case "list_pending":
        return this.toolListPending(args);
      case "get_note":
        return this.toolGetNote(args);
      default:
        throw new Error(`Unknown tool: ${params.name}`);
    }
  }

  private toolRecall(args: Record<string, unknown>): { content: Array<{ type: "text"; text: string }> } {
    const query = String(args.query ?? "");
    const limit = typeof args.limit === "number" ? args.limit : 8;
    const workspace = typeof args.workspace === "string" ? args.workspace : this.firstWorkspace();
    if (!workspace) {
      return { content: [{ type: "text", text: "No workspace available." }] };
    }
    touchSession(this.db, { id: this.sessionId, projectRoot: this.paths.cwd, workspace });
    const adapter = new Fts5Adapter(this.db, this.paths);
    const result = adapter.search({ workspace, query, limit });
    const lines = result.hits.map((h) => `- [${h.tier}] ${h.path}\n  ${h.body.slice(0, 200)}`);
    return { content: [{ type: "text", text: lines.length > 0 ? lines.join("\n") : "No results." }] };
  }

  private toolRemember(args: Record<string, unknown>): { content: Array<{ type: "text"; text: string }> } {
    const fact = String(args.fact ?? "");
    const source = args.source as { type: string; value: string } | undefined;
    if (!source || typeof source.type !== "string" || typeof source.value !== "string") {
      throw new Error("source.type and source.value are required");
    }
    const workspace = this.firstWorkspace();
    if (!workspace) throw new Error("No workspace available");
    const record = createEvidenceRecord({
      workspace,
      sourceType: source.type,
      origin: source.value,
      label: source.value.slice(0, 60),
      rawContent: fact,
    });
    insertEvidence(this.db, record, new Date().toISOString());
    return {
      content: [{
        type: "text",
        text: `Remembered: evidence_id=${record.id} (state=ingested). Pending human review. Use \`zbrain ingest review ${record.id}\` next.`,
      }],
    };
  }

  private toolAddNote(args: Record<string, unknown>): { content: Array<{ type: "text"; text: string }> } {
    const title = String(args.title ?? "");
    const body = String(args.body ?? "");
    const tierArg = String(args.tier ?? "");
    if (!title || !body) throw new Error("title and body are required");
    if (!isWikiTier(tierArg)) {
      throw new Error(`Invalid tier "${tierArg}". Must be one of: axioms, mental-models, projects, decisions.`);
    }
    const tier: WikiTier = tierArg;
    const workspace = typeof args.workspace === "string" ? args.workspace : this.firstWorkspace();
    if (!workspace) throw new Error("No workspace available");
    const slug = typeof args.slug === "string" && args.slug ? args.slug : slugify(title);
    const sources = Array.isArray(args.sources) ? args.sources.filter((s): s is string => typeof s === "string") : [];

    const conflict = detectConflict(this.paths, workspace, tier, slug, []);
    if (conflict) {
      throw new Error(
        `Conflict: ${conflict.proposedPath} already exists (id: ${conflict.existing.id}). ` +
          `Call get_note with that id to inspect it, then supersede via \`zbrain note update ${conflict.existing.id}\` or retry with a different slug.`,
      );
    }

    const note = createNote(this.paths, { workspace, tier, slug, body, title, sources });
    upsertNote({ paths: this.paths, workspace, db: this.db }, note);
    return {
      content: [{
        type: "text",
        text: `Added: id=${note.id} path=${note.relPath} tier=${note.tier} workspace=${workspace}`,
      }],
    };
  }

  private toolListPending(args: Record<string, unknown>): { content: Array<{ type: "text"; text: string }> } {
    const workspace = typeof args.workspace === "string" ? args.workspace : this.firstWorkspace();
    if (!workspace) return { content: [{ type: "text", text: "No workspace." }] };
    const rows = this.db.prepare(`
      SELECT id, label, state, ingested_at FROM evidence_sources
      WHERE workspace = ? AND state IN ('ingested', 'reviewed')
      ORDER BY ingested_at ASC
    `).all(workspace) as Array<{ id: string; label: string; state: string; ingested_at: string }>;
    const lines = rows.map((r) => `- ${r.id} [${r.state}] ${r.label} (${r.ingested_at})`);
    return { content: [{ type: "text", text: lines.length > 0 ? lines.join("\n") : "No pending evidence." }] };
  }

  private toolGetNote(args: Record<string, unknown>): { content: Array<{ type: "text"; text: string }> } {
    const id = String(args.id ?? "");
    if (!id) throw new Error("id is required");
    // Find note by id across workspaces/tiers.
    for (const ws of this.allWorkspaces()) {
      for (const note of listNotes(this.paths, ws)) {
        if (note.id === id) {
          const text = [
            `id: ${note.id}`,
            `status: ${note.status}`,
            `tier: ${note.tier}`,
            `path: ${note.relPath}`,
            `sources: ${JSON.stringify(note.sources)}`,
            `supersedes: ${JSON.stringify(note.supersedes)}`,
            `superseded_by: ${note.supersededBy ?? "null"}`,
            "",
            "---",
            "",
            note.body,
          ].join("\n");
          return { content: [{ type: "text", text }] };
        }
      }
    }
    return { content: [{ type: "text", text: `Note not found: ${id}` }] };
  }

  private firstWorkspace(): string | null {
    if (!existsSync(this.paths.workspacesDir)) return null;
    for (const e of readdirSync(this.paths.workspacesDir, { withFileTypes: true })) {
      if (e.isDirectory()) return e.name;
    }
    return null;
  }

  private allWorkspaces(): string[] {
    if (!existsSync(this.paths.workspacesDir)) return [];
    return readdirSync(this.paths.workspacesDir, { withFileTypes: true })
      .filter((e) => e.isDirectory())
      .map((e) => e.name);
  }

  private respond(
    id: number | string | null,
    result: unknown,
    error: { code: number; message: string; data?: unknown } | undefined,
  ): string {
    const response: JsonRpcResponse = { jsonrpc: "2.0", id };
    if (error) response.error = error;
    else response.result = result;
    return JSON.stringify(response);
  }

  // Read from a stream-like source line by line and write responses.
  async serve(input: AsyncIterable<string>, output: { write(s: string): void }): Promise<void> {
    for await (const chunk of input) {
      this.buffer += chunk;
      const lines = this.buffer.split("\n");
      this.buffer = lines.pop() ?? "";
      for (const line of lines) {
        if (line.trim().length === 0) continue;
        const response = await this.handleLine(line);
        if (response) output.write(response + "\n");
      }
    }
  }
}
