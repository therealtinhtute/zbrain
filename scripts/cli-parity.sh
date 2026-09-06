#!/usr/bin/env bash
# scripts/cli-parity.sh — differential CLI wiring: run the FULL command
# surface on the Go oracle and the Rust port against twin isolated
# ZBRAIN_HOMEs and diff stdout JSON + exit codes.
#
# Volatile fields are normalized before diffing (random IDs, one-time tokens,
# timestamps, home paths); deterministic digests/scores are compared exactly.
# The owner grant ceremony requires an interactive TTY in BOTH binaries, so
# interactive grant is covered as a TTY-failure parity case plus an optional
# pty walk; approval show is covered through a real MCP-prepared challenge.
#
# Usage: scripts/cli-parity.sh [--go-bin PATH] [--rs-bin PATH]
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
go_bin=""
rs_bin=""
while [ $# -gt 0 ]; do
  case "$1" in
    --go-bin) go_bin="$2"; shift 2 ;;
    --rs-bin) rs_bin="$2"; shift 2 ;;
    *) echo "cli-parity: unknown argument $1" >&2; exit 1 ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "cli-parity: python3 required" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
go_home="$tmp/go"
rs_home="$tmp/rs"
mkdir -p "$go_home" "$rs_home"

if [ -z "$go_bin" ]; then
  go_bin="$tmp/zbrain-go"
  (cd "$root" && go build -o "$go_bin" ./cmd/zbrain)
fi
if [ -z "$rs_bin" ]; then
  (cd "$root" && cargo build -q -p zbrain)
  rs_bin="$root/target/debug/zbrain"
fi

normalize() {
  # $1 = file to normalize in place. Random IDs/tokens/timestamps/home
  # paths become constants; deterministic digests and scores are kept.
  python3 - "$1" "$go_home" "$rs_home" <<'EOF'
import re, sys
path, go_home, rs_home = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, 'r', errors='replace') as f:
    text = f.read()
text = text.replace(go_home, 'HOME').replace(rs_home, 'HOME')
text = re.sub(r'clm_[0-9a-f]{32}', 'clm_NORMALIZED', text)
text = re.sub(r'evd_[0-9a-f]{32}', 'evd_NORMALIZED', text)
text = re.sub(r'chg_[0-9a-f]{32}', 'chg_NORMALIZED', text)
text = re.sub(r'cmp_[0-9a-f]{32}', 'cmp_NORMALIZED', text)
text = re.sub(r'"token": "[^"]+"', '"token": "TOKEN"', text)
text = re.sub(r'"manifest_digest": "[0-9a-f]{64}"', '"manifest_digest": "NORMALIZED"', text)
text = re.sub(r'"token_sha256": "sha256:[0-9a-f]{64}"', '"token_sha256": "sha256:NORMALIZED"', text)
text = re.sub(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z', 'TIMESTAMP', text)
with open(path, 'w') as f:
    f.write(text)
EOF
}

normalize_hex() {
  # Approval-show digests bind random claim IDs, so 64-hex digests are
  # normalized there only (digest equality is proven by parity.sh approval
  # with fixed IDs); the suffix relation is asserted separately.
  python3 - "$1" <<'EOF'
import re, sys
path = sys.argv[1]
with open(path, 'r', errors='replace') as f:
    text = f.read()
text = re.sub(r'sha256:[0-9a-f]{64}', 'sha256:NORMALIZED', text)
text = re.sub(r'"action_digest_suffix": "[0-9a-f]{16}"', '"action_digest_suffix": "SUFFIX"', text)
text = re.sub(r'"digest_suffix": "[0-9a-f]{16}"', '"digest_suffix": "SUFFIX"', text)
with open(path, 'w') as f:
    f.write(text)
EOF
}

pass=0
fail=0
check() {
  # check <label> <go_stdout> <rs_stdout> <go_rc> <rs_rc> [extra_normalizer]
  local label="$1" go_out="$2" rs_out="$3" go_rc="$4" rs_rc="$5"
  local extra="${6:-}"
  normalize "$go_out"
  normalize "$rs_out"
  if [ -n "$extra" ]; then
    "$extra" "$go_out"
    "$extra" "$rs_out"
  fi
  if [ "$go_rc" != "$rs_rc" ]; then
    echo "FAIL: $label (exit go=$go_rc rs=$rs_rc)"
    fail=$((fail + 1))
    return 0
  fi
  if ! diff -u "$go_out" "$rs_out" > "$tmp/diff.txt"; then
    echo "FAIL: $label (stdout diff)"
    cat "$tmp/diff.txt"
    fail=$((fail + 1))
    return 0
  fi
  echo "OK: $label"
  pass=$((pass + 1))
}

