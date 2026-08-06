# zbrain — Trusted Memory Spec

> Current authority for the Go-native trusted-memory runtime.
>
> This document defines the trust contract that `zbrain ask`, claim storage,
> evidence storage, and the disposable index must uphold. Future retrieval
> designs such as vectors, hybrid search, MCP, and sync belong to
> [`references/memory-engine-spec.md`](references/memory-engine-spec.md) and are
> not authorized by this document.

**Status:** current · **Created:** 2026-08-03 · **Canonical sources:** workspace
Markdown and immutable evidence snapshots

## 1. Product Boundary

`zbrain` is a local-first CLI for workspace-isolated trusted memory. It stores
OKF-style Markdown claim concepts, captures immutable local evidence snapshots,
builds a disposable SQLite FTS5 index, and returns trusted context JSON for
agents.

The trust rule is simple: an agent receives only explicit, approved, valid
claim concepts. Drafts, revoked or superseded claims, invalid documents,
gaps, conflicts, and stale indexes do not silently become answer material.

The current implementation is Go-native and standalone. It does not call an
LLM or model provider; `zbrain ask` returns JSON context for the caller to use.

## 2. Current Command Surface

The shipped commands are:

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

The command surface is authoritative from `zbrain --help`. Documentation and
embedded skills must not name commands or integrations that are absent from
that output.

## 3. Scope

### In scope

- Standalone Go binary with embedded runtime assets.
- Workspace setup and active-workspace resolution.
- OKF claim concepts in the four workspace wiki tiers.
- The zbrain trusted-memory profile on top of OKF frontmatter.
- Immutable local evidence snapshots.
- Claim lifecycle: `draft -> approved -> superseded|revoked`.
- Rebuildable per-workspace SQLite FTS5 indexes.
- Trusted context JSON from `zbrain ask`.
- Explicit migration from legacy `schema: zbrain.claim/v1` documents to OKF
  claim concepts.

### Out of scope

- LLM or model-provider calls.
- MCP integration.
- Vector databases and semantic search.
- Network crawling or web research.
- Hosted sync, team authentication, background services, or GUI.
- Session transcript storage.
- Generic OKF editing for arbitrary concept types.
- Git-backed versioning, backup, or synchronization of the runtime directory.

## 4. Runtime and Ownership Model

The default runtime root is `~/.zbrain/`; `ZBRAIN_HOME` overrides it for
isolated tests and smoke runs.

```text
~/.zbrain/
  config.yml
  README.md                  # extracted runtime asset
  agents/                    # extracted runtime agents
  engine/                    # extracted engine rules
  skills/                    # extracted skills and references
  templates/                 # extracted templates
  indexes/                   # created when a workspace is reindexed
    <workspace>.sqlite
    <workspace>.dirty        # present while a rebuild is incomplete
  workspaces/<workspace>/
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

`zbrain setup` extracts the embedded `README.md`, `agents/`, `engine/`, `skills/`, and `templates/` paths directly under the runtime root. The embedded `workspaces/` seed is not activated; `workspace create` creates the selected workspace and `reindex` creates its disposable index.

Markdown is canonical. SQLite is a disposable derived cache. A database row,
FTS index, or dirty marker must never be the only copy of trusted content.
Workspace boundaries are hard: no read may cross workspace boundaries unless
the caller explicitly passes the supported include option.

Runtime ownership permissions are part of this boundary contract:

- runtime, workspace, evidence, and index directories are owner-only (`0700`);
- mutable runtime metadata and canonical Markdown are owner-only read/write
  (`0600`);
- immutable evidence raw snapshots and metadata are owner-only read (`0400`);
- derived SQLite indexes and dirty markers are owner-only read/write (`0600`).

Fresh outputs and normal mutation paths normalize these modes without changing
canonical claim or evidence content semantics.

## 5. Claim Model

A trusted claim is an OKF-style Markdown document with:

```yaml
type: zbrain.claim
zbrain:
  profile: zbrain.trusted-memory/v1
  id: clm_<32 lowercase hex characters>
  tier: axioms | mental-models | projects | decisions
  basis: owner | evidence | derived
