import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";

export interface RuntimePathOptions {
  cwd?: string;
  homeDir?: string;
  runtimeDir?: string;
}

export interface RuntimePaths {
  cwd: string;
  homeDir: string;
  runtimeDir: string;
  configFile: string;
  workspacesDir: string;
  projectsDir: string;
  projectRegistryFile: string;
  legacyProjectPointerFile: string;
}

export function resolveRuntimePaths(options: RuntimePathOptions = {}): RuntimePaths {
  const cwd = resolve(options.cwd ?? process.cwd());
  const homeDir = resolve(options.homeDir ?? homedir());
  const preferredRuntimeDir = options.runtimeDir ?? process.env.ZBRAIN_HOME ?? join(homeDir, ".zbrain");
  const legacyRuntimeDir = join(homeDir, ".zwiki");
  const runtimeDir = resolve(
    options.runtimeDir || process.env.ZBRAIN_HOME
      ? preferredRuntimeDir
      : (existsSync(join(homeDir, ".zbrain")) || !existsSync(legacyRuntimeDir)
          ? preferredRuntimeDir
          : legacyRuntimeDir),
  );

  return {
    cwd,
    homeDir,
    runtimeDir,
    configFile: join(runtimeDir, "config.yml"),
    workspacesDir: join(runtimeDir, "workspaces"),
    projectsDir: join(runtimeDir, "projects"),
    projectRegistryFile: join(runtimeDir, "projects.json"),
    legacyProjectPointerFile: join(cwd, ".claude", "zbrain.json"),
  };
}