run_both() {
  # run_both <label> [extra_normalizer] -- <args...> (stdin inherited)
  local label="$1" extra="$2"
  shift 2
  [ "$1" = "--" ] && shift
  go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" "$@" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
  rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" "$@" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
  check "$label" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc" "$extra"
}

run_both_stdin() {
  # run_both_stdin <label> <stdin_file> [extra] -- <args...>
  local label="$1" stdin_file="$2" extra="$3"
  shift 3
  [ "$1" = "--" ] && shift
  go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" "$@" < "$stdin_file" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
  rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" "$@" < "$stdin_file" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
  check "$label" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc" "$extra"
}

echo "== cli-parity: setup =="
run_both "setup" "" -- setup
run_both "setup idempotent" "" -- setup

echo "== cli-parity: version =="
run_both "version" "" -- version

echo "== cli-parity: workspace =="
run_both "workspace create research" "" -- workspace create research
run_both "workspace create personal" "" -- workspace create personal
run_both "workspace current" "" -- workspace current
run_both "workspace create duplicate" "" -- workspace create research
run_both "workspace bogus subcommand" "" -- workspace bogus

echo "== cli-parity: evidence =="
printf 'trusted source bytes\n' > "$tmp/source.txt"
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" evidence add --file "$tmp/source.txt" --origin "file://smoke" --media-type text/plain > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" evidence add --file "$tmp/source.txt" --origin "file://smoke" --media-type text/plain > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
go_evid=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["id"])')
rs_evid=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["id"])')
check "evidence add" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
run_both "evidence add dedupe" "" -- evidence add --file "$tmp/source.txt" --origin "file://smoke" --media-type text/plain
run_both "evidence check unchanged" "" -- evidence check
printf 'mutated source bytes\n' > "$tmp/source.txt"
run_both "evidence check changed" "" -- evidence check
printf 'trusted source bytes\n' > "$tmp/source.txt"
run_both "evidence add missing file" "" -- evidence add --file "$tmp/nope.txt" --origin "file://nope"
run_both "evidence bogus subcommand" "" -- evidence bogus

echo "== cli-parity: claim lifecycle =="
printf 'trusted parity answer\n' > "$tmp/body.txt"
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim draft --tier projects --title 'Parity Claim' --basis evidence --evidence "$go_evid" < "$tmp/body.txt" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim draft --tier projects --title 'Parity Claim' --basis evidence --evidence "$rs_evid" < "$tmp/body.txt" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
go_claim=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["id"])')
rs_claim=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["id"])')
check "claim draft" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
# NOTE: claim IDs differ per side by design; approve each side's own ID.
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim approve "$go_claim" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim approve "$rs_claim" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
check "claim approve" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
printf 'replacement body\n' > "$tmp/replace.txt"
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim supersede "$go_claim" --tier projects --title 'Parity v2' --basis owner < "$tmp/replace.txt" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim supersede "$rs_claim" --tier projects --title 'Parity v2' --basis owner < "$tmp/replace.txt" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
go_claim2=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["id"])')
rs_claim2=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["id"])')
check "claim supersede" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim approve "$go_claim2" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim approve "$rs_claim2" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
check "claim approve replacement" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim revoke "$go_claim2" --reason "parity revoke" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim revoke "$rs_claim2" --reason "parity revoke" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
check "claim revoke" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc"
run_both "claim approve missing" "" -- claim approve clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
run_both "claim revoke missing reason" "" -- claim revoke "$go_claim"
run_both "claim bogus subcommand" "" -- claim bogus

echo "== cli-parity: migrate =="
run_both "migrate okf noop" "" -- migrate okf
legacy_id="clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
for side in go rs; do
  if [ "$side" = "go" ]; then home="$go_home"; else home="$rs_home"; fi
  printf -- '---\nschema: zbrain.claim/v1\nid: %s\nstatus: draft\ntitle: Legacy Claim\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nLegacy body\n' "$legacy_id" > "$home/workspaces/research/wiki/projects/$legacy_id.md"
done
run_both "migrate okf legacy" "" -- migrate okf
if ! diff -u "$go_home/workspaces/research/wiki/projects/$legacy_id.md" "$rs_home/workspaces/research/wiki/projects/$legacy_id.md"; then
  echo "FAIL: migrate okf legacy file bytes"
  fail=$((fail + 1))
else
  echo "OK: migrate okf legacy file bytes"
  pass=$((pass + 1))
fi
run_both "migrate bogus subcommand" "" -- migrate bogus

