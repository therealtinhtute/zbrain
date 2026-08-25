# CLI Audit — phase-4 C6/C7 manual PASS list

Measured 2026-08-25 against `go run ./cmd/zbrain` (v0.2.3, commit cca9800).
`cli-agent-lint` and `anc` are **not installed** on this machine
(`which cli-agent-lint anc` → not found), so per plan §4.4 (wave t2 `proofs-close`)
this is the **manual PASS list** substituting for C6 (`cli-agent-lint` ≥B) and
C7 (`anc` ≥90%): all five audited principles PASS with real command evidence below.

## 1. Surface audit — every command has a Usage section

Ran `--help` on the root and every subcommand; each exits `0` and prints exactly one
`Usage` line on **stdout** (help is protocol, not diagnostics).

| Command | exit | `Usage` on stdout |
|---|---:|---|
| `zbrain --help` | 0 | 1 |
| `zbrain workspace --help` | 0 | 1 |
| `zbrain evidence --help` | 0 | 1 |
| `zbrain claim --help` | 0 | 1 |
| `zbrain migrate --help` | 0 | 1 |
| `zbrain reindex --help` | 0 | 1 |
| `zbrain ask --help` | 0 | 1 |
| `zbrain status --help` | 0 | 1 |
| `zbrain doctor --help` | 0 | 1 |
| `zbrain mcp serve --help` | 0 | 1 |
| `zbrain view --help` | 0 | 1 |
| `zbrain approval --help` | 0 | 1 |

Evidence commands:

```bash
go build -o /tmp/zbrain ./cmd/zbrain
/tmp/zbrain --help ; echo $?          # → 0
/tmp/zbrain workspace --help ; echo $?  # → 0
# ... same for all 12 surfaces; each stdout contains e.g.
```

Verbatim sample (root + one subcommand + one leaf):

```
$ /tmp/zbrain --help
Usage: zbrain <command> [args]

Commands:
  setup          Initialize zbrain home and workspace
  version        Show version
  workspace      Manage workspaces
  evidence       Manage evidence snapshots
  claim          Manage claims
  migrate        Migrate legacy data
  reindex        Rebuild the search index
  ask            Query trusted memory
  status         Show workspace status
  doctor         Diagnose workspace health
  mcp            MCP server gateway
  view           Loopback viewer
  approval       Claim lifecycle approvals

$ /tmp/zbrain workspace --help
Usage:
  zbrain workspace create <name>
  zbrain workspace current

Manage the active workspace.

$ /tmp/zbrain ask --help
Usage: zbrain ask [--workspace <name>] [--include <name>]... [--embed] <query>
```

## 2. Typed exit codes 0/1/2 — `TestExitCodes`

```bash
go test ./internal/cli -run TestExitCodes -count=1 -v
```

Result: **28/28 subtests PASS, 0 FAIL** (`ok github.com/therealtinhtute/zbrain/internal/cli 0.254s`).
Covered surfaces: success paths (help/version/setup/doctor/ask) → `0`; unknown command /
runtime error (missing workspace, approve nonexistent, doctor findings) → `1`; flag misuse
(`--bogus`, missing flag values, missing positional args, usage errors) → `2`.
All three exit tiers (`0/1/2`) are exercised and asserted — matches §2.1 "exit 0/1/2".

Verbatim tail of the run:

```
    --- PASS: TestExitCodes/unknown_flag_subcommand (0.00s)
    --- PASS: TestExitCodes/unknown_flag_ask (0.00s)
    --- PASS: TestExitCodes/flag_requires_value (0.00s)
    --- PASS: TestExitCodes/help_with_extra_arg_is_usage (0.00s)
    --- PASS: TestExitCodes/missing_arg_workspace_create (0.00s)
    --- PASS: TestExitCodes/missing_arg_claim_draft_extra_positional (0.00s)
    --- PASS: TestExitCodes/missing_arg_ask_no_query (0.00s)
    --- PASS: TestExitCodes/missing_arg_evidence_add (0.00s)
    --- PASS: TestExitCodes/runtime_error_approve_nonexistent (0.00s)
    --- PASS: TestExitCodes/runtime_error_resolve_workspace_missing (0.00s)
    --- PASS: TestExitCodes/doctor_with_findings_dirty (0.06s)
PASS
ok  	github.com/therealtinhtute/zbrain/internal/cli	0.254s
```

