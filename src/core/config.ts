import { existsSync, readFileSync } from "node:fs";
import { dirname } from "node:path";
import YAML from "js-yaml";
import {
  globalConfigSchema,
  projectBindingSchema,
  projectPointerSchema,
  projectRegistrySchema,
  type GlobalConfig,
  type ProjectBinding,
  type ProjectPointer,
  type ProjectRegistry,
} from "../schemas/config";
import { ensureDir, writeTextFile } from "./fs";

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

export function readProjectRegistry(registryFile: string): ProjectRegistry {
  if (!existsSync(registryFile)) {
    return { projects: [] };
  }

  return parseProjectRegistry(readFileSync(registryFile, "utf8"));
}

export function writeGlobalConfig(configFile: string, config: GlobalConfig): void {
  ensureDir(dirname(configFile));
  writeTextFile(configFile, YAML.dump(config, { noRefs: true }), { overwrite: true });
}

export function writeProjectPointer(pointerFile: string, pointer: ProjectPointer): void {
  ensureDir(dirname(pointerFile));
  writeTextFile(pointerFile, `${JSON.stringify(pointer, null, 2)}\n`, { overwrite: true });
}

export function writeProjectRegistry(registryFile: string, registry: ProjectRegistry): void {
  ensureDir(dirname(registryFile));
  writeTextFile(registryFile, `${JSON.stringify(projectRegistrySchema.parse(registry), null, 2)}\n`, { overwrite: true });
}

export function upsertProjectBinding(registryFile: string, binding: ProjectBinding): ProjectRegistry {
  const nextBinding = projectBindingSchema.parse(binding);
  const registry = readProjectRegistry(registryFile);
  const projects = registry.projects.filter((entry) => entry.project_root !== nextBinding.project_root);
  projects.push(nextBinding);
  projects.sort((a, b) => a.project_root.localeCompare(b.project_root));

  const nextRegistry = { ...registry, projects };
  writeProjectRegistry(registryFile, nextRegistry);
  return nextRegistry;
}

export function readProjectBinding(registryFile: string, projectRoot: string): ProjectBinding | null {
  const registry = readProjectRegistry(registryFile);
  return registry.projects.find((entry) => entry.project_root === projectRoot) ?? null;
}
