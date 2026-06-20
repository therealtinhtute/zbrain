import { describe, expect, test } from "bun:test";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readlinkSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { runInit } from "../src/commands/init";
import { runSetup } from "../src/commands/setup";
import { runUpdate } from "../src/commands/update";
import { runWorkspaceCreate } from "../src/commands/workspace";
import { runInteractive } from "../src/commands/interactive";
import { runAsk } from "../src/commands/ask";
import { runIngestApply, runIngestList, runIngestReview } from "../src/commands/ingest";
import { runLearn } from "../src/commands/learn";
import type { CommandUi } from "../src/commands/ui";
import { openDb } from "../src/core/db";
import { readProject } from "../src/core/db-projects";
import { readEvidence } from "../src/core/db-evidence";

class FakeUi implements CommandUi {
  logs: string[] = [];
  notes: string[] = [];
  confirms: boolean[];
  selects: string[];
  multiselects: string[][];
  texts: string[];

  constructor(options: { confirms?: boolean[]; selects?: string[]; multiselects?: string[][]; texts?: string[] } = {}) {
    this.confirms = options.confirms ?? [];
    this.selects = options.selects ?? [];
    this.multiselects = options.multiselects ?? [];
    this.texts = options.texts ?? [];
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

  async text(): Promise<string> {
    const value = this.texts.shift();
    if (value === undefined) {
      throw new Error("No fake text value available");
    }
    return value;
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
      expect(existsSync(join(fixture.homeDir, ".zbrain", "skills", "zbrain-ask", "SKILL.md"))).toBe(true);
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

  test("init writes central project registration, preserves existing CLAUDE.md, and links runtime assets", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi({
      selects: ["programming"],
      multiselects: [["claude_rules", "skills", "agents", "mcp"]],
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

      const db1 = openDb(join(fixture.homeDir, ".zbrain"));
      const binding1 = readProject(db1, fixture.projectDir);
      expect(binding1?.workspace).toBe("programming");
      expect(binding1?.project_root).toBe(fixture.projectDir);
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# Existing");
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# zbrain Integration");
      expect(existsSync(join(fixture.projectDir, ".claude", "skills", "zbrain-ask", "SKILL.md"))).toBe(true);
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
      multiselects: [["claude_rules", "skills", "agents", "mcp"]],
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

      expect(existsSync(join(claudeDir, "zbrain.json"))).toBe(false);
      expect(existsSync(join(claudeDir, "zwiki.json"))).toBe(false);
      expect(existsSync(join(claudeDir, "skills", "zbrain-ask", "SKILL.md"))).toBe(true);
      expect(existsSync(join(claudeDir, "commands", "zbrain:ask.md"))).toBe(false);
      expect(existsSync(join(claudeDir, "agents", "legacy-agent.md"))).toBe(false);
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).toContain("# zbrain Integration");
      expect(readFileSync(join(fixture.projectDir, "CLAUDE.md"), "utf8")).not.toContain("# zwiki Integration");
      const db2 = openDb(join(fixture.homeDir, ".zbrain"));
      expect(readProject(db2, fixture.projectDir)?.project_root).toBe(fixture.projectDir);

      const skillDir = join(claudeDir, "skills", "zbrain-ask");
      if (lstatSync(skillDir).isSymbolicLink()) {
        expect(readlinkSync(skillDir)).toContain(".zbrain/skills/zbrain-ask");
      }
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });
  test("init can integrate Codex rules while keeping project config in ~/.zbrain", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi({
      selects: ["programming"],
      multiselects: [["codex_rules"]],
    });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      writeFileSync(join(fixture.projectDir, "AGENTS.md"), "# Existing Codex Rules\n");

      await runInit({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      const db3 = openDb(join(fixture.homeDir, ".zbrain"));
      const projectEntry = readProject(db3, fixture.projectDir);

      expect(readFileSync(join(fixture.projectDir, "AGENTS.md"), "utf8")).toContain("# Existing Codex Rules");
      expect(readFileSync(join(fixture.projectDir, "AGENTS.md"), "utf8")).toContain("# zbrain Integration");
      expect(projectEntry?.runtimes).toEqual(["codex"]);
      expect(existsSync(join(fixture.projectDir, ".claude", "zbrain.json"))).toBe(false);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("learn records source material into workspace evidence sources", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      await runLearn({
        ui,
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        type: "paste",
        origin: "test",
        label: "Runtime note",
        rawContent: "Prefer reversible changes.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      const evidenceRoot = join(fixture.homeDir, ".zbrain", "workspaces", "programming", "evidence");
      const rawFile = join(evidenceRoot, "sources", "2026-05-25-paste-runtime-note", "raw.md");
      expect(existsSync(rawFile)).toBe(true);
      const db4 = openDb(join(fixture.homeDir, ".zbrain"));
      const evidenceRow = readEvidence(db4, "programming", "2026-05-25-paste-runtime-note");
      expect(evidenceRow?.label).toBe("Runtime note");
      expect(ui.notes.join("\n")).toContain("next: zbrain ingest review 2026-05-25-paste-runtime-note");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("ingest list, review, and apply process learned evidence", async () => {
    const fixture = makeFixture();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });
      await runLearn({
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        label: "Runtime note",
        rawContent: "Prefer reversible changes.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      const evidenceId = "2026-05-25-paste-runtime-note";
      const listUi = new FakeUi();
      await runIngestList({ ui: listUi, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir }, workspace: "programming" });
      await runIngestReview(evidenceId, {
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        fact: "Prefer reversible changes.",
        wikiPath: "axioms/reversible-changes.md",
        nowIso: "2026-05-25T02:01:00.000Z",
      });
      await runIngestApply(evidenceId, {
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        path: "axioms/reversible-changes.md",
        content: "# Reversible Changes\n\nPrefer reversible changes.\n",
        nowIso: "2026-05-25T02:02:00.000Z",
      });

      expect(listUi.notes.join("\n")).toContain("zbrain ingest review");
      expect(readFileSync(join(fixture.homeDir, ".zbrain", "workspaces", "programming", "axioms", "reversible-changes.md"), "utf8")).toContain("Prefer reversible changes.");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("apply reindexes the workspace so applied knowledge becomes searchable", async () => {
    const fixture = makeFixture();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });
      await runLearn({
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        label: "Reindex note",
        rawContent: "Reindex after apply.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      const evidenceId = "2026-05-25-paste-reindex-note";
      await runIngestReview(evidenceId, {
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        fact: "Reindex after apply.",
        wikiPath: "axioms/reindex.md",
        nowIso: "2026-05-25T02:01:00.000Z",
      });

      const indexCalls: string[][] = [];
      await runIngestApply(evidenceId, {
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        path: "axioms/reindex.md",
        content: "# Reindex\n\nReindex after apply.\n",
        nowIso: "2026-05-25T02:02:00.000Z",
        qmdRunner: (args) => {
          if (args[0] === "index") {
            indexCalls.push(args);
          }
          return { stdout: "", stderr: "", exitCode: 0 };
        },
      });

      // ISSUE-002 regression guard: apply must reindex qmd, or `ask` can never
      // retrieve the freshly applied document.
      expect(indexCalls.length).toBe(1);
      expect(indexCalls[0]).toContain("programming");
      expect(
        readFileSync(join(fixture.homeDir, ".zbrain", "workspaces", "programming", "axioms", "reindex.md"), "utf8"),
      ).toContain("Reindex after apply.");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("review records QA status that the apply gate enforces end-to-end (ISSUE-003)", async () => {
    const fixture = makeFixture();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });
      await runLearn({
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        label: "Gate note",
        rawContent: "Needs external confirmation.",
        nowIso: "2026-05-25T02:00:00.000Z",
      });

      const evidenceId = "2026-05-25-paste-gate-note";
      // Record an unresolved P0 — no answered fact required for a blocking status.
      await runIngestReview(evidenceId, {
        ui: new FakeUi(),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        workspace: "programming",
        severity: "P0",
        status: "awaiting_external",
        nowIso: "2026-05-25T02:01:00.000Z",
      });

      // Apply must read the persisted answer and block — the loop is no longer self-certified.
      await expect(
        runIngestApply(evidenceId, {
          ui: new FakeUi(),
          pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
          workspace: "programming",
          path: "axioms/gate.md",
          content: "# Gate\n",
          nowIso: "2026-05-25T02:02:00.000Z",
        }),
      ).rejects.toThrow("QA gate blocked");

      expect(
        existsSync(join(fixture.homeDir, ".zbrain", "workspaces", "programming", "axioms", "gate.md")),
      ).toBe(false);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("ask writes current task context for the active workspace", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });
      await runInit({
        ui: new FakeUi({ selects: ["programming"], multiselects: [[]] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
      });

      await runAsk("reversible changes", {
        ui,
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        adapter: {
          searchWorkspace: ({ workspace }) => {
            expect(workspace).toBe("programming");
            return [{ path: "/ws/programming/axioms/reversible.md", score: 5, snippet: "reversible", body: "Prefer reversible changes." }];
          },
        },
      });

      expect(ui.notes.join("\n")).toContain("results: 1");
      expect(ui.notes.join("\n")).toContain(".zbrain/projects");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("ask with non-numeric --limit falls back to the default of 8 (ISSUE-022)", async () => {
    const fixture = makeFixture();
    const ui = new FakeUi();
    let seenLimit: number | undefined;

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });
      await runInit({
        ui: new FakeUi({ selects: ["programming"], multiselects: [[]] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
      });

      await runAsk("solid design", {
        ui,
        limit: "abc",
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        adapter: {
          searchWorkspace: ({ limit }) => {
            seenLimit = limit;
            return Array.from({ length: 10 }, (_, i) => ({
              path: `/ws/programming/axioms/a${i}.md`,
              score: 10 - i,
              snippet: `s${i}`,
            }));
          },
        },
      });

      expect(seenLimit).toBe(8);
      expect(ui.notes.join("\n")).toContain("results: 8");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("ingest review with a non-existent --workspace errors like learn (ISSUE-013)", async () => {
    const fixture = makeFixture();

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-25T01:00:00.000Z",
      });

      await expect(
        runIngestReview("any-id", {
          ui: new FakeUi(),
          pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
          workspace: "nope",
        }),
      ).rejects.toThrow('Workspace "nope" does not exist.');
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });
});

describe("interactive mode", () => {
  test("shows setup option and runs setup when runtime does not exist", async () => {
    const fixture = makeFixture();
    // selects[0] = menu action "setup"
    const ui = new FakeUi({ selects: ["setup"] });

    try {
      await runInteractive({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(existsSync(join(fixture.homeDir, ".zbrain", "config.yml"))).toBe(true);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("shows workspace_create option after setup and scaffolds preset workspace", async () => {
    const fixture = makeFixture();
    const setupUi = new FakeUi();
    // selects[0] = menu action "workspace_create", selects[1] = preset name "research"
    const ui = new FakeUi({ selects: ["workspace_create", "research"], confirms: [true] });

    try {
      await runSetup({ ui: setupUi, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runInteractive({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(existsSync(join(fixture.homeDir, ".zbrain", "workspaces", "research", "workspace.md"))).toBe(true);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("workspace_create with custom name prompts for text input", async () => {
    const fixture = makeFixture();
    const setupUi = new FakeUi();
    // selects[0] = "workspace_create", selects[1] = "__custom__", texts[0] = custom name
    const ui = new FakeUi({ selects: ["workspace_create", "__custom__"], texts: ["my-notes"], confirms: [true] });

    try {
      await runSetup({ ui: setupUi, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runInteractive({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      expect(existsSync(join(fixture.homeDir, ".zbrain", "workspaces", "my-notes", "workspace.md"))).toBe(true);
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("shows init option once a workspace exists", async () => {
    const fixture = makeFixture();
    // selects[0] = "init", selects[1] = workspace for init, multiselects[0] = inject targets
    const ui = new FakeUi({
      selects: ["init", "programming"],
      multiselects: [["claude_rules"]],
    });

    try {
      await runSetup({ ui: new FakeUi(), pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });
      await runWorkspaceCreate("programming", {
        ui: new FakeUi({ confirms: [true] }),
        pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir },
        nowIso: "2026-05-28T00:00:00.000Z",
      });
      await runInteractive({ ui, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } });

      const db5 = openDb(join(fixture.homeDir, ".zbrain"));
      expect(readProject(db5, fixture.projectDir)).not.toBeNull();
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });

  test("propagates Command cancelled so the top-level handler in index.ts can exit cleanly", async () => {
    const fixture = makeFixture();
    // Simulate Ctrl+C at the top-level menu: select throws "Command cancelled"
    const cancelUi = new FakeUi();
    cancelUi.select = async () => { throw new Error("Command cancelled"); };

    try {
      await expect(
        runInteractive({ ui: cancelUi, pathOptions: { cwd: fixture.projectDir, homeDir: fixture.homeDir } })
      ).rejects.toThrow("Command cancelled");
    } finally {
      rmSync(fixture.root, { recursive: true, force: true });
    }
  });
});
