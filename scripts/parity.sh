#!/usr/bin/env bash
# scripts/parity.sh — differential parity: run the Go oracle (fixture-gen) and
# the Rust port (zbrain-parity) against fresh isolated homes and diff their
# manifests byte-for-byte.
#
# Usage: scripts/parity.sh [workspace-name] [op]
#   op: workspace | setup | claims | lifecycle   (defaults: research, workspace)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
workspace="${1:-research}"
op="${2:-workspace}"

if ! command -v go >/dev/null 2>&1; then
  echo "parity: go toolchain required" >&2
  exit 1
fi
if ! command -v cargo >/dev/null 2>&1; then
  echo "parity: cargo toolchain required" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

go_home="$tmp/go"
rs_home="$tmp/rs"

run_go() { (cd "$root" && go run ./crates/tools/fixture-gen "$@"); }
run_rs() { (cd "$root" && cargo run -q -p zbrain --bin parity -- "$@"); }
diff_or_fail() {
  local label="$1" left="$2" right="$3"
  if ! diff -u "$left" "$right" > "$tmp/diff.txt"; then
    echo "parity: DIFF ($label)" >&2
    cat "$tmp/diff.txt" >&2
    exit 1
  fi
}

case "$op" in
  workspace|setup)
    run_go --home "$go_home" --workspace "$workspace" --op "$op" > "$tmp/go.json"
    run_rs --home "$rs_home" --workspace "$workspace" --op "$op" > "$tmp/rs.json"
    diff_or_fail "op=$op workspace=$workspace" "$tmp/go.json" "$tmp/rs.json"
    ;;
  claims)
    # Build the same deterministic claim/evidence tree with both runtimes and
    # diff the manifests, then prove two-way tree readability: the Go oracle
    # verifies the Rust-written tree and the Rust port verifies the
    # Go-written tree; every manifest must be byte-identical.
    run_go --home "$go_home" --workspace "$workspace" --op claims > "$tmp/go.json"
    run_rs --home "$rs_home" --workspace "$workspace" --op claims > "$tmp/rs.json"
    diff_or_fail "op=claims workspace=$workspace" "$tmp/go.json" "$tmp/rs.json"

    run_go --home "$rs_home" --workspace "$workspace" --op claims-verify > "$tmp/go_of_rs.json"
    run_rs --home "$go_home" --workspace "$workspace" --op claims-verify > "$tmp/rs_of_go.json"
    diff_or_fail "op=claims-verify go-reads-rust" "$tmp/go_of_rs.json" "$tmp/go.json"
    diff_or_fail "op=claims-verify rust-reads-go" "$tmp/rs_of_go.json" "$tmp/go.json"
    ;;
  lifecycle)
    # Build the same draft -> approve -> supersede -> revoke chain with both
    # runtimes (including the pending-transition journal round trip on
    # supersede) and diff the lifecycle manifests; then prove two-way tree
    # readability of the resulting workspace.
    run_go --home "$go_home" --workspace "$workspace" --op lifecycle > "$tmp/go.json"
    run_rs --home "$rs_home" --workspace "$workspace" --op lifecycle > "$tmp/rs.json"
    diff_or_fail "op=lifecycle workspace=$workspace" "$tmp/go.json" "$tmp/rs.json"

    run_go --home "$rs_home" --workspace "$workspace" --op lifecycle-verify > "$tmp/go_of_rs.json"
    run_rs --home "$go_home" --workspace "$workspace" --op lifecycle-verify > "$tmp/rs_of_go.json"
    diff_or_fail "op=lifecycle-verify go-reads-rust" "$tmp/go_of_rs.json" "$tmp/go.json"
    diff_or_fail "op=lifecycle-verify rust-reads-go" "$tmp/rs_of_go.json" "$tmp/go.json"
    ;;
  *)
    echo "parity: unknown op $op" >&2
    exit 1
    ;;
esac

echo "parity: OK (op=$op workspace=$workspace)"
