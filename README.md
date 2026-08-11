# zbrain

`zbrain` is a Go-native CLI for local-first trusted memory and workspace-isolated agent context.

The current slice stores canonical OKF-style Markdown claim concepts, immutable local evidence snapshots, and a disposable SQLite FTS5 index. `zbrain ask` returns trusted context JSON only; it does not call an LLM.

**License:** [MIT](LICENSE) · **Contributing:** [CONTRIBUTING.md](CONTRIBUTING.md) · [Changelog](CHANGELOG.md)

## Install

Requires Go 1.24+.

```bash
go install github.com/therealtinhtute/zbrain@latest
```

Then run `zbrain setup`. Use `ZBRAIN_HOME` to isolate test runs from real runtime data.

## Current command surface

```text
zbrain setup
zbrain workspace create <name>
zbrain workspace current
zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]
zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
zbrain claim approve <id> [--workspace <name>]
zbrain claim supersede <id> --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
zbrain claim revoke <id> --reason <reason> [--workspace <name>]
zbrain migrate okf [--workspace <name>]
zbrain reindex [--workspace <name>]
zbrain ask [--workspace <name>] [--include <name>]... <query>
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
  README.md                  # extracted runtime asset
  agents/                    # extracted runtime agents
  engine/                    # extracted engine rules
  skills/                    # extracted skills and references
  templates/                 # extracted templates
  indexes/                   # created by zbrain reindex
    research.sqlite
    research.dirty            # present while a rebuild is incomplete
  workspaces/
    research/
      workspace.md
      agents/
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

`zbrain setup` extracts embedded files directly under the runtime root. The
embedded workspace seed is not activated; `workspace create` creates the
selected workspace and `reindex` creates its disposable index.

`zbrain workspace current` prints JSON with:

- `project_root`
- `workspace`
- `secondary_workspaces`

## OKF trusted memory model

- One Markdown file is one OKF concept document.
- Trusted zbrain claim concepts use top-level `type: zbrain.claim` plus `zbrain.profile: zbrain.trusted-memory/v1`.
- zbrain keeps stable `clm_<32 hex>` IDs in `zbrain.id`; OKF path identity is not used as the trust identity.
- Claim lifecycle is `draft -> approved -> superseded|revoked`.
- Only approved claim concepts are trusted by `zbrain ask`.
- Drafts appear only as `promotion_candidates`.
- External factual claims need local immutable evidence snapshots.
- Approval verifies referenced evidence hashes and records OKF `sources` plus `verified` metadata.
- `zbrain reindex` rejects approved claims whose canonical content no longer matches `verified.digest` and reports the offending path.
- `zbrain ask` fails closed when the index is dirty, missing, stale after an outside edit, or contains rejected claims.
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
