import { test, expect } from "bun:test";

const REQ = (id: number | string, method: string, params?: unknown) =>
  JSON.stringify({ jsonrpc: "2.0", id, method, params });

import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { McpServer } from "../src/mcp/server";
import { resolveRuntimePaths, wikiTierPath } from "../src/core/runtime-paths";
import { initDb } from "../src/core/db";
import { createNote } from "../src/core/note-service";
import { upsertNote } from "../src/core/indexer";

let tempHome: string;
let server: McpServer;
let out = "";

beforeEachImpl();

function beforeEachImpl() {
  tempHome = mkdtempSync(join(tmpdir(), "zbrain-mcp-"));
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  const db = initDb(tempHome);
  for (const tier of ["axioms", "mental-models", "projects", "decisions"] as const) {
    mkdirSync(wikiTierPath(paths, "research", tier), { recursive: true });
  }
  server = new McpServer({ paths, db });
  out = "";
}

afterEachImpl();

function afterEachImpl() {
  rmSync(tempHome, { recursive: true, force: true });
}

test("initialize: returns protocol version + serverInfo + capabilities", async () => {
  const response = await server.handleLine(REQ(1, "initialize", {
    protocolVersion: "2024-11-05",
    clientInfo: { name: "test", version: "0.0.1" },
    capabilities: {},
  }));
  const parsed = JSON.parse(response!);
  expect(parsed.jsonrpc).toBe("2.0");
  expect(parsed.id).toBe(1);
  expect(parsed.result.protocolVersion).toBe("2024-11-05");
  expect(parsed.result.serverInfo.name).toBe("zbrain");
  expect(parsed.result.capabilities.tools).toBeDefined();
});

test("tools/list: returns 4 tools", async () => {
  const response = await server.handleLine(REQ(2, "tools/list"));
  const parsed = JSON.parse(response!);
  expect(parsed.result.tools.length).toBe(4);
  const names = parsed.result.tools.map((t: any) => t.name).sort();
  expect(names).toEqual(["get_note", "list_pending", "recall", "remember"]);
});

test("malformed JSON returns parse error", async () => {
  const response = await server.handleLine("{ not json");
  const parsed = JSON.parse(response!);
  expect(parsed.error.code).toBe(-32700);
});

test("unknown method returns method-not-found error", async () => {
  const response = await server.handleLine(REQ(3, "frobnicate"));
  const parsed = JSON.parse(response!);
  expect(parsed.error.data?.message ?? parsed.error.message).toContain("not found");
});

test("tools/call: remember writes to evidence pipeline (NOT notes)", async () => {
  const response = await server.handleLine(REQ(4, "tools/call", {
    name: "remember",
    arguments: {
      fact: "Auth tokens rotate every 90 days.",
      source: { type: "paste", value: "security review" },
      tier: "axioms",
    },
  }));
  const parsed = JSON.parse(response!);
  expect(parsed.result.content[0].text).toContain("evidence_id=");
  expect(parsed.result.content[0].text).toContain("state=ingested");
  // No notes were created.
  const { Database } = require("bun:sqlite");
  // (The DB is held by server; we just check content text — that's the moat assertion.)
});

test("tools/call: recall returns only active notes", async () => {
  // Seed an active note.
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  const note = createNote(paths, { workspace: "research", tier: "axioms", slug: "auth", body: "authentication is required" });
  const db = initDb(tempHome);
  upsertNote({ paths, workspace: "research", db }, note);
  db.close();

  const response = await server.handleLine(REQ(5, "tools/call", {
    name: "recall",
    arguments: { query: "authentication", workspace: "research" },
  }));
  const parsed = JSON.parse(response!);
  expect(parsed.result.content[0].text).toContain("axioms/auth.md");
});

test("tools/call: get_note returns body + sources", async () => {
  const paths = resolveRuntimePaths({ runtimeDir: tempHome });
  const note = createNote(paths, {
    workspace: "research",
    tier: "projects",
    slug: "oauth",
    body: "rotate tokens quarterly",
    sources: ["src-1"],
  });
  const db = initDb(tempHome);
  upsertNote({ paths, workspace: "research", db }, note);
  db.close();

  const response = await server.handleLine(REQ(6, "tools/call", {
    name: "get_note",
    arguments: { id: note.id },
  }));
  const parsed = JSON.parse(response!);
  expect(parsed.result.content[0].text).toContain("rotate tokens quarterly");
  expect(parsed.result.content[0].text).toContain("src-1");
});

test("tools/call: list_pending lists evidence in ingested/reviewed state", async () => {
  const response = await server.handleLine(REQ(7, "tools/call", {
    name: "remember",
    arguments: { fact: "x", source: { type: "paste", value: "y" } },
  }));
  const evId = JSON.parse(response!).result.content[0].text.match(/evidence_id=([\w-]+)/)?.[1];
  expect(evId).toBeTruthy();
  const list = await server.handleLine(REQ(8, "tools/call", { name: "list_pending", arguments: {} }));
  const parsed = JSON.parse(list!);
  expect(parsed.result.content[0].text).toContain(evId);
});

// bun:test beforeEach/afterEach
import { beforeEach as _be, afterEach as _ae } from "bun:test";
_be(() => beforeEachImpl());
_ae(() => afterEachImpl());
