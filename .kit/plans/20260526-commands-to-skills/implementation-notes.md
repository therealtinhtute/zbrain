# Implementation Notes: commands → skills migration

Date: 2026-05-26  
Status: complete — 52/52 tests pass, types clean

---

## What changed (file map)

| Action | File |
|--------|------|
| Created (×5) | `assets/skills/zbrain-{ask,learn,reflect,reindex,workspace}/SKILL.md` |
| Created (×1) | `assets/skills/zbrain-learn/references/pipeline.md` |
| Deleted (×5) | `assets/commands/{ask,learn,reflect,reindex,workspace}.md` |
| Deleted | `assets/commands/` directory |
| Modified | `src/core/fs.ts` |
| Modified | `src/commands/helpers.ts` |
| Modified | `src/commands/init.ts` |
| Regenerated | `src/generated/bundled-assets.ts` |
| Updated (×4) | `tests/assets.test.ts`, `tests/core/assets.test.ts`, `tests/assets/commands.test.ts`, `tests/commands.integration.test.ts` |

---

## Decisions not in the spec

### 1. Colon-named file cleanup added (not planned)

**What**: The legacy test had `commands/zbrain:ask.md` — a file with a literal colon in the name from an even older migration era. The plan only specified cleanup of `commands/zbrain/` (directory). But since we no longer sync `commands/` at all, `commands/zbrain:ask.md` would persist silently.

**Decision**: Added a loop in `helpers.ts` that removes any file in `.claude/commands/` starting with `zbrain:` or `zwiki:` when the `skills` inject target runs. This is surgical — it targets only the colon-named legacy artifacts, not the whole `commands/` directory.

**Why not in spec**: The spec covered the directory-based stale cleanup. The colon-file was an older legacy artifact that only showed up when reading the test.

---

### 2. `lstatSync` check before fallback in `createSymlinkOrCopy`

**What**: The original plan said "update `createSymlinkOrCopy` fallback: `copyFileSync` fails for directories → use `copyDirectory`". The implementation uses `lstatSync(target).isDirectory()` to decide which fallback to use, rather than catching a second exception from `copyFileSync`.

**Why**: Catching a second error from `copyFileSync(directory)` works but silently swallows the EISDIR error. Using `lstatSync` is explicit and doesn't rely on error-shape assumptions. Costs one stat call, which is negligible.

---

### 3. `syncAssetGroup` unchanged — works for directories as-is

**What**: The plan said I might need to update `syncAssetGroup` to handle directory trees. In practice, `syncAssetGroup` calls `createSymlinkOrCopy` per entry. Since `symlinkSync` works on directories (creates a directory symlink), and `copyDirectory` is now the fallback, `syncAssetGroup` already handles skill directories without changes.

**Tradeoff**: Skill directories install as **directory symlinks** pointing to `~/.zbrain/skills/zbrain-ask/`, not as copied directories. This means modifying a skill file in `.claude/skills/zbrain-ask/` would also change the runtime asset (same inode). This is consistent with how agents were already being installed.

---

### 4. `tests/assets/commands.test.ts` kept at same path (not renamed to `skills.test.ts`)

**Why**: The test file was rewritten completely to test skills, but renaming it would require updating any CI config that references it by name. The filename is slightly misleading now but the test descriptions are clear. If the user wants to rename it, it's a 1-line git mv.

---

### 5. Skills install to `.claude/skills/zbrain-*/` — project-local, not global

**What**: Skills are installed per-project into `.claude/skills/`, not into `~/.claude/skills/` (global). This matches the existing workspace-isolation design: zbrain skills are only active in projects that have been `zbrain init`-ed.

**Implication**: A user who runs `zbrain init` in `/project-A/` will have `zbrain:ask` available in project-A but not in project-B until they run `zbrain init` there too. This is intentional — workspace isolation requires the skill to know which workspace it's operating in.

---

### 6. `disable-model-invocation: true` preserved on `learn`, `reindex`, `workspace`

The original commands had `disable-model-invocation: true` on three skills. This frontmatter is preserved in the new SKILL.md files. It prevents Claude from calling the model when these skills are invoked (they're orchestrator/tool commands, not answer-generating skills). The spec didn't mention this; it was carried forward from the source files.

---

## Things I had to change from the plan

**Plan said**: "Delete `assets/commands/` after migration" — done.

**Plan said**: "Rename inject target `commands` → `skills`" — done. The string `"commands"` in `init.ts` `initTargetOptions` and in `helpers.ts` conditions is now `"skills"`. Existing tests that passed `["claude_rules", "commands", "agents", "mcp"]` as multiselects were updated to `["claude_rules", "skills", "agents", "mcp"]`.

**Plan said**: "`syncAssetGroup` needs update for directories" — not needed (see §3 above).

---

## Risks / Watch-fors going forward

- **Symlink + Windows**: On Windows, creating directory symlinks requires elevated permissions or Developer Mode. The `copyDirectory` fallback covers this — tested path exists in `createSymlinkOrCopy`. No test exercises this path because we're on Linux and symlinks succeed.
- **`zbrain:ask` visible in Claude skills list**: After `zbrain init`, `.claude/skills/zbrain-ask` is a directory symlink. Claude Code must traverse the symlink to find `SKILL.md`. Verified locally (these skills are already visible in this session as `zbrain:ask`, `zbrain:reflect` etc.).
- **Acceptance test**: The release acceptance test (`tests/release/acceptance.test.ts`) was not broken by this change — it passed in the full test run. Worth noting in case a manual smoke test is done against a rebuilt binary.
