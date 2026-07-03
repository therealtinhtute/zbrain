import { existsSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import type { Database } from "bun:sqlite";
import { extractBundledAssets, type AssetExtractionResult } from "../core/assets";
import { readGlobalConfig, upsertProjectBinding, writeGlobalConfig } from "../core/config";
import { currentTaskFilePath } from "../core/current-task";
import { createSymlinkOrCopy, ensureDir, pathInside, readTextFile, writeTextFile } from "../core/fs";
import { resolveRuntimePaths, type RuntimePathOptions, type RuntimePaths } from "../core/runtime-paths";
import {
  WikiTiers,
  WORKSPACE_LAYOUT_VERSION,
  setWorkspaceLayoutVersion,
} from "../core/workspace-layout";
import { migrateAllWorkspaces } from "../core/workspace-migration";
import { type RuntimeName } from "../schemas/config";

export interface CommandContext {
  paths: RuntimePaths;
}

export function createCommandContext(options: RuntimePathOptions = {}): CommandContext {
  return {
    paths: resolveRuntimePaths(options),
  };
}

export function ensureRuntimeConfig(paths: RuntimePaths): { created: boolean } {
  if (existsSync(paths.configFile)) {
    return { created: false };
  }

  writeGlobalConfig(paths.configFile, {});
  return { created: true };
}

export function checkQmdInstalled(): { installed: boolean; version: string | null } {
  const result = spawnSync("qmd", ["--version"], { encoding: "utf8" });

  if (result.status !== 0) {
    return { installed: false, version: null };
  }

  return {
    installed: true,
    version: result.stdout.trim() || result.stderr.trim() || "unknown",
  };
}

export function setupRuntime(paths: RuntimePaths): AssetExtractionResult & { configCreated: boolean } {
  const extraction = extractBundledAssets(paths, { overwriteExisting: true });
  const config = ensureRuntimeConfig(paths);
  return { ...extraction, configCreated: config.created };
}

export function updateRuntime(paths: RuntimePaths): AssetExtractionResult {
  return extractBundledAssets(paths, { overwriteExisting: true });
}

export function listWorkspaceNames(paths: RuntimePaths): string[] {
  if (!existsSync(paths.workspacesDir)) {
    return [];
  }

  return readdirSync(paths.workspacesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function renderTemplate(contents: string, variables: Record<string, string>): string {
  return Object.entries(variables).reduce((acc, [key, value]) => {
    return acc.replaceAll(`{{${key}}}`, value);
  }, contents);
}

export function createWorkspaceScaffold(
  paths: RuntimePaths,
  workspaceName: string,
  nowIso: string,
): { createdPaths: string[]; defaultWorkspaceSet: boolean } {
  const workspaceRoot = join(paths.workspacesDir, workspaceName);
  const createdPaths: string[] = [];
  // V2 layout: tier content lives under `wiki/`; `evidence/` stays at root.
  // See SPEC §7 AD-2. Only `wiki/` is indexed for retrieval.
  const requiredDirs = [
    workspaceRoot,
    ...WikiTiers.map((tier) => join(workspaceRoot, "wiki", tier)),
    join(workspaceRoot, "agents"),
    join(workspaceRoot, "evidence"),
    join(workspaceRoot, "evidence", "sources"),
    join(workspaceRoot, "evidence", "analysis"),
    join(workspaceRoot, "evidence", "qa"),
    join(workspaceRoot, "evidence", "applied"),
    join(workspaceRoot, "evidence", "archive"),
  ];

  for (const dir of requiredDirs) {
    ensureDir(dir);
    createdPaths.push(dir);
  }

  const templateRoot = join(paths.runtimeDir, "templates");
  const workspaceTemplate = readFileSync(join(templateRoot, "workspace.md"), "utf8");
  const evidenceIndexTemplate = readFileSync(join(templateRoot, "evidence-index.md"), "utf8");
  const renderedWorkspace = renderTemplate(workspaceTemplate, {
    workspace_name: workspaceName,
    created_at: nowIso,
  });
  const renderedIndex = renderTemplate(evidenceIndexTemplate, {
    evidence_id: "seed",
    updated_at: nowIso,
  });

  writeTextFile(join(workspaceRoot, "workspace.md"), renderedWorkspace, { overwrite: true });
  writeTextFile(join(workspaceRoot, "evidence", "_index.md"), renderedIndex, { overwrite: true });
  setWorkspaceLayoutVersion(workspaceRoot, WORKSPACE_LAYOUT_VERSION);

  const config = readGlobalConfig(paths.configFile);
  let defaultWorkspaceSet = false;
  if (!config.default_workspace) {
    config.default_workspace = workspaceName;
    writeGlobalConfig(paths.configFile, config);
    defaultWorkspaceSet = true;
  }

  return { createdPaths, defaultWorkspaceSet };
}

export function validateWorkspaceName(input: string): string {
  const value = input.trim();

  if (!/^[a-z0-9-]+$/.test(value)) {
    throw new Error("Workspace name must use lowercase letters, numbers, or hyphens only.");
  }

  return value;
}

export interface InitSelection {
  workspace: string;
  injectTargets: string[];
}

export function appendIntegrationRules(
  rulesFile: string,
  rulesContent: string,
  title: string,
  legacyTitles: string[] = [],
): { updated: boolean } {
  const section = rulesContent.trim();
  const existing = existsSync(rulesFile) ? readTextFile(rulesFile) : "";
  const legacySection = section.replaceAll("zbrain", "zwiki");
  let withoutLegacy = existing;
  while (withoutLegacy.includes(legacySection)) {
    withoutLegacy = withoutLegacy.replace(legacySection, "").trimEnd();
  }

  for (const legacyTitle of [...legacyTitles, "zwiki Integration"]) {
    withoutLegacy = withoutLegacy.replace(
      new RegExp(`(?:^|\\n)# ${legacyTitle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\n[\\s\\S]*?(?=\\n# [^\\n]+\\n|\\s*$)`, "g"),
      "",
    ).trimEnd();
  }

  withoutLegacy = withoutLegacy.replace(
    new RegExp(`(?:^|\\n)# ${title.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\n[\\s\\S]*?(?=\\n# [^\\n]+\\n|\\s*$)`, "g"),
    "",
  ).trimEnd();

  if (withoutLegacy.includes(section)) {
    if (withoutLegacy === existing.trimEnd()) {
      return { updated: false };
    }

    writeFileSync(rulesFile, `${withoutLegacy}\n`, "utf8");
    return { updated: true };
  }

  const nextContent = withoutLegacy.trim().length === 0
    ? `${section}\n`
    : `${withoutLegacy}\n\n${section}\n`;
  writeFileSync(rulesFile, nextContent, "utf8");
  return { updated: true };
}

function syncAssetGroup(sourceDir: string, destinationDir: string): string[] {
  ensureDir(destinationDir);
  const sourceFiles = readdirSync(sourceDir).sort();
  const sourceFileSet = new Set(sourceFiles);

  for (const destinationFile of readdirSync(destinationDir)) {
    if (!sourceFileSet.has(destinationFile)) {
      rmSync(join(destinationDir, destinationFile), { recursive: true, force: true });
    }
  }

  const created: string[] = [];
  for (const file of sourceFiles) {
    const source = join(sourceDir, file);
    const destination = join(destinationDir, file);
    createSymlinkOrCopy(source, destination);
    created.push(destination);
  }

  return created;
}

export function initProject(
  db: Database,
  paths: RuntimePaths,
  selection: InitSelection,
): { created: string[]; updated: string[] } {
  const created: string[] = [];
  const updated: string[] = [];
  const runtimes = selectedRuntimes(selection.injectTargets);
  upsertProjectBinding(db, {
    project_root: paths.cwd,
    workspace: selection.workspace,
    context_file: currentTaskFilePath(paths),
    runtimes,
  });

  const claudeDir = join(paths.cwd, ".claude");
  const legacyPointerFile = join(claudeDir, "zwiki.json");
  if (existsSync(legacyPointerFile)) {
    rmSync(legacyPointerFile, { force: true });
    updated.push(legacyPointerFile);
  }

  const staleProjectPointerFile = paths.legacyProjectPointerFile;
  if (existsSync(staleProjectPointerFile)) {
    rmSync(staleProjectPointerFile, { force: true });
    updated.push(staleProjectPointerFile);
  }

  if (selection.injectTargets.includes("claude_rules")) {
    ensureDir(claudeDir);
    const rulesPath = join(paths.runtimeDir, "engine", "claude-rules.md");
    const claudeFile = join(paths.cwd, "CLAUDE.md");
    const rulesContent = readTextFile(rulesPath).replaceAll("zwiki", "zbrain");
    const result = appendIntegrationRules(claudeFile, rulesContent, "zbrain Integration");
    if (result.updated) {
      updated.push(claudeFile);
    }
  }

  if (selection.injectTargets.includes("skills")) {
    ensureDir(claudeDir);
    const skillsDir = join(claudeDir, "skills");
    created.push(...syncAssetGroup(join(paths.runtimeDir, "skills"), skillsDir));
    const staleCommandsDir = join(claudeDir, "commands", "zbrain");
    if (existsSync(staleCommandsDir)) {
      rmSync(staleCommandsDir, { recursive: true, force: true });
      updated.push(staleCommandsDir);
    }
    const legacyCommandsDir = join(claudeDir, "commands");
    if (existsSync(legacyCommandsDir)) {
      for (const file of readdirSync(legacyCommandsDir)) {
        if (file.startsWith("zbrain:") || file.startsWith("zwiki:")) {
          const staleFile = join(legacyCommandsDir, file);
          rmSync(staleFile, { force: true });
          updated.push(staleFile);
        }
      }
    }
  }

  if (selection.injectTargets.includes("agents")) {
    ensureDir(claudeDir);
    const agentsDir = join(claudeDir, "agents");
    created.push(...syncAssetGroup(join(paths.runtimeDir, "agents"), agentsDir));
  }

  if (selection.injectTargets.includes("mcp")) {
    ensureDir(claudeDir);
    const settingsFile = join(claudeDir, "settings.local.json");
    const result = mergeSettingsFile(settingsFile);
    if (result.updated) {
      updated.push(settingsFile);
    }
  }

  if (selection.injectTargets.includes("codex_rules")) {
    const rulesPath = join(paths.runtimeDir, "engine", "codex-rules.md");
    const agentsFile = join(paths.cwd, "AGENTS.md");
    const result = appendIntegrationRules(
      agentsFile,
      readTextFile(rulesPath).replaceAll("zwiki", "zbrain"),
      "zbrain Integration",
      ["Codex zbrain Integration"],
    );
    if (result.updated) {
      updated.push(agentsFile);
    }
  }

  return { created, updated };
}

function selectedRuntimes(injectTargets: string[]): RuntimeName[] {
  const runtimes: RuntimeName[] = [];

  if (injectTargets.some((target) => ["claude_rules", "skills", "agents", "mcp"].includes(target))) {
    runtimes.push("claude");
  }

  if (injectTargets.includes("codex_rules")) {
    runtimes.push("codex");
  }

  return runtimes;
}

export function assertRuntimeReady(paths: RuntimePaths): void {
  if (!existsSync(paths.runtimeDir)) {
    throw new Error("Runtime not found. Run `zbrain setup` first.");
  }
  // Auto-migrate V1 workspaces to V2 layout on every CLI boot.
  // Idempotent. Per-workspace failures are logged and do not block commands.
  if (existsSync(paths.workspacesDir)) {
    const summary = migrateAllWorkspaces(paths.workspacesDir);
    for (const { workspace, result } of summary.results) {
      if (!result.skipped && result.movedFiles.length > 0) {
        for (const moved of result.movedFiles) {
          console.warn(`[zbrain] migrated ${workspace}: ${moved}`);
        }
      }
    }
  }
}

export function summarizeExtraction(result: AssetExtractionResult): string {
  return [
    `copied: ${result.copied.length}`,
    `skipped: ${result.skipped.length}`,
  ].join("\n");
}

export function assetGroupPaths(paths: RuntimePaths): { commands: string; agents: string } {
  return {
    commands: join(paths.runtimeDir, "commands"),
    agents: join(paths.runtimeDir, "agents"),
  };
}

export function isProjectPathSafe(paths: RuntimePaths, filePath: string): boolean {
  return pathInside(paths.cwd, filePath);
}

export function fileLabel(filePath: string): string {
  return basename(resolve(filePath));
}
export function mergeSettingsFile(settingsFile: string): { updated: boolean } {
  const base = existsSync(settingsFile)
    ? (JSON.parse(readTextFile(settingsFile)) as Record<string, unknown>)
    : {};
  const mcpServers = typeof base.mcpServers === "object" && base.mcpServers !== null
    ? (base.mcpServers as Record<string, unknown>)
    : {};
  const existing = mcpServers.qmd as Record<string, unknown> | undefined;
  const qmdServer = { command: "qmd", args: ["mcp"] };

  if (existing?.command === qmdServer.command && JSON.stringify(existing?.args) === JSON.stringify(qmdServer.args)) {
    return { updated: false };
  }

  const next = {
    ...base,
    mcpServers: {
      ...mcpServers,
      qmd: qmdServer,
    },
  };

  ensureDir(dirname(settingsFile));
  writeFileSync(settingsFile, `${JSON.stringify(next, null, 2)}\n`, "utf8");
  return { updated: true };
}
