# zbrain

`zbrain` is a Go-native CLI for local-first trusted memory and workspace-isolated agent context.

The current slice stores canonical Markdown claims, immutable local evidence snapshots, and a disposable SQLite FTS5 index. `zbrain ask` returns trusted context JSON only; it does not call an LLM.

## Current command surface

```bash
zbrain setup
zbrain workspace create <name>
zbrain workspace current
zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>]
zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived>
zbrain claim approve <id>
zbrain claim supersede <id>
zbrain claim revoke <id> --reason <reason>
zbrain reindex [--workspace <name>]
zbrain ask [--workspace <name>] [--include <name>] <query>
zbrain version
```

## Development

Prerequisite: Go 1.24.

```bash
make test
make build
make smoke
```

Equivalent raw commands:

```bash
go test ./...
go build -o dist/zbrain ./cmd/zbrain
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

Use `ZBRAIN_HOME` to isolate tests or experiments from real runtime data.

## Runtime layout

After `zbrain setup` and `zbrain workspace create research`:

```text
~/.zbrain/
  config.yml
  agents/
  engine/
  skills/
  templates/
  indexes/
    research.sqlite
  workspaces/
    research/
      workspace.md
      wiki/
        axioms/
        mental-models/
        projects/
        decisions/
      evidence/
        _index.md
        sources/
        analysis/
        qa/
        applied/
        archive/
```

`zbrain workspace current` prints JSON with:

- `project_root`
- `workspace`
- `secondary_workspaces`

## Trusted memory model

- One Markdown file is one atomic claim.
- Claim lifecycle is `draft -> approved -> superseded|revoked`.
- Only approved claims are trusted by `zbrain ask`.
- Drafts appear only as `promotion_candidates`.
- External factual claims need local immutable evidence snapshots.
- Approved claims are replaced through superseding claims, not in-place mutation.
- Explicit conflicts make `zbrain ask` fail closed with `status: "blocked"`.
- Missing approved memory returns `status: "gap"`.
- Secondary workspaces are searched only when explicitly passed with `--include`.

## Repository layout

```text
cmd/zbrain/         CLI entrypoint
internal/cli/       command dispatch and user-facing behavior
internal/runtime/   runtime paths, config, assets, claims, evidence, index, query
assets/             embedded runtime assets copied by setup
docs/               durable plans and supporting docs
```

## Reset note

The current implementation is intentionally Go-native and minimal. It does not include hosted sync, vector search, network crawling, background services, or model-provider integration.
