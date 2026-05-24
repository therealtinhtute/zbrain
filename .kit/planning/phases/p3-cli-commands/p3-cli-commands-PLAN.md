# Phase Plan: P3 — CLI Commands

Inputs: P2 deliverables (workspace-resolver, asset-extractor, schemas)
Depends on: P1, P2

---

## Wave 1: Setup Command

### Task 1.1: zwiki setup
- Create src/commands/setup.ts
- Import: asset-extractor (extractAssets, checkQmd) from P2
- Flow:
  1. `intro({ title: 'zwiki setup' })`
  2. Check if ~/.zwiki/ exists → if yes, `confirm('~/.zwiki/ already exists. Re-extract assets?')`
  3. `spinner()` → extractAssets('~/.zwiki/')
  4. `spinner()` → checkQmd()
  5. If qmd missing: `note('qmd not found. Install: npm i -g @tobilu/qmd')`
  6. `note()` with summary of extracted files
  7. Generate ~/.zwiki/config.yml with default_workspace: '' (empty until workspace created)
  8. `outro('Setup complete!')`
- Wire up in src/index.ts replacing setup stub
- **Verification**: Run `bun src/index.ts setup` → ~/.zwiki/ created with engine/, templates/, commands/, agents/, config.yml
- **Verification**: Run again → prompts for re-extract confirmation
- **Touched**: src/commands/setup.ts, src/index.ts

---

## Wave 2: Workspace Create Command

### Task 2.1: zwiki workspace create
- Create src/commands/workspace.ts
- Flow:
  1. `intro({ title: 'zwiki — Create Workspace' })`
  2. `text({ message: 'Workspace name?' })` (validate: lowercase, no spaces, no special chars)
  3. `confirm({ message: 'Create workspace "{name}"?' })`
  4. `spinner()` → create ~/.zwiki/workspaces/{name}/ with subdirs: axioms/, mental-models/, projects/, decisions/, evidence/ (with sources/, analysis/, qa/, applied/, archive/), agents/
  5. Copy templates: workspace.md (fill name), evidence-index.md → _index.md
  6. Update ~/.zwiki/config.yml → set default_workspace if currently empty
  7. Update qmd-config.yml → add collection entry for new workspace
  8. `outro('Workspace "{name}" created!')`
- **Verification**: `bun src/index.ts workspace create` → prompts name → creates full directory tree
- **Verification**: Workspace.md has correct name in frontmatter
- **Touched**: src/commands/workspace.ts, src/index.ts

---

## Wave 3: Init Command

### Task 3.1: zwiki init
- Create src/commands/init.ts
- Import: workspace-resolver from P2
- Flow:
  1. `intro({ title: 'zwiki — Project Integration' })`
  2. Check ~/.zwiki/ exists → if not, suggest `zwiki setup` first, exit
  3. List available workspaces from ~/.zwiki/workspaces/
  4. `select({ message: 'Workspace for this project?' })` with workspace list
  5. `multiselect({ message: 'What to inject?' })`:
     - `.claude/zwiki.json` (always selected, not deselectable)
     - `CLAUDE.md rules`
     - `Slash commands (.claude/commands/)`
     - `Subagents (.claude/agents/)`
     - `MCP config (.claude/settings.local.json)`
  6. `spinner()` → execute injections:
     - Create .claude/ dir if needed
     - Write .claude/zwiki.json: { "workspace": "{name}" }
     - If CLAUDE.md selected: read existing, check for "## zwiki Integration" section, append if not present
     - If commands selected: symlink each .md from ~/.zwiki/commands/ → .claude/commands/
     - If agents selected: symlink each .md from ~/.zwiki/agents/ → .claude/agents/
     - If MCP selected: write/merge .claude/settings.local.json with qmd server entry
  7. `note()` listing all created/modified files
  8. `outro('Project initialized with zwiki!')`
- **Verification**: Run in a test project → .claude/zwiki.json exists with correct workspace
- **Verification**: Symlinks resolve (readlink shows ~/.zwiki/ paths)
- **Verification**: CLAUDE.md has zwiki section appended, existing content preserved
- **Verification**: Run again → detects existing section, doesn't duplicate
- **Touched**: src/commands/init.ts, src/index.ts

---

## Wave 4: Update Command

### Task 4.1: zwiki update
- Create src/commands/update.ts
- Flow:
  1. `intro({ title: 'zwiki — Update Assets' })`
  2. Check ~/.zwiki/ exists → if not, suggest setup first
  3. `spinner()` → extractAssets('~/.zwiki/') with overwrite
  4. `note()` with list of updated files
  5. `outro('Assets updated!')`
- **Verification**: Modify an engine file manually → run update → file restored to bundled version
- **Touched**: src/commands/update.ts, src/index.ts

---

## Stop Conditions
- Symlink creation fails → check if target exists, check OS permissions
- CLAUDE.md injection duplicates content → check section detection regex
- clack prompts don't render in CI → acceptable, CI not a target

## Escalation
- Symlinks not supported on target OS → fall back to file copy with warning
- CLAUDE.md format varies too much → use simple string search for "## zwiki Integration"
