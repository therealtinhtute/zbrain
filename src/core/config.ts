import { existsSync, readFileSync } from "node:fs";
import { dirname } from "node:path";
import YAML from "js-yaml";
import { globalConfigSchema, projectPointerSchema, type GlobalConfig, type ProjectPointer } from "../schemas/config";
import { ensureDir, writeTextFile } from "./fs";

export function parseGlobalConfig(contents: string): GlobalConfig {
  const parsed = contents.trim().length === 0 ? {} : YAML.load(contents);
  return globalConfigSchema.parse(parsed ?? {});
}

export function parseProjectPointer(contents: string): ProjectPointer {
  const parsed = JSON.parse(contents) as unknown;
  return projectPointerSchema.parse(parsed);
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
