# Repository Guidelines

## Project Structure & Module Organization

This repository is currently template- and planning-focused. Root files include [README.md](/home/tinhpt/Lab/zbrain/README.md) plus planning material under `.kit/planning/`. The main implementation lives in `wiki-template/`:

- `wiki-template/agents/`: engine prompts, constraints, and pipeline rules
- `wiki-template/templates/`: reusable Markdown and JSON/YAML templates
- `wiki-template/.claude/`: Claude commands and agent definitions
- `wiki-template/scripts/`: install, lint, validate, and test utilities

Keep new engine logic inside `wiki-template/`; avoid mixing workspace-specific content into the root.

## Build, Test, and Development Commands

This repo does not have a single root build. Use the Python and shell utilities in `wiki-template/scripts/`:

- `bash wiki-template/scripts/install-to-claude.sh --dry-run`: preview Claude sync changes
- `python3 wiki-template/scripts/lint-wiki.py --workspace <name> --wiki-root <path>`: detect broken links and orphaned docs
- `python3 wiki-template/scripts/validate.py --file <path> --pretty`: run Layer 1 validation against a code file
- `python3 wiki-template/scripts/test_lint_wiki.py`: run the self-contained regression tests for the linter

Run commands from the repository root so relative paths stay consistent.

## Coding Style & Naming Conventions

Python scripts use 4-space indentation, standard-library-first implementations, and descriptive snake_case names. Markdown files should use short sections, relative links, and stable filenames such as `workspace.md`, `knowledge-map.md`, and `patterns-index.md`. Keep templates and command docs explicit; write for both humans and agents.

## Testing Guidelines

Testing is script-based today. Add or update focused tests near the script they cover, following the existing `test_*.py` naming pattern in `wiki-template/scripts/`. Prefer self-contained tests that use temporary directories and avoid touching real user data. For doc or link changes, run `lint-wiki.py`; for validator changes, run `validate.py` on a representative fixture.

## Commit & Pull Request Guidelines

Use Conventional Commit style, matching recent history such as `feat(template): ...` and `docs(planning): ...`. Keep scopes specific to the area changed. PRs should include a short summary, affected paths, validation commands run, and screenshots only when command UX or rendered docs materially change.

## Security & Configuration Tips

Do not commit secrets, local workspace data, or populated `~/.claude` output. Prefer `--dry-run` before install scripts, and treat workspace isolation as a hard rule when adding new wiki guidance.
