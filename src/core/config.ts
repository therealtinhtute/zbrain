import { existsSync, readFileSync } from "node:fs";
import { dirname } from "node:path";
import type { Database } from "bun:sqlite";
import YAML from "js-yaml";
import {
  globalConfigSchema,
  projectPointerSchema,
  projectRegistrySchema,
  type GlobalConfig,
  type ProjectBinding,
  type ProjectPointer,
  type ProjectRegistry,
} from "../schemas/config";
// parseProjectPointer / parseGlobalConfig / parseProjectRegistry kept for migration script + tests
import { ensureDir, writeTextFile } from "./fs";
import { upsertProject, readProject, readProjectRegistry as dbReadProjectRegistry } from "./db-projects";

export function parseGlobalConfig(contents: string): GlobalConfig {
  const parsed = contents.trim().length === 0 ? {} : YAML.load(contents);
  return globalConfigSchema.parse(parsed ?? {});
}

export function parseProjectPointer(contents: string): ProjectPointer {
  const parsed = JSON.parse(contents) as unknown;
  return projectPointerSchema.parse(parsed);
}

export function parseProjectRegistry(contents: string): ProjectRegistry {
  const parsed = contents.trim().length === 0 ? {} : JSON.parse(contents);
  return projectRegistrySchema.parse(parsed);
}

export function readGlobalConfig(configFile: string): GlobalConfig {
  if (!existsSync(configFile)) {
    return {};
  }

  return parseGlobalConfig(readFileSync(configFile, "utf8"));
}

export function readProjectPointer(pointerFile: string): ProjectPointer | null {
  if (!existsSync(pointerFile)) {
    return null;
  }

  return parseProjectPointer(readFileSync(pointerFile, "utf8"));
}

export function writeGlobalConfig(configFile: string, config: GlobalConfig): void {
  ensureDir(dirname(configFile));
  writeTextFile(configFile, YAML.dump(config, { noRefs: true }), { overwrite: true });
}

export function writeProjectPointer(pointerFile: string, pointer: ProjectPointer): void {
  ensureDir(dirname(pointerFile));
  writeTextFile(pointerFile, `${JSON.stringify(pointer, null, 2)}\n`, { overwrite: true });
}

export function upsertProjectBinding(db: Database, binding: ProjectBinding): ProjectRegistry {
  upsertProject(db, binding, new Date().toISOString());
  return dbReadProjectRegistry(db);
}

export function readProjectBinding(db: Database, projectRoot: string): ProjectBinding | null {
  return readProject(db, projectRoot);
}

export function readProjectRegistry(db: Database): ProjectRegistry {
  return dbReadProjectRegistry(db);
}
