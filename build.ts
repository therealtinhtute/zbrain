import { mkdirSync } from "node:fs";

mkdirSync("dist", { recursive: true });

const generateAssets = Bun.spawnSync(["node", "scripts/generate-bundled-assets.mjs"]);

if (generateAssets.exitCode !== 0) {
  throw new Error(
    new TextDecoder().decode(generateAssets.stderr) || "asset generation failed",
  );
}

const result = Bun.spawnSync([
  "bun",
  "build",
  "--compile",
  "--outfile",
  "dist/zbrain",
  "src/index.ts",
]);

if (result.exitCode !== 0) {
  throw new Error(
    new TextDecoder().decode(result.stderr) || "bun build --compile failed",
  );
}

const stdout = new TextDecoder().decode(result.stdout).trim();

if (stdout) {
  console.log(stdout);
}
