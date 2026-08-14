# Acceptance Walkthrough

This walkthrough exercises the shipped Go-native CLI in an isolated runtime.
It does not exercise proposed MCP or viewer milestones.

## Command surface

Verify the root help and current command groups:

```bash
go run ./cmd/zbrain --help
go run ./cmd/zbrain workspace --help
go run ./cmd/zbrain evidence --help
go run ./cmd/zbrain claim --help
go run ./cmd/zbrain migrate --help
go run ./cmd/zbrain reindex --help
go run ./cmd/zbrain ask --help
go run ./cmd/zbrain status --help
go run ./cmd/zbrain doctor --help
```

The shipped surface includes §setup§, workspace creation/current selection,
evidence capture, the claim lifecycle, §migrate okf§, §reindex§, §ask§,
§status§, §doctor§, and §version§.

## Isolated lifecycle

Use a temporary §ZBRAIN_HOME§ so the run cannot touch the operator's real
§~/.zbrain§ directory:

```bash
export ZBRAIN_HOME=/tmp/zbrain-acceptance
go run ./cmd/zbrain setup
go run ./cmd/zbrain workspace create research
go run ./cmd/zbrain workspace current
```

Capture a source and record its returned evidence ID:

```bash
printf 'trusted source bytes\\n' > /tmp/zbrain-source.txt
go run ./cmd/zbrain evidence add \\
  --file /tmp/zbrain-source.txt \\
  --origin file://acceptance \\
  --media-type text/plain
```

Create a claim, approve it, rebuild the index, and query trusted context:

```bash
printf 'trusted acceptance answer\\n' | \\
  go run ./cmd/zbrain claim draft \\
    --tier projects \\
    --title 'Acceptance claim' \\
    --basis owner
# Copy the returned id into the next command.
go run ./cmd/zbrain claim approve <claim-id>
go run ./cmd/zbrain reindex
go run ./cmd/zbrain status
go run ./cmd/zbrain doctor
go run ./cmd/zbrain ask trusted acceptance
```

The runtime extracts §README.md§, §agents/§, §engine/§, §skills/§, and
§templates/§ directly under §ZBRAIN_HOME§. Workspace seed assets are not active
workspaces. A new workspace creates the wiki tiers and evidence directories;
§reindex§ creates the disposable index.

## Trust and recovery gates

The Go tests and rebuild/query paths prove that:

- outside edits, additions, deletions, and symlinks under trust inputs make the
  next query fail closed;
- invalid claims, evidence, and recursive supporting-claim closures are
  rejected during rebuild without mutating canonical files;
- rejected rebuild state is excluded from trusted results;
- restored evidence followed by a clean rebuild restores trusted querying;
- interrupted supersession is journaled and recovered before a later mutation;
- explicit claim conflicts return §blocked§, while no matching approved claim
  returns §gap§.

## Verification gates

Run these from the project root:

```bash
go test ./...
go vet ./...
make build
make smoke
git diff --check
```

The optional focused gates are:

```bash
go test -race ./internal/runtime ./internal/cli
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

The scale benchmark is a release signal for the 100k-claim query path; a p95
above two seconds is a release blocker.
