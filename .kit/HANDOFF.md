# HANDOFF

**Branch:** `master` (synced with `origin/master` — clean, nothing uncommitted)
**Continuity mode:** harness (post-phase)
**Active phase:** post-p7 (all roadmap phases complete)
**Latest cook run:** `.kit/runs/cook/20260525-0320-p7-release-hardening.md`
**Latest check verdict:** APPROVE (`.kit/reports/check/20260528-1200-interactive-cli-and-doc-rewrite.md`)
**Session date:** 2026-05-28

---

## Completed This Session

1. **Rewrote `wiki-spec.md`** — full rewrite from stale wiki-template analysis into living zbrain product spec (CLI layer, engine/skills/workspace architecture, BM25 retrieval pipeline, evidence pipeline, MVP-1 scope).
2. **Rewrote `AGENTS.md`** — now describes actual Bun/TypeScript project layout, bun commands, asset authoring rules. All wiki-template/ references removed.
3. **Added interactive guided menu** (`src/commands/interactive.ts`) — state-aware menu on `zbrain` (no args + TTY): shows only relevant options based on whether runtime and workspaces exist; preset workspace names; custom name via `ui.text()`.
4. **Added `text()` to `CommandUi`** (`src/commands/ui.ts`) — new prompt method + clackUi implementation + FakeUi support in both integration test files.
5. **Fixed Ctrl+C crash** (`src/index.ts`) — top-level try/catch catches `"Command cancelled"` → `process.exit(0)`; before this fix, cancelling any prompt printed an unhandled exception stack trace.
6. **Routed banner through `ui.intro()`** (`src/commands/interactive.ts`) — moved `console.log(BANNER)` to `ui.intro(BANNER)` to keep all output through the CommandUi abstraction; eliminates 4× banner noise in test output.
7. **Removed `wiki-template/`** — deleted all 46 legacy files (~8000 lines); migration to `assets/` was complete.
8. **57/57 tests pass** — 5 new tests: 4 interactive menu paths + 1 Ctrl+C regression guard.
9. **All 3 commits pushed** to `origin/master`.

---

## In Progress

**None** — working tree clean, all changes committed and pushed.

---

## Blockers

**None.**

---

## Open Doc Debt

- **`assets/README.md:5`** — still says "wiki-template/ is migration input material only" — stale post-deletion. Low priority (bundled into binary, not user-visible). One-line fix.
- **No wizard chaining** after first-run setup — user picks "Setup runtime," it completes, menu exits with no prompt to create a workspace next. UX improvement; explicitly out of scope for current session.
- **Dead code `assetGroupPaths`** in `helpers.ts:265` — pre-existing unused function returning `{ commands: ... }`. Safe to delete but not introduced by this session.

---

## Next Steps

1. **→ START HERE:** Run `zbrain setup && zbrain workspace create programming && zbrain init` via the interactive menu against real `~/.zbrain/` to verify first-run UX end-to-end. Acceptance walkthrough is at `docs/acceptance-walkthrough.md`.
2. Fix `assets/README.md:5` — remove or update the stale wiki-template reference (1 line).
3. UX: add wizard chaining — after "Setup runtime" completes, prompt "Create a workspace now?" → if yes, go directly to `promptWorkspaceName`. This is the biggest remaining first-run UX gap.
4. Delete dead code `assetGroupPaths` in `helpers.ts:265`.

---

## Key Decisions This Session

- **Interactive mode = no-args TTY only** — `zbrain setup`, `zbrain init`, etc. still work exactly as before; interactive mode only activates when no args and `process.stdin.isTTY`. Non-TTY (CI, scripts) falls through to Commander normally.
- **Cancellation caught at top level only** — `clackUi` throws `"Command cancelled"` on Ctrl+C; the catch is in `index.ts` entry point only. `runInteractive` propagates the error — the regression test confirms this contract.
- **Banner via `ui.intro()`** — routes through CommandUi abstraction so FakeUi silently swallows it in tests; no test noise, no behavioral difference in real TTY.
- **`text()` added to CommandUi interface** — required method (not optional) so TypeScript enforces all FakeUi implementations stay complete.

---

## Environment

- Working directory: `/home/tinhpt/Lab/zbrain`
- Runtime home: `~/.zbrain/`
- Active workspace for this project: `programming` (`.claude/zbrain.json`)
- Tests: 57/57 pass (`bun test --run`)
- Build: 40 modules, ~30ms (`bun run build.ts`)
- Binary: `dist/zbrain` — interactive mode works; `--help` works; Ctrl+C exits cleanly
