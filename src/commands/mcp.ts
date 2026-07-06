// `zbrain mcp serve` CLI — start the MCP server on stdio.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { MCP_TOOLS, McpServer } from "../mcp/server";

export async function runMcpServe(): Promise<void> {
  const context = createCommandContext();
  assertRuntimeReady(context.paths);
  const server = new McpServer({ paths: context.paths });
  // Stream raw stdin chunks; `server.serve` buffers and splits lines itself.
  const decoder = new TextDecoder();
  const readable = (async function* () {
    for await (const chunk of Bun.stdin.stream() as AsyncIterable<Uint8Array>) {
      yield decoder.decode(chunk, { stream: true });
    }
  })();
  await server.serve(readable, {
    write: (s) => {
      process.stdout.write(s);
    },
  });
}

export async function runMcpToolsList(): Promise<void> {
  console.log(JSON.stringify({ tools: MCP_TOOLS }, null, 2));
}

export function registerMcpCommand(program: Command): void {
  const mcp = program.command("mcp").description("MCP server (Model Context Protocol)");
  mcp.command("serve").description("Start MCP server on stdio").action(runMcpServe);
  mcp.command("tools").description("List available MCP tools (debug)").action(runMcpToolsList);
}
