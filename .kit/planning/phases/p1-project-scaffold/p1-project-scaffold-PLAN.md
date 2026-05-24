# Phase Plan: P1 — Project Scaffold

Inputs: SPEC.md (Tech Stack, File Architecture)
Depends on: nothing (entry phase)

---

## Wave 1: Project Init

### Task 1.1: Initialize Bun project
- Run `bun init` in ~/Lab/zwiki/
- Create package.json with name "zwiki", type "module"
- Add all core dependencies: commander, @clack/prompts, js-yaml, marked, zod
- Add dev dependencies: typescript, vitest, @types/bun
- **Verification**: `bun install` succeeds, node_modules/ exists
- **Touched**: package.json, bun.lockb
- **Avoid**: modifying any existing .kit/ files

### Task 1.2: TypeScript config
- Create tsconfig.json: target ES2022, module ESNext, moduleResolution bundler, strict true, outDir dist/, rootDir src/
- **Verification**: `bun run tsc --noEmit` passes on empty src/
- **Touched**: tsconfig.json

### Task 1.3: Vitest config
- Create vitest.config.ts with default Bun preset
- Create tests/ directory with a smoke test (import package.json, assert name === "zwiki")
- **Verification**: `bun test` passes
- **Touched**: vitest.config.ts, tests/

---

## Wave 2: CLI Entry Point

### Task 2.1: Commander CLI shell
- Create src/index.ts with commander program
- Register 4 subcommands as stubs: setup, init, workspace, update
- Each stub prints "Not implemented yet" via @clack/prompts log.info()
- Add shebang line for binary execution
- **Verification**: `bun src/index.ts --help` shows all 4 subcommands
- **Verification**: `bun src/index.ts setup` prints clack-styled "Not implemented yet"
- **Touched**: src/index.ts
- **Avoid**: implementing actual command logic (that's P3)

---

## Wave 3: Asset Directory

### Task 3.1: Engine assets
- Create assets/engine/ with placeholder files:
  - system-prompt.md (agent role definition for personal knowledge retrieval)
  - constraints.md (hard rules: workspace isolation, citation required, no guessing)
  - retrieval-rules.md (3-stage pipeline spec — copy from SPEC architecture section)
  - evidence-pipeline.md (4-stage learning workflow — copy from SPEC)
  - qmd-config.yml (template with {workspace_path} placeholder for collection paths)
- **Verification**: All 5 files exist and contain meaningful content (not empty)
- **Touched**: assets/engine/

### Task 3.2: Template assets
- Create assets/templates/ with:
  - workspace.md (YAML frontmatter: name, domain, created_at; body: workspace description)
  - axiom.md (frontmatter: title, priority P0, source, created_at; body: core fact)
  - mental-model.md (frontmatter: title, priority P1, source; body: framework description)
  - project.md (frontmatter: title, priority P2, type [book|course|experiment], source; body)
  - evidence-index.md (_index.md template with state tracking table)
  - evidence-source.yaml (source.yaml template: id, type, workspace_at_ingest, ingested_at, state)
- **Verification**: Each template has valid YAML frontmatter parseable by js-yaml
- **Touched**: assets/templates/

### Task 3.3: Command + agent assets
- Create assets/commands/ with placeholder .md files: ask.md, learn.md, reflect.md, workspace.md, reindex.md
- Create assets/agents/ with placeholder .md files: wiki-planner.md, wiki-qmd-selector.md
- Create assets/claude-md-rules.md with the CLAUDE.md injection template from SPEC
- **Verification**: All files exist, each has valid YAML frontmatter
- **Touched**: assets/commands/, assets/agents/, assets/claude-md-rules.md

---

## Wave 4: Build Script

### Task 4.1: Bun compile script
- Create build.ts that:
  1. Uses Bun.build to compile src/index.ts
  2. Embeds assets/ directory contents (import as strings or use Bun's file embedding)
  3. Outputs to ./zwiki binary
- **Verification**: `bun run build.ts` produces ./zwiki binary
- **Verification**: `./zwiki --help` works
- **Verification**: Binary size is reasonable (~50-80MB)
- **Touched**: build.ts
- **Stop if**: Bun compile doesn't support asset embedding cleanly → escalate

---

## Stop Conditions
- Bun compile fails → check Bun version, try alternative embedding approach
- Dependencies don't resolve → check package names and versions
- TypeScript compilation errors → fix tsconfig

## Escalation
- Asset embedding in Bun binary unclear → research Bun docs, may need to inline assets as string constants
