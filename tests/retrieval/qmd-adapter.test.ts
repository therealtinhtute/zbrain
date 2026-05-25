import { describe, expect, test } from "bun:test";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { QmdAdapter, workspaceCollectionName } from "../../src/core/qmd-adapter";
import { resolveRuntimePaths } from "../../src/core/runtime-paths";

describe("QmdAdapter", () => {
  test("rejects empty workspace collections", () => {
    expect(() => workspaceCollectionName("")).toThrow();
  });

  test("passes collection-scoped search arguments", () => {
    const calls: string[][] = [];
    const adapter = new QmdAdapter(
      resolveRuntimePaths({ cwd: join(tmpdir(), "zbrain"), runtimeDir: join(tmpdir(), "zbrain-runtime") }),
      (args) => {
        calls.push(args);
        return {
          exitCode: 0,
          stdout: JSON.stringify([{ path: "/tmp/workspaces/programming/axioms/fact.md", score: 12.3, snippet: "fact" }]),
          stderr: "",
        };
      },
    );

    const results = adapter.searchWorkspace({
      workspace: "programming",
      query: "solid design",
      limit: 5,
    });

    expect(calls[0]).toEqual([
      "search",
      "solid design",
      "--collection",
      "programming",
      "--limit",
      "5",
      "--json",
    ]);
    expect(results).toHaveLength(1);
  });

  test("returns empty results for empty stdout", () => {
    const adapter = new QmdAdapter(
      resolveRuntimePaths({ cwd: join(tmpdir(), "zbrain"), runtimeDir: join(tmpdir(), "zbrain-runtime") }),
      () => ({ exitCode: 0, stdout: "", stderr: "" }),
    );

    expect(
      adapter.searchWorkspace({ workspace: "programming", query: "nothing" }),
    ).toEqual([]);
  });
});