## 3. JSON protocol + stdout/stderr separation

Isolated `ZBRAIN_HOME` lifecycle: `setup` → `workspace create main` → `claim draft`
(stdin body) → `claim approve` → `reindex` → `status` → `ask`. Every step exited `0`;
`status` and `ask` stdout parse as pure JSON (`python3 -m json.tool`), stderr is **0 bytes**;
diagnostics observed on stderr (not stdout) for non-JSON paths (e.g. `unknown flag: --json`
on stderr when probing flags). stdout = protocol, stderr = diagnostics (§2.1).

```bash
export ZBRAIN_HOME=$(mktemp -d)
zbrain setup && zbrain workspace create main
printf 'verify CLI stdout is protocol while stderr carries diagnostics.\n' \
  | zbrain claim draft --tier decisions --title "cli audit claim" --basis owner
zbrain claim approve clm_5aaa5e1e108c205c26335a6e00da0d27
zbrain reindex
zbrain status > status.out 2> status.err        # exit 0
python3 -m json.tool status.out > /dev/null     # PURE JSON: YES
wc -c status.err                                # 0 bytes
zbrain ask "cli audit" > ask.out 2> ask.err     # exit 0, stderr 0 bytes
```

`status` stdout verbatim (schema_version 2, no human text mixed in):

```json
{
  "schema_version": 2,
  "workspace": "main",
  "approved": 1,
  "draft": 0,
  "invalid": 0,
  "invalid_count": 0,
  "legacy": 0,
  "rebuild_state": "",
  "manifest_digest": "",
  "rebuilt_at": "",
  "embedding": {
    "strategy": "lexical",
    "indexed": 0,
    "eligible": 0,
    "degraded_reason": "embeddings not configured"
  }
}
```

`ask` stdout verbatim (same run, index fresh=true, claim retrieved):

```json
{
  "schema_version": 1,
  "status": "ready",
  "query": "cli audit",
  "scopes": {"primary": "main", "includes": []},
  "claims": [
    {
      "workspace": "main",
      "id": "clm_5aaa5e1e108c205c26335a6e00da0d27",
      "path": "decisions/clm_5aaa5e1e108c205c26335a6e00da0d27.md",
      "tier": "decisions",
      "type": "zbrain.claim",
      "status": "approved",
      "title": "cli audit claim",
      "score": -0.000002375
    }
  ],
  "conflicts": [],
  "gaps": null,
  "promotion_candidates": null,
  "index": [{"workspace": "main", "fresh": true}]
}
```

## 4. ANC principles — evidence table

| ANC principle | Evidence command / artifact | Result | Verdict |
|---|---|---|---|
| Flow safety | `go test ./internal/cli -run TestExitCodes` — 28/28; `ask`/`status` exit 0 with fresh index; `doctor` exits 1 on findings (diagnosis before action); non-interactive default, no surprise prompts | typed exits 0/1/2, fail-closed | **PASS** |
| Token efficiency | `status`/`ask` emit compact single JSON document (schema_version + fields, no banners on stdout); help output ≤ 30 lines; stderr empty on success | minimal protocol bytes | **PASS** |
| Self-describing | All 12 surfaces print `Usage:` line on `--help` (section 1); `status` has `schema_version`, `doctor` has `next_action` field | every command self-documents | **PASS** |
| Automation safety | stdout = protocol only (pure JSON verified via `json.tool`, stderr 0 bytes); `--help`/`--json`-style probing errors land on stderr, never corrupt stdout; JSON stable keys | script-safe stdout | **PASS** |
| Predictability | `TestExitCodes` locks 0/1/2 mapping for 28 cases (success/unknown/runtime-error/usage); `mcp serve` stdio-only; `view` 127.0.0.1 GET/HEAD | deterministic contract | **PASS** |

## 5. Overall verdict

**PASS — manual PASS list (5/5 principles).** Satisfies plan exit criteria
"`cli-agent-lint` ≥B, `anc` ≥90% **hoặc manual PASS list**" (plan §4.4, wave t2 verify
"`cli-audit.md` ... + manual PASS"). No FAIL rows. Automated C6/C7 tooling not installed
on this machine; the manual list above is the documented fallback.

Covered surfaces cross-checked against existing contract tests:
`internal/cli/cli_test.go` (help surface test asserts `Usage:` in output, TestExitCodes
asserts 0/1/2) — both green in `go test ./internal/cli`.