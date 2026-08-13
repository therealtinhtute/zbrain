# zbrain Trusted Agent Gateway Spec

**Status:** proposed implementation spec · **Created:** 2026-08-13 ·
**Authority:** amended `trusted-memory-spec.md` and the owner-approved
`references/zbrain-knowledge-architecture-brainstorm.md`

## Outcome

Expose zbrain's existing trusted-memory runtime to Codex/Claude through a local
stdio MCP gateway while keeping canonical Markdown and immutable evidence as the
only trust inputs. Agents can capture evidence, create drafts, ask trusted
memory, inspect health, and request a lifecycle challenge. Only a local owner
ceremony can grant approve, supersede, or revoke.

## Non-negotiable invariants

- No LLM, completion, provider SDK, API key, network fetch, remote bind, or
  background daemon in zbrain core.
- Workspace resolver and runtime locks are reused by every surface.
- Raw evidence is fenced as `untrusted_evidence`; it is never answer material.
- Drafts are promotion candidates and are excluded from trusted claims.
- Canonical mutations dirty the derived index and never auto-reindex.
- Challenge action digests bind workspace, operation, claim ID, canonical draft
  digest, superseded IDs, prior verification digest, and revoke reason.
- Challenge lifetime is 15 minutes; grant token lifetime is 5 minutes and
  atomically one-time consumed. Only token SHA-256 is persisted.

## MCP surface

Transport is `zbrain mcp serve` over stdio. stdout is protocol-only; diagnostics
go to stderr. The first release exposes exactly these tools:

1. `workspace_current`
2. `memory_ask`
3. `memory_status`
4. `memory_reindex`
5. `evidence_capture`
6. `claim_draft`
7. `claim_lifecycle` with `prepare|apply`

Resources are read-only:

- `zbrain://workspace/{workspace}/claim/{id}`
- `zbrain://workspace/{workspace}/evidence/{id}`

The MCP layer maps domain failures to `isError`, invalid parameters to
`-32602`, and genuine server failures to `-32603`. It does not expose grant,
approval UI, or mutation HTTP endpoints.

## Owner-pinned lifecycle

`claim_lifecycle prepare` returns a challenge ID, action summary, and SHA-256
action digest. The owner runs `zbrain approval show <challenge-id>` and
`zbrain approval grant <challenge-id>`, confirms a digest suffix in an
interactive TTY, and gives the one-time token to the agent. `apply` validates
the challenge, token hash, expiry, workspace, and current canonical draft under
the workspace lock before consuming the token and applying the transition.

The existing CLI approval path remains compatible and does not require an MCP
challenge. MCP lifecycle writes record `verified.by: owner:mcp` plus transition
authorization metadata containing challenge ID, method, and MCP client.

## Evidence spans

`EvidenceSpan` coordinates are 1-based and inclusive over exact raw UTF-8 bytes.
No CRLF normalization is allowed. The span digest binds the evidence snapshot
digest, `start-end`, and exact bytes. Approval and rebuild reject missing,
moved, out-of-range, binary, or digest-mismatched spans. Existing claims with
only `evidence_ids` render and digest as before.

## Diagnostics and viewer

`status` is machine-readable and includes claim/evidence counts, index state,
pending approvals, embedding coverage, and exact recovery action. `doctor` is
read-only, returns exit 0 for healthy, 2 for domain findings, and 1 for usage or
internal failures; embedding probes require an explicit flag.

`view` binds only loopback, serves embedded HTML/CSS/JS, escapes Markdown and raw
evidence, sends strict CSP and `nosniff`, has no CORS, and returns 405 for every
mutation method. It uses the same runtime services as CLI/MCP.

## Milestone order

1. diagnostics and span validation;
2. stdio MCP gateway;
3. owner-pinned challenge lifecycle;
4. optional hybrid retrieval;
5. read-only viewer.

Each milestone must pass `go test ./...`, `go vet ./...`, `go test -race
./internal/runtime ./internal/cli`, `make build`, `make smoke`, `git diff
--check`, and `CGO_ENABLED=0 go build ./cmd/zbrain`.
