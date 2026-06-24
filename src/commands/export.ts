// `zbrain export` and `zbrain import` (AC-P2-3).
// Produces a portable tarball of `~/.zbrain` minus volatile files (WAL, SHM,
// logs). Round-trip safe: `import` re-extracts and reindexes.

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync, mkdirSync, rmSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, basename } from "node:path";
import { Command } from "commander";
import { createCommandContext, assertRuntimeReady } from "./helpers";
import { clackUi, type CommandUi } from "./ui";
import { rebuildWorkspace } from "../core/indexer";
import { initDb } from "../core/db";
import { listWorkspaceNames } from "./helpers";
import { createHash } from "node:crypto";
import type { RuntimePathOptions } from "../core/runtime-paths";

export interface ExportOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  output?: string;
}

export async function runExport(options: ExportOptions = {}): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);

  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const output = options.output ?? join(process.cwd(), `zbrain-export-${stamp}.tar.gz`);

  // Stage a temp dir with manifest.sha256 of the runtime dir.
  const stage = join(tmpdir(), `zbrain-export-${Date.now()}`);
  mkdirSync(stage, { recursive: true });
  try {
    const manifest = buildManifest(context.paths.runtimeDir);
    writeFileSync(join(stage, "manifest.sha256"), manifest, "utf8");
    // tar -czf output -C <runtimeDir> .
    const result = spawnSync("tar", ["czf", output, "-C", context.paths.runtimeDir, "."], { encoding: "utf8" });
    if (result.status !== 0) {
      throw new Error(`tar failed: ${result.stderr}`);
    }
    ui.note(`exported: ${output}`, "zbrain export");
  } finally {
    rmSync(stage, { recursive: true, force: true });
  }
}

function buildManifest(runtimeDir: string): string {
  const skip = new Set(["zbrain.db-wal", "zbrain.db-shm"]);
  const lines: string[] = [];
  walk(runtimeDir, runtimeDir, skip, lines);
  return lines.join("\n") + "\n";
}

function walk(root: string, dir: string, skip: Set<string>, lines: string[]): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (skip.has(entry.name)) continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(root, full, skip, lines);
      continue;
    }
    if (!entry.isFile()) continue;
    const rel = full.slice(root.length + 1);
    const hash = createHash("sha256").update(readFileSync(full)).digest("hex");
    lines.push(`${hash}  ${rel}`);
  }
}

export interface ImportOptions {
  ui?: CommandUi;
  pathOptions?: RuntimePathOptions;
  tarball: string;
}

export async function runImport(options: ImportOptions): Promise<void> {
  const ui = options.ui ?? clackUi;
  const context = createCommandContext(options.pathOptions);
  assertRuntimeReady(context.paths);
  const stage = join(tmpdir(), `zbrain-import-${Date.now()}`);
  mkdirSync(stage, { recursive: true });
  try {
    const result = spawnSync("tar", ["xzf", options.tarball, "-C", stage], { encoding: "utf8" });
    if (result.status !== 0) {
      throw new Error(`tar extract failed: ${result.stderr}`);
    }
    // Verify manifest if present.
    const manifestPath = join(stage, "manifest.sha256");
    if (existsSync(manifestPath)) {
      const expected = readFileSync(manifestPath, "utf8").split("\n").filter(Boolean);
      for (const line of expected) {
        const [hash, rel] = line.split(/\s+/);
        const full = join(stage, rel);
        if (!existsSync(full)) {
          throw new Error(`Manifest references missing file: ${rel}`);
        }
        const actual = createHash("sha256").update(readFileSync(full)).digest("hex");
        if (actual !== hash) {
          throw new Error(`Hash mismatch: ${rel} (expected ${hash}, got ${actual})`);
        }
      }
    }
    // Copy contents into the runtime dir (preserve files; do not delete what isn't there).
    const result2 = spawnSync("cp", ["-a", `${stage}/.`, context.paths.runtimeDir + "/"], { encoding: "utf8" });
    if (result2.status !== 0) {
      throw new Error(`cp failed: ${result2.stderr}`);
    }
    // Rebuild indexes.
    const db = initDb(context.paths.runtimeDir);
    for (const ws of listWorkspaceNames(context.paths)) {
      rebuildWorkspace({ paths: context.paths, workspace: ws, db });
    }
    ui.note(`imported: ${options.tarball} -> ${context.paths.runtimeDir}`, "zbrain import");
  } finally {
    rmSync(stage, { recursive: true, force: true });
  }
}

export function registerExportCommand(program: Command): void {
  const exp = program.command("export").description("Export the zbrain runtime to a tarball");
  exp.option("-o, --output <path>", "output tarball path").action((options: { output?: string }) =>
    runExport({ output: options.output }),
  );
  const imp = program.command("import").description("Import a zbrain tarball");
  imp.argument("<tarball>", "tarball to import").action((tarball: string) => runImport({ tarball }));
}
