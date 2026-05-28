# Plan: zbrain Skill Refactor — Ingest + State + Learn Split

## Scope

Split `zbrain:learn` monolith into semantic roles. Add `zbrain:ingest` and `zbrain:state`.

## Changes

1. New `assets/skills/zbrain-ingest/` — full pipeline driver for existing material
2. Refactor `assets/skills/zbrain-learn/SKILL.md` — active research entry point
3. New `assets/skills/zbrain-state/` — pipeline state navigator
4. Update `assets/skills/zbrain-reflect/SKILL.md` — route to zbrain:ingest
5. Update `assets/engine/evidence-rules.md` — reflect new skill split
6. Update `wiki-spec.md` — skills layer table

## Not in scope

- Pipeline state machine changes
- zbrain:ask, zbrain:reindex, zbrain:workspace changes
- Auto-driver behavior

## Verify

- `bun run generate:assets` — must succeed
- `bunx tsc --noEmit` — no type errors
- Skill files have correct frontmatter (name, description, version)
