import { describe, expect, test } from "bun:test";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readlinkSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runInit } from "../src/commands/init";
import { runSetup } from "../src/commands/setup";
import { runUpdate } from "../src/commands/update";
import { runWorkspaceCreate } from "../src/commands/workspace";
import type { CommandUi } from "../src/commands/ui";

class FakeUi implements CommandUi {
  logs: string[] = [];
  notes: string[] = [];
  confirms: boolean[];
  selects: string[];
  multiselects: string[][];

  constructor(options: { confirms?: boolean[]; selects?: string[]; multiselects?: string[][] } = {}) {
    this.confirms = options.confirms ?? [];
    this.selects = options.selects ?? [];
    this.multiselects = options.multiselects ?? [];
  }

  intro(message: string): void {
    this.logs.push(`intro:${message}`);
  }

  outro(message: string): void {
    this.logs.push(`outro:${message}`);
  }

  info(message: string): void {
    this.logs.push(`info:${message}`);
  }

  note(message: string, title?: string): void {
    this.notes.push(`${title ?? "note"}:${message}`);
  }

  spinner() {
    return {
      start: (message: string) => {
        this.logs.push(`spinner-start:${message}`);
      },
      stop: (message: string) => {
        this.logs.push(`spinner-stop:${message}`);
      },
    };
  }

  async confirm(): Promise<boolean> {
    return this.confirms.shift() ?? true;
  }

  async select(): Promise<string> {
    const value = this.selects.shift();
    if (!value) {
      throw new Error("No fake select value available");
    }
    return value;
  }

  async multiselect(): Promise<string[]> {
    return this.multiselects.shift() ?? [];
  }
}

function makeFixture() {
  const root = mkdtempSync(join(tmpdir(), "zbrain-cmd-"));
  const homeDir = join(root, "home");
  const projectDir = join(root, "project");
  mkdirSync(homeDir, { recursive: true });
  mkdirSync(projectDir, { recursive: true });
  return { root, homeDir, projectDir };
}

describe("phase 4 command integrations", () => {
  test("setup creates runtime assets and config", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi();

    try {
      await runSetup({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(existsSync(join(fixture.homeDir, ".zbrain", "config.yml"))).toBe(true);
      expect(existsSync(join(fixture.homeDir, ".zbrain", "commands", "ask.md"))).toBe(true);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("workspace create scaffolds a workspace and sets default config", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi({ confirms: [true] });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui,
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      expect(existsSync(join(fixture.homeDir, ".zbrain", "workspaces", "programming", "workspace.md"))).toBe(true);
      expect(readFileSync(join(fixture.homeDir, ".zbrain", "config.yml"), "utf8")).toContain("default_workspace: programming");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("update refreshes bundled assets without overwriting workspace content", async () => {
    const fixture = makeFixture();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      const workspaceReadme = join(fixture.homeDir, ".zbrain", "workspaces", "README.md");
      writeFileSync(workspaceReadme, "custom user workspace root\n");
      await runUpdate({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(readFileSync(workspaceReadme, "utf8")).toBe("custom user workspace root\n");
      expect(existsSync(join(fixture.homeDir, ".zbrain", "engine", "claude-rules.md"))).toBe(true);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("init writes project pointers, preserves existing CLAUDE.md, and links runtime assets", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi({
      selects: ["programming"],
      multiselects: [["claude_rules", "commands", "agents", "mcp"]],
    });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      writeFileSync(join(fixture.projectDir, "CLAUDE.md"), "# Existing\n");

      await runInit({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(readFileSync(join(fixture.projectDir, ".claude", "zbrain.json"), "utf8")).toContain("\"workspace\": \"programming\"");
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# Existing");
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# zbrain Integration");
      expect(existsSync(join(fixture.projectDir, ".claude", "commands", "ask.md"))).toBe(true);
      expect(existsSync(join(fixture.projectDir, ".claude", "agents", "wiki-planner.md"))).toBe(true);
      expect(readFileSync(join(fixture.projectDir, ".claude", "settings.local.json"), "utf8")).toContain("\"qmd\"");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("init refreshes legacy project integration and removes stale files", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi({
      selects: ["programming"],
      multiselects: [["claude_rules", "commands", "agents", "mcp"]],
    });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      const claudeDir = join(fixture.projectDir, ".claude");
      mkdirSync(join(claudeDir, "commands"), { recursive: true });
      mkdirSync(join(claudeDir, "agents"), { recursive: true });
      writeFileSync(join(claudeDir, "zwiki.json"), JSON.stringify({ workspace: "programming" }));
      writeFileSync(join(claudeDir, "commands", "zbrain:ask.md"), "legacy command\n");
      writeFileSync(join(claudeDir, "agents", "legacy-agent.md"), "legacy agent\n");
      writeFileSync(join(fixture.projectDir, "CLAUDE.md"), "# Existing\n");

      const rulesTemplate = readFileSync(join(fixture.homeDir, ".zbrain", "engine", "claude-rules.md"), "utf8");
      const legacyRules = rulesTemplate.replaceAll("zbrain", "zwiki");
      writeFileSync(join(fixture.projectDir, "CLAUDE.md"), `# Existing\n\n${legacyRules.trim()}\n`);

      await runInit({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(existsSync(join(claudeDir, "zbrain.json"))).toBe(true);
      expect(existsSync(join(claudeDir, "zwiki.json"))).toBe(false);
      expect(existsSync(join(claudeDir, "commands", "ask.md"))).toBe(true);
      expect(existsSync(join(claudeDir, "commands", "zbrain:ask.md"))).toBe(false);
      expect(existsSync(join(claudeDir, "agents", "legacy-agent.md"))).toBe(false);
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# zbrain Integration");
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).not.toContain("# zwiki Integration");

      const askLink = join(claudeDir, "commands", "ask.md");
      if (lstatSync(askLink).isSymbolicLink()) {
        expect(readlinkSync(askLink)).toContain(".zbrain/commands/ask.md");
      }
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });
});