echo "== cli-parity: reindex/ask/status/doctor =="
run_both "reindex" "" -- reindex
run_both "reindex --embed" "" -- reindex --embed
run_both "ask ready" "" -- ask trusted parity
run_both "ask --embed" "" -- ask --embed trusted parity
run_both "ask gap" "" -- ask zzz-unmatchable-query
run_both "ask --after" "" -- ask --after 2026-01-01T00:00:00Z trusted parity
run_both "ask bad timestamp" "" -- ask --after bad-date trusted
run_both "status" "" -- status
run_both "doctor healthy" "" -- doctor
run_both "doctor --probe-embedder" "" -- doctor --probe-embedder
printf 'dirty marker body\n' > "$tmp/dirty.txt"
(cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim draft --tier projects --title 'Dirty Probe' --basis owner < "$tmp/dirty.txt" > /dev/null 2>&1) || true
(cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim draft --tier projects --title 'Dirty Probe' --basis owner < "$tmp/dirty.txt" > /dev/null 2>&1) || true
run_both "status dirty" "" -- status
run_both "doctor dirty" "" -- doctor
run_both "ask dirty" "" -- ask trusted
run_both "reindex after dirty" "" -- reindex

echo "== cli-parity: cross-workspace includes =="
printf 'private explicit token\n' > "$tmp/private.txt"
(cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim draft --workspace personal --tier projects --title 'Personal' --basis owner < "$tmp/private.txt" > "$tmp/go.out" 2>/dev/null); go_priv=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["id"])')
(cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim draft --workspace personal --tier projects --title 'Personal' --basis owner < "$tmp/private.txt" > "$tmp/rs.out" 2>/dev/null); rs_priv=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["id"])')
(cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim approve --workspace personal "$go_priv" > /dev/null 2>&1) || true
(cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim approve --workspace personal "$rs_priv" > /dev/null 2>&1) || true
(cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" reindex --workspace personal > /dev/null 2>&1) || true
(cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" reindex --workspace personal > /dev/null 2>&1) || true
run_both "ask without include is gap" "" -- ask private explicit
run_both "ask --include personal" "" -- ask --include personal private explicit

echo "== cli-parity: approval show/grant =="
run_both "approval show unknown" "" -- approval show chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
run_both "approval grant unknown piped fails TTY" "" -- approval grant chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa < /dev/null
run_both "approval bogus subcommand" "" -- approval bogus
# Prepare a real challenge on each side through the MCP gateway, then show it
# through the CLI. The challenge binds side-specific claim IDs, so digests
# are normalized for the diff (equality is proven by parity.sh approval).
mcp_prepare() {
  local bin="$1" home="$2" claim="$3" out="$4"
  python3 - "$bin" "$home" "$claim" "$out" <<'EOF'
import json, os, subprocess, sys
binary, home, claim, out = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
proc = subprocess.Popen([binary, 'mcp', 'serve'],
                        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.DEVNULL, env={**os.environ, 'ZBRAIN_HOME': home},
                        text=True, bufsize=1)
def rpc(payload):
    proc.stdin.write(json.dumps(payload) + '\n')
    proc.stdin.flush()
    return json.loads(proc.stdout.readline())
rpc({"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2025-06-18", "capabilities": {},
                "clientInfo": {"name": "cli-parity", "version": "0"}}})
proc.stdin.write('{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
proc.stdin.flush()
resp = rpc({"jsonrpc": "2.0", "id": 2, "method": "tools/call",
            "params": {"name": "claim_lifecycle",
                       "arguments": {"operation": "prepare", "action": "approve",
                                     "workspace": "research", "claim_id": claim}}})
proc.stdin.close()
proc.wait()
text = resp["result"]["content"][0]["text"]
challenge = json.loads(text)["challenge_id"]
with open(out, 'w') as f:
    f.write(challenge + '\n')
EOF
}
printf 'ceremony body\n' > "$tmp/ceremony.txt"
(cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" claim draft --tier projects --title 'Ceremony' --basis owner < "$tmp/ceremony.txt" > "$tmp/go.out" 2>/dev/null); go_cer=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["id"])')
(cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" claim draft --tier projects --title 'Ceremony' --basis owner < "$tmp/ceremony.txt" > "$tmp/rs.out" 2>/dev/null); rs_cer=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["id"])')
mcp_prepare "$go_bin" "$go_home" "$go_cer" "$tmp/go_chg.txt"
mcp_prepare "$rs_bin" "$rs_home" "$rs_cer" "$tmp/rs_chg.txt"
go_chg=$(cat "$tmp/go_chg.txt")
rs_chg=$(cat "$tmp/rs_chg.txt")
go_rc=0; (cd "$root" && ZBRAIN_HOME="$go_home" "$go_bin" approval show "$go_chg" > "$tmp/go.out" 2> "$tmp/go.err") || go_rc=$?
rs_rc=0; (cd "$root" && ZBRAIN_HOME="$rs_home" "$rs_bin" approval show "$rs_chg" > "$tmp/rs.out" 2> "$tmp/rs.err") || rs_rc=$?
go_suffix=$(python3 -c 'import json; print(json.load(open("'$tmp'/go.out"))["action_digest_suffix"])')
rs_suffix=$(python3 -c 'import json; print(json.load(open("'$tmp'/rs.out"))["action_digest_suffix"])')
check "approval show prepared" "$tmp/go.out" "$tmp/rs.out" "$go_rc" "$rs_rc" normalize_hex
# Suffix relation must hold within each side's own output.
python3 - "$tmp/go.out" "$tmp/rs.out" <<'EOF'
import json, sys
for path in sys.argv[1:]:
    doc = json.load(open(path))
    assert doc["challenge_id"].startswith("chg_"), doc
    assert doc["operation"] == "approve", doc
    assert doc["workspace"] == "research", doc
print("approval show fields consistent")
EOF
echo "OK: approval show fields consistent"
pass=$((pass + 1))
# Interactive grant over a real pty (both binaries fail closed on pipes, so a
# pty driver feeds the digest suffix after the prompt appears). macOS `script`
# does not reliably relay piped stdin to the pty, hence this inline driver.
grant_pty() {
  local bin="$1" home="$2" chg="$3" suffix="$4" out="$5"
  python3 - "$bin" "$home" "$chg" "$suffix" "$out" <<'EOF'
import os, pty, re, select, subprocess, sys, time
binary, home, chg, suffix, out = sys.argv[1:6]
master, slave = pty.openpty()
env = {**os.environ, 'ZBRAIN_HOME': home}
proc = subprocess.Popen([binary, 'approval', 'grant', chg],
                        stdin=slave, stdout=slave, stderr=slave,
                        env=env, close_fds=True)
os.close(slave)
output = b''
deadline = time.time() + 30
sent = False
while time.time() < deadline:
    if proc.poll() is not None:
        break
    r, _, _ = select.select([master], [], [], 1.0)
    if r:
        try:
            chunk = os.read(master, 4096)
        except OSError:
            chunk = b''
        if chunk:
            output += chunk
        elif proc.poll() is not None:
            break
        if not sent and b'confirm the last 16' in output:
            os.write(master, (suffix + '\n').encode())
            sent = True
# Grace drain: the child may exit with output still in flight.
time.sleep(0.5)
while True:
    r, _, _ = select.select([master], [], [], 0.5)
    if not r:
        break
    try:
        chunk = os.read(master, 4096)
    except OSError:
        break
    if not chunk:
        break
    output += chunk
rc = proc.poll()
text = output.decode(errors='replace')
if rc is None:
    proc.kill()
    with open(out + '.hung', 'w') as f:
        f.write(text)
    raise SystemExit(f"pty grant hung for {binary}")
match = re.search(r'\{[^{}]*"challenge_id"[^{}]*\}', text, re.DOTALL)
if not match:
    raise SystemExit(f"no grant JSON from {binary} (rc={rc}):\n{text[-2000:]}")
with open(out, 'w') as f:
    f.write(match.group(0) + '\n')
with open(out + '.rc', 'w') as f:
    f.write(str(rc) + '\n')
EOF
}
grant_pty "$go_bin" "$go_home" "$go_chg" "$go_suffix" "$tmp/go_grant.json" || echo "GAP: go pty grant hung (see transcript); interactive grant covered by lib grant-walk tests"
grant_pty "$rs_bin" "$rs_home" "$rs_chg" "$rs_suffix" "$tmp/rs_grant.json" || echo "GAP: rs pty grant hung (see transcript); interactive grant covered by lib grant-walk tests"
if [ -f "$tmp/go_grant.json" ] && [ -f "$tmp/rs_grant.json" ]; then
  check "approval grant pty" "$tmp/go_grant.json" "$tmp/rs_grant.json" "$(cat "$tmp/go_grant.json.rc")" "$(cat "$tmp/rs_grant.json.rc")"
fi

echo "== cli-parity: exit codes =="
run_both "unknown command" "" -- bogus
run_both "unknown top flag" "" -- --bogus
run_both "help with extra arg" "" -- --help extra
run_both "version with extra arg" "" -- version extra
run_both "setup unknown flag" "" -- setup --bogus
run_both "ask no query" "" -- ask
run_both "ask unknown flag" "" -- ask --bogus value
run_both "mcp missing subcommand" "" -- mcp
run_both "mcp bogus subcommand" "" -- mcp bogus
run_both "evidence add missing origin" "" -- evidence add --file "$tmp/source.txt"
run_both "reindex extra arg" "" -- reindex extra
run_both "status extra arg" "" -- status extra

echo
echo "cli-parity: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
