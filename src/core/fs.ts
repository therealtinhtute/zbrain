import {
  copyFileSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, normalize, relative, resolve } from "node:path";

export interface WriteTextOptions {
  overwrite?: boolean;
}

export function ensureDir(dirPath: string): void {
  mkdirSync(dirPath, { recursive: true });
}

export function normalizeFsPath(input: string): string {
  return normalize(isAbsolute(input) ? input : resolve(input));
}

export function pathInside(parent: string, child: string): boolean {
  const relativePath = relative(normalizeFsPath(parent), normalizeFsPath(child));
  return relativePath === "" || !relativePath.startsWith("..");
}

export function writeTextFile(filePath: string, contents: string, options: WriteTextOptions = {}): void {
  if (!options.overwrite && existsSync(filePath)) {
    throw new Error(`Refusing to overwrite existing file: ${filePath}`);
  }

  ensureDir(dirname(filePath));
  writeFileSync(filePath, contents, "utf8");
}

export function createSymlinkOrCopy(target: string, destination: string): "symlink" | "copy" {
  ensureDir(dirname(destination));
  rmSync(destination, { recursive: true, force: true });

  try {
    symlinkSync(target, destination);
    return "symlink";
  } catch {
    if (lstatSync(target).isDirectory()) {
      copyDirectory(target, destination);
    } else {
      copyFileSync(target, destination);
    }
    return "copy";
  }
}

export function walkFiles(rootDir: string): string[] {
  if (!existsSync(rootDir)) {
    return [];
  }

  const results: string[] = [];

  for (const entry of readdirSync(rootDir, { withFileTypes: true })) {
    const absolutePath = resolve(rootDir, entry.name);

    if (entry.isDirectory()) {
      results.push(...walkFiles(absolutePath));
      continue;
    }

    if (entry.isFile()) {
      results.push(absolutePath);
    }
  }

  return results.sort();
}

export function readTextFile(filePath: string): string {
  return readFileSync(filePath, "utf8");
}

export function copyDirectory(sourceDir: string, destinationDir: string): void {
  ensureDir(dirname(destinationDir));
  cpSync(sourceDir, destinationDir, { recursive: true });
}