status: draft | approved | superseded | revoked
```

The stable trust identity is `zbrain.id`; the filesystem path is not the
identity. Approved claims are not edited in place. A correction is a new draft
that supersedes the approved claim, followed by explicit approval.

Approval records `verified.at`, `verified.by`, and `verified.digest`. The
approved claim is trusted only while its rendered content still matches that
digest.

Only claims with `status: approved` and a valid document shape may enter the
trusted answer set. Draft, superseded, revoked, legacy-unindexed, and invalid
claims remain visible to maintenance commands where useful but are excluded
from `zbrain ask`.

## 6. Verification Digest Contract

`ClaimVerificationDigest` hashes the canonical output of
`RenderClaimMarkdown` after removing `verified.at`, `verified.by`, and
`verified.digest` from the value being hashed. The digest therefore covers the
canonical claim metadata and body, including the claim identity, tier, basis,
relations, tags, sources, status, and generated metadata that the renderer
emits.

The contract is semantic through the canonical renderer, not a raw byte hash of
the file as typed by a user. Formatting that parses to the same canonical claim
may remain valid. A changed body, title, modeled frontmatter value, relation,
source, tag, or list order changes the canonical digest and invalidates the
approved claim.

For an approved claim:

- missing `verified.digest` is invalid;
- a recomputed digest that differs from `verified.digest` is invalid;
- a digest calculation error is invalid;
- the claim is not inserted into the derived FTS index;
- `zbrain reindex` reports its relative path and reason.

For non-approved claims, a missing verification digest is expected and is not
itself an error. `reindex` never repairs, rewrites, or silently backfills a
claim. The operator must use the normal claim lifecycle to correct it.

Legacy approved documents that do not carry an OKF verification digest are not
a special trust exception: migrate them with `zbrain migrate okf` and approve
them again, or they remain invalid and excluded.

## 7. Evidence Model

`zbrain evidence add` copies an already-local source file into the workspace as
an immutable snapshot. Each evidence item records an ID, origin, capture time,
media type, byte length, and SHA-256 digest.

The raw snapshot and metadata are made read-only. Verification recomputes the
hash and byte length. Raw evidence is untrusted source data, not trusted
context, and is not indexed by `zbrain ask`.

Evidence-based claims must reference existing immutable evidence IDs before
approval. Their `sources[].digest` uses the versioned
`sha256:evidence-v1:<hex>` snapshot format, which covers the exact metadata bytes
and the raw-byte length/SHA-256. The older raw-only `sha256:<hex>` source digest is
rejected with an explicit supersede-and-reapprove recovery path rather than being
silently trusted without metadata binding. Derived claims must reference approved
supporting claims or verified evidence. Owner claims require owner confirmation
metadata.

## 8. Derived Index and Freshness

Each workspace index is disposable and rebuildable from canonical Markdown and
evidence metadata. It uses SQLite FTS5 and indexes claim metadata plus body text
needed for lexical retrieval.

`zbrain reindex` must:

1. scan the workspace without mutating canonical Markdown or immutable evidence;
2. classify parse failures, digest failures, invalid evidence, invalid supporting-claim closures, dependency cycles, and legacy-unindexed documents;
3. insert only valid claims into the temporary index;
4. publish the rebuilt database and its `clean` or `rejected` state atomically;
5. remove the dirty marker only after publication succeeds; and
6. return counts plus enough path/reason data for an operator to repair
   invalid input.

`zbrain ask` must fail closed when:

- the workspace index is marked dirty;
- the index database does not exist;
- the index is stale relative to workspace Markdown; or
- a claim was rejected during the most recent rebuild.

`zbrain reindex` reports rejected claim paths and reasons. Any outside edit,
addition, deletion, or symlink under canonical wiki or evidence inputs makes
the next `zbrain ask` fail closed with the offending path and instructs the
operator to run `zbrain reindex`; retrieval is restored only after a clean
rebuild.

The full trust-input digest check belongs at the index boundary, so normal queries do not
need to reopen and rehash the entire workspace or every returned owner claim. The
outside-edit freshness check is a separate invariant: it prevents an already-built
index from serving content that changed after the rebuild. Because SQLite freshness
metadata is disposable, `zbrain ask` also verifies that each returned indexed claim
belongs to the published trust-input manifest and revalidates the current evidence
and supporting-claim closure for returned approved claims; these targeted checks do
not rebuild or rehash the workspace manifest.

Digest verification, recursive supporting-claim and evidence validation, and
outside-edit freshness checks are enforced at the rebuild boundary. Trusted queries
repeat only the returned approved claim's canonical binding and evidence/dependency
checks needed to keep disposable freshness metadata from bypassing current evidence
validation. Rejected rebuild state remains fail-closed through `zbrain ask`;
rebuilding never repairs, rewrites, deletes, or auto-revokes invalid canonical inputs.

## 9. Trusted Query Contract

`zbrain ask <query>` returns JSON and does not call an LLM. The response must
make the trust state visible:

- `ready` when approved, valid claims support the query;
- `gap` when no trusted claim supports the query; and
- an explicit error when the index is dirty, missing, or stale.

Approved claim results may include identity, tier, title, description, source
references, and relevance metadata. The query layer must not promote drafts,
raw evidence, invalid claims, or conflicts into trusted context merely to avoid
a gap.

## 10. Design Rules

1. Markdown and immutable evidence are the source of truth.
2. Derived indexes are disposable and must be rebuildable.
3. Trust checks fail closed rather than guessing.
4. Approved content is replaced through supersession, not in-place editing.
5. Workspace isolation is explicit and enforced at every read path.
6. Keep command handlers thin and runtime behavior in `internal/runtime/`.
7. Keep the implementation Go-native and minimal.
8. Every behavior change gets focused tests and an isolated runtime smoke.
9. Do not add vector search, MCP, LLM calls, or hosted sync under this spec;
   those require a separate scope decision.

## 11. Release Gate

The trusted-memory slice is releasable only when all of these pass:

```bash
go test ./...
make build
make smoke
```

The 100k-claim query benchmark must keep p95 below two seconds. A failed trust
check, stale index, missing workspace boundary, or unverified approved claim
is a release blocker even if the happy-path command succeeds.
