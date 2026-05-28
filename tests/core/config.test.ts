import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { parseGlobalConfig, parseProjectPointer, readGlobalConfig, readProjectPointer } from "../../src/core/config";
import { projectPointerSchema } from "../../src/schemas/config";

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

describe("secondary_workspaces schema", () => {
  test("parses pointer with valid secondary_workspaces array", () => {
    const result = projectPointerSchema.parse({
      workspace: "ttdvkh",
      secondary_workspaces: [
        { workspace: "framework-core", keywords: ["file-storage", "boilerplate"], limit: 3 },
      ],
    });
    expect(result.workspace).toBe("ttdvkh");
    expect(result.secondary_workspaces).toHaveLength(1);
    expect(result.secondary_workspaces![0]!.workspace).toBe("framework-core");
    expect(result.secondary_workspaces![0]!.keywords).toEqual(["file-storage", "boilerplate"]);
    expect(result.secondary_workspaces![0]!.limit).toBe(3);
  });

  test("parses pointer without secondary_workspaces (backward compat)", () => {
    const result = projectPointerSchema.parse({ workspace: "ttdvkh" });
    expect(result.workspace).toBe("ttdvkh");
    expect(result.secondary_workspaces).toBeUndefined();
  });

  test("defaults limit to 3 when omitted", () => {
    const result = projectPointerSchema.parse({
      workspace: "ttdvkh",
      secondary_workspaces: [{ workspace: "research", keywords: ["paper"] }],
    });
    expect(result.secondary_workspaces![0]!.limit).toBe(3);
  });

  test("rejects secondary_workspaces entry with empty workspace name", () => {
    expect(() =>
      projectPointerSchema.parse({
        workspace: "ttdvkh",
        secondary_workspaces: [{ workspace: "", keywords: ["paper"] }],
      }),
    ).toThrow();
  });

  test("rejects secondary_workspaces entry with empty keywords array", () => {
    expect(() =>
      projectPointerSchema.parse({
        workspace: "ttdvkh",
        secondary_workspaces: [{ workspace: "research", keywords: [] }],
      }),
    ).toThrow();
  });
});
