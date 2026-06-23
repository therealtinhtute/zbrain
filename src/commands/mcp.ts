// `zbrain mcp serve` CLI — start the MCP server on stdio.

import { Command } from "commander";
import { assertRuntimeReady, createCommandContext } from "./helpers";
import { McpServer } from "../mcp/server";

export async function runMcpServe(): Promise<void> {
  const context = createCommandContext();
  assertRuntimeReady(context.paths);
  const server = new McpServer({ paths: context.paths });
  // Read line-by-line from stdin, write responses to stdout.
  const decoder = new TextDecoder();
  let buffer = "";
  const readable = (async function* () {
    for await (const chunk of Bun.stdin.stream() as AsyncIterable<Uint8Array>) {
      buffer += decoder.decode(chunk, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";
      for (const line of lines) yield line;
    }
    if (buffer.length > 0) yield buffer;
  })();
  await server.serve(readable, {
    write: (s) => {
      process.stdout.write(s);
    },
  });
}

export function registerMcpCommand(program: Command): void {
  const mcp = program.command("mcp").description("MCP server (Model Context Protocol)");
  mcp.command("serve").description("Start MCP server on stdio").action(runMcpServe);
}
