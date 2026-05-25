import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { readGlobalConfig, readProjectPointer } from "./config";
import { type RuntimePaths } from "./runtime-paths";

export type WorkspaceResolutionSource = "project_pointer" | "global_config" | "single_workspace_auto";

export interface ResolvedWorkspace {
  name: string;
  path: string;
  source: WorkspaceResolutionSource;
}

export class WorkspaceResolutionError extends Error {}

function listWorkspaceNames(workspacesDir: string): string[] {
  if (!existsSync(workspacesDir)) {
    return [];
  }

  return readdirSync(workspacesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function assertWorkspaceExists(workspacesDir: string, workspaceName: string, source: WorkspaceResolutionSource): ResolvedWorkspace {
  const workspacePath = join(workspacesDir, workspaceName);

  if (!existsSync(workspacePath)) {
    throw new WorkspaceResolutionError(
      `Workspace "${workspaceName}" from ${source} does not exist under ${workspacesDir}`,
    );
  }

  return {
    name: workspaceName,
    path: workspacePath,
    source,
  };
}

export function resolveActiveWorkspace(paths: RuntimePaths): ResolvedWorkspace {
  const projectPointer = readProjectPointer(paths.projectPointerFile);

  if (projectPointer) {
    return assertWorkspaceExists(paths.workspacesDir, projectPointer.workspace, "project_pointer");
  }

  const globalConfig = readGlobalConfig(paths.configFile);

  if (globalConfig.default_workspace) {
    return assertWorkspaceExists(paths.workspacesDir, globalConfig.default_workspace, "global_config");
  }

  const workspaceNames = listWorkspaceNames(paths.workspacesDir);

  if (workspaceNames.length === 1) {
    return assertWorkspaceExists(paths.workspacesDir, workspaceNames[0], "single_workspace_auto");
  }

  throw new WorkspaceResolutionError(
    "Unable to resolve an active workspace. Run `zbrain init` or set `default_workspace` in ~/.zbrain/config.yml.",
  );
}
