# HANDOFF

**Branch:** `master` (synced with `origin/master`)  
**Continuity mode:** harness (post-phase)  
**Active phase:** post-p7 (all roadmap phases complete)  
**Latest cook run:** `.kit/runs/cook/20260525-0320-p7-release-hardening.md`  
**Latest check verdict:** APPROVE with requests (`.kit/reports/check/20260526-1650-commands-to-skills.md`)  
**Session date:** 2026-05-26

---

## Completed This Session

1. **Migrated `assets/commands/` → `assets/skills/`** — converted 5 flat command files into skill-creator-compliant skill directories with SKILL.md, role, security, numbered workflows, invariants.
2. **Created `zbrain-learn/references/pipeline.md`** — 4-stage evidence pipeline detail (ingest/analyze/qa/apply) moved out of SKILL.md to stay under 150-line budget.
3. **Updated `src/core/fs.ts`** — `createSymlinkOrCopy` now handles directory targets via `lstatSync` + `copyDirectory` fallback (Windows safety).
4. **Updated `src/commands/helpers.ts`** — renamed inject target `commands` → `skills`; syncs `runtimeDir/skills/` → `.claude/skills/`; auto-removes stale `commands/zbrain/` directory and colon-named legacy files.
5. **Updated `src/commands/init.ts`** — UI option renamed "Slash commands" → "Agent skills", hint shows `.claude/skills`.
6. **Regenerated `src/generated/bundled-assets.ts`** — 6 skill files bundled, 0 command files.
7. **Updated 4 test files** — `tests/assets.test.ts`, `tests/core/assets.test.ts`, `tests/assets/commands.test.ts` (rewritten as skills test), `tests/commands.integration.test.ts`.
8. **Ran `/check full`** — gate passed (52/52 tests, types clean, build clean); applied 2 safe_auto fixes (restored `.claude/zbrain.json` + agents, fixed `init.ts:37` `initialValues`).

---

## In Progress

**None** — migration complete, gate passed, all tests green.

---

## Blockers

**None** — check verdict is APPROVE with requests, not REQUEST CHANGES.

---

## Open Requests (Doc Debt)

**README.md** has 5 stale references to `assets/commands/`, `.claude/commands/`, and "slash commands" terminology:
- Line 42: `files are stored as flat markdown under assets/commands/*.md`
- Line 74: `commands/`
- Line 81: `optional symlinked commands/ and agents/`
- Line 133: `slash commands are shipped as assets for project integration`

These should be updated to reflect `assets/skills/`, `.claude/skills/`, and "agent skills" terminology.

---

## Next Steps

1. **→ START HERE:** Update README.md lines 42, 74, 81, 133 — replace `commands/` references with `skills/`, "slash commands" with "agent skills".
2. Commit the migration: `git add -A && git commit -m "feat(skills): migrate commands to skill-creator format"`.
3. Optional: remove dead code `assetGroupPaths` in `helpers.ts:265` (pre-existing, not introduced by this diff).
4. Optional: run acceptance test against rebuilt binary to verify skills load correctly in `.claude/skills/zbrain-*/`.

---

## Key Decisions This Session

- **Skills install project-local** (`.claude/skills/`) not global — workspace isolation requires this; skills are only active in projects where `zbrain init` has run.
- **Directory symlinks** — `syncAssetGroup` creates directory symlinks for skill dirs (e.g., `.claude/skills/zbrain-ask/` → `~/.zbrain/skills/zbrain-ask/`), not copied directories. Consistent with how agents were already installed.
- **Colon-file cleanup added** — the old `commands/zbrain:ask.md` artifact would have persisted; added a loop in `helpers.ts` to remove any file in `.claude/commands/` starting with `zbrain:` or `zwiki:`.
- **`zbrain-learn` gets `references/pipeline.md`** — only skill with a reference file; others fit in 150 lines.

---

## Environment

- Working directory: `/home/tinhpt/Lab/zbrain`
- Runtime home: `~/.zbrain/` (legacy `~/.zwiki/` still exists with 7 workspaces)
- Active workspace for this project: `programming` (`.claude/zbrain.json`)
- Tests: 52/52 pass
- Build: 39 modules, 56ms
- Uncommitted changes: 13 modified/deleted tracked files + 6 new skill files + 1 implementation notes file

---

## Notes

- The two bugs found by `/check` (`.claude/zbrain.json` deletion, `init.ts:37` stale `initialValues`) were both introduced during this session's implementation and fixed via safe_auto before writing this handoff.
- Implementation notes with full decision log: `.kit/plans/20260526-commands-to-skills/implementation-notes.md`
