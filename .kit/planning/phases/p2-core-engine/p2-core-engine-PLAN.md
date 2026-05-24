# Phase Plan: P2 — Core Engine

Inputs: P1 deliverables (project initialized, assets exist)
Depends on: P1

---

## Wave 1: Parsers

### Task 1.1: YAML parser with Zod validation
- Create src/parsers/yaml.ts
- Export: `parseYaml<T>(content: string, schema: ZodSchema<T>): T`
- Export: `parseYamlFile<T>(path: string, schema: ZodSchema<T>): Promise<T>`
- Handle: file not found → return undefined (not throw)
- **Verification**: `bun test tests/parsers/yaml.test.ts` — valid YAML, invalid schema, missing file
- **Touched**: src/parsers/yaml.ts, tests/parsers/yaml.test.ts

### Task 1.2: Markdown frontmatter parser
- Create src/parsers/markdown.ts
- Export: `parseFrontmatter(content: string): { frontmatter: Record<string, unknown>, body: string }`
- Uses marked for markdown, js-yaml for frontmatter extraction
- **Verification**: `bun test tests/parsers/markdown.test.ts` — with frontmatter, without, empty
- **Touched**: src/parsers/markdown.ts, tests/parsers/markdown.test.ts

---

## Wave 2: Workspace Resolver

### Task 2.1: Config schemas
- Create src/core/schemas.ts with Zod schemas:
  - `ZwikiProjectConfig`: { workspace: string }
  - `ZwikiGlobalConfig`: { default_workspace: string, workspaces_path?: string }
  - `SourceYaml`: { id: string, type: string, workspace_at_ingest: string, ingested_at: string, state: string }
  - `EvidenceState`: enum — ingested, analyzed, qa_in_progress, qa_awaiting_external, qa_done, applied, archived
- **Verification**: `bun test tests/core/schemas.test.ts` — valid/invalid payloads
- **Touched**: src/core/schemas.ts, tests/core/schemas.test.ts

### Task 2.2: Workspace resolver
- Create src/core/workspace-resolver.ts
- Export: `resolveWorkspace(cwd: string): Promise<{ workspace: string, source: 'project' | 'global' | 'auto' }>`
- Priority chain:
  1. `{cwd}/.claude/zwiki.json` → field "workspace"
  2. `~/.zwiki/config.yml` → field "default_workspace"
  3. Single workspace auto-detect (list dirs in ~/.zwiki/workspaces/, if exactly 1 → use it)
  4. Throw `WorkspaceNotFound` error
- Export: `getWorkspacePath(workspace: string): string` → `~/.zwiki/workspaces/{workspace}`
- **Verification**: `bun test tests/core/workspace-resolver.test.ts` — all 4 priority levels + error case
- **Touched**: src/core/workspace-resolver.ts, tests/core/workspace-resolver.test.ts

---

## Wave 3: Evidence State Machine

### Task 3.1: State machine transitions
- Create src/core/evidence-state-machine.ts
- Export: `transition(current: EvidenceState, action: 'analyze' | 'start_qa' | 'complete_qa' | 'defer_qa' | 'apply' | 'archive'): EvidenceState`
- Valid transitions:
  - ingested + analyze → analyzed
  - analyzed + start_qa → qa_in_progress
  - qa_in_progress + complete_qa → qa_done
  - qa_in_progress + defer_qa → qa_awaiting_external
  - qa_awaiting_external + complete_qa → qa_done
  - qa_done + apply → applied
  - applied + archive → archived
- Invalid transitions → throw `InvalidTransition` error
- Export: `validateWorkspaceLock(sourceWorkspace: string, activeWorkspace: string): void` — throws if mismatch (I-2)
- Export: `validateQAGate(questions: Question[]): void` — throws if P0/P1 awaiting_external (I-3)
- **Verification**: `bun test tests/core/evidence-state-machine.test.ts` — all valid transitions, all invalid transitions, workspace lock violation, QA gate violation
- **Touched**: src/core/evidence-state-machine.ts, tests/core/evidence-state-machine.test.ts

---

## Wave 4: Asset Extractor

### Task 4.1: Asset extractor
- Create src/core/asset-extractor.ts
- Export: `extractAssets(targetDir: string): Promise<{ extracted: string[], skipped: string[] }>`
  - targetDir defaults to ~/.zwiki/
  - Creates directory structure: engine/, templates/, commands/, agents/
  - Copies bundled assets to target (overwrite existing)
  - Returns list of extracted file paths
- Export: `checkQmd(): Promise<{ installed: boolean, version?: string }>`
  - Runs `qmd --version` via Bun.spawn
  - Returns installed status
- **Verification**: `bun test tests/core/asset-extractor.test.ts` — extract to temp dir, verify all files exist, verify directory structure
- **Touched**: src/core/asset-extractor.ts, tests/core/asset-extractor.test.ts

---

## Stop Conditions
- State machine transition logic unclear → re-read SPEC evidence state machine diagram
- Workspace resolver can't read config → check file permissions, YAML parse errors

## Escalation
- If Bun's fs API behaves differently from Node's → check Bun docs for compatibility
