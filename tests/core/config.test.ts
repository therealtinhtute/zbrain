import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { parseGlobalConfig, parseProjectPointer, readGlobalConfig, readProjectPointer } from "../../src/core/config";

describe("config parsing", () => {
  test("parses empty global config as defaults", () => {
    expect(parseGlobalConfig("")).toEqual({});
  });

  test("reads missing config files as defaults", () => {
    const baseDir = mkdtempSync(join(tmpdir(), "zbrain-config-"));

    try {
      expect(readGlobalConfig(join(baseDir, "config.yml"))).toEqual({});
      expect(readProjectPointer(join(baseDir, "zbrain.json"))).toBeNull();
    } finally {
      rmSync(baseDir, { recursive: true, force: true });
    }
  });

  test("parses persisted config and project pointer files", () => {
    const baseDir = mkdtempSync(join(tmpdir(), "zbrain-config-"));
    const configFile = join(baseDir, "config.yml");
    const pointerFile = join(baseDir, "zbrain.json");

    try {
      writeFileSync(configFile, "default_workspace: programming\nruntime_version: 0.1.0\n");
      writeFileSync(pointerFile, JSON.stringify({ workspace: "finance" }));

      expect(readGlobalConfig(configFile)).toEqual({
        default_workspace: "programming",
        runtime_version: "0.1.0",
      });
      expect(readProjectPointer(pointerFile)).toEqual({ workspace: "finance" });
      expect(parseProjectPointer("{\"workspace\":\"health\"}")).toEqual({ workspace: "health" });
    } finally {
      rmSync(baseDir, { recursive: true, force: true });
    }
  });

  test("rejects invalid config values", () => {
    expect(() => parseGlobalConfig("default_workspace: 42")).toThrow();
    expect(() => parseProjectPointer("{\"workspace\": \"\"}")).toThrow();
  });
});
