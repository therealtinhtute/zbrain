import { existsSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { ensureDir, writeTextFile } from "./fs";
import { type RuntimePaths } from "./runtime-paths";
import { bundledAssets } from "../generated/bundled-assets";

export interface BundledAsset {
  relativePath: string;
  contents: string;
}

export interface AssetExtractionOptions {
  assetRoot?: string;
  overwriteExisting?: boolean;
}

export interface AssetExtractionResult {
  copied: string[];
  skipped: string[];
}

export function getBundledAssets(): BundledAsset[] {
  return bundledAssets.map((asset) => ({
    relativePath: asset.relativePath,
    contents: asset.contents,
  }));
}

function shouldPreserveExisting(relativePath: string): boolean {
  return relativePath.startsWith("workspaces/");
}

export function extractBundledAssets(
  paths: RuntimePaths,
  options: AssetExtractionOptions = {},
): AssetExtractionResult {
  const overwriteExisting = options.overwriteExisting ?? true;
  const copied: string[] = [];
  const skipped: string[] = [];

  for (const asset of getBundledAssets()) {
    const destination = resolve(paths.runtimeDir, asset.relativePath);
    const destinationExists = existsSync(destination);

    if (destinationExists && (!overwriteExisting || shouldPreserveExisting(asset.relativePath))) {
      skipped.push(asset.relativePath);
      continue;
    }

    ensureDir(dirname(destination));
    writeTextFile(destination, asset.contents, { overwrite: true });
    copied.push(asset.relativePath);
  }

  return { copied, skipped };
}
