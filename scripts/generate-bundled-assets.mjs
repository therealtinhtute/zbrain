import { mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const repoRoot = resolve(import.meta.dirname, "..");
const assetsRoot = join(repoRoot, "assets");
const outputDir = join(repoRoot, "src", "generated");
const outputFile = join(outputDir, "bundled-assets.ts");

function walk(dir) {
  const entries = readdirSync(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const absolute = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...walk(absolute));
      continue;
    }
    if (entry.isFile()) {
      files.push(absolute);
    }
  }
  return files.sort();
}

const assets = walk(assetsRoot).map((absolutePath) => {
  const relativePath = relative(assetsRoot, absolutePath).replaceAll("\\", "/");
  const contents = readFileSync(absolutePath, "utf8");
  return {
    relativePath,
    contents,
  };
});

const fileContents = `export interface BundledAssetRecord {
  relativePath: string;
  contents: string;
}

export const bundledAssets: BundledAssetRecord[] = ${JSON.stringify(assets, null, 2)};\n`;

mkdirSync(outputDir, { recursive: true });
writeFileSync(outputFile, fileContents, "utf8");
