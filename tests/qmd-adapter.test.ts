import { test, expect } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { QmdAdapter, type QmdRunner } from "../src/core/qmd-adapter";
import { resolveRuntimePaths, wikiRoot } from "../src/core/runtime-paths";

function makeTempPaths() {
  const tempHome = mkdtempSync(join(tmpdir(), "zbrain-qmd-"));
  return { paths: resolveRuntimePaths({ runtimeDir: tempHome }), tempHome };
}

test("indexWorkspace runs `qmd update` (not `collection add`) when the collection already exists at the expected path", () => {
  const { paths, tempHome } = makeTempPaths();
  const wikiPath = wikiRoot(paths, "research");
  const calls: string[][] = [];
  const runner: QmdRunner = (args) => {
    calls.push(args);
    if (args[0] === "collection" && args[1] === "show") {
      return { stdout: `Collection: research\n  Path:     ${wikiPath}\n  Pattern:  **/*.md\n`, stderr: "", exitCode: 0 };
    }
    return { stdout: "", stderr: "", exitCode: 0 };
  };

  new QmdAdapter(paths, runner).indexWorkspace({ workspace: "research" });

  expect(calls.some((a) => a[0] === "update")).toBe(true);
  expect(calls.some((a) => a[0] === "collection" && a[1] === "add")).toBe(false);
  rmSync(tempHome, { recursive: true, force: true });
});

test("indexWorkspace refuses to reuse a same-named collection that points at a different path (e.g. the workspace root, leaking evidence/)", () => {
  const { paths, tempHome } = makeTempPaths();
  const workspaceRootPath = join(paths.workspacesDir, "research");
  const runner: QmdRunner = (args) => {
    if (args[0] === "collection" && args[1] === "show") {
      return { stdout: `Collection: research\n  Path:     ${workspaceRootPath}\n  Pattern:  **/*.md\n`, stderr: "", exitCode: 0 };
    }
    return { stdout: "", stderr: "", exitCode: 0 };
  };

  expect(() => new QmdAdapter(paths, runner).indexWorkspace({ workspace: "research" })).toThrow(
    /already exists but points at/,
  );
  rmSync(tempHome, { recursive: true, force: true });
});

test("indexWorkspace passes --name so the collection is named after the workspace, not the wiki/ folder basename", () => {
  const { paths, tempHome } = makeTempPaths();
  const calls: string[][] = [];
  const runner: QmdRunner = (args) => {
    calls.push(args);
    if (args[0] === "collection" && args[1] === "show") return { stdout: "", stderr: "", exitCode: 1 };
    return { stdout: "", stderr: "", exitCode: 0 };
  };

  new QmdAdapter(paths, runner).indexWorkspace({ workspace: "research" });

  const addCall = calls.find((a) => a[0] === "collection" && a[1] === "add");
  expect(addCall).toBeDefined();
  expect(addCall).toContain("--name");
  expect(addCall?.[addCall.indexOf("--name") + 1]).toBe("research");
  rmSync(tempHome, { recursive: true, force: true });
});

test("searchWorkspace uses -n / --format json / --full-path (qmd 2.5.3 syntax, not the legacy --limit / --json flags)", () => {
  const { paths, tempHome } = makeTempPaths();
  const wikiPath = wikiRoot(paths, "research");
  const calls: string[][] = [];
  const runner: QmdRunner = (args) => {
    calls.push(args);
    return { stdout: "[]", stderr: "", exitCode: 0 };
  };

  new QmdAdapter(paths, runner).searchWorkspace({ workspace: "research", query: "foo", limit: 5 });

  const call = calls[0];
  expect(call).toContain("-n");
  expect(call?.[call.indexOf("-n") + 1]).toBe("5");
  expect(call).toContain("--format");
  expect(call?.[call.indexOf("--format") + 1]).toBe("json");
  expect(call).toContain("--full-path");
  expect(call).not.toContain("--limit");
  rmSync(tempHome, { recursive: true, force: true });
});

test("searchWorkspace normalizes --full-path absolute paths to wiki/-relative paths", () => {
  const { paths, tempHome } = makeTempPaths();
  const wikiPath = wikiRoot(paths, "research");
  const runner: QmdRunner = (args) => {
    if (args[0] === "search") {
      return {
        stdout: JSON.stringify([
          { file: join(wikiPath, "mental-models", "foo.md"), score: 0.5, snippet: "..." },
        ]),
        stderr: "",
        exitCode: 0,
      };
    }
    return { stdout: "", stderr: "", exitCode: 0 };
  };

  const results = new QmdAdapter(paths, runner).searchWorkspace({ workspace: "research", query: "foo" });

  expect(results).toHaveLength(1);
  expect(results[0]?.path).toBe(join("mental-models", "foo.md"));
  rmSync(tempHome, { recursive: true, force: true });
});
