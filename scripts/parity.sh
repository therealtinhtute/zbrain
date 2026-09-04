#!/usr/bin/env bash
# scripts/parity.sh — differential parity: run the Go oracle (fixture-gen) and
# the Rust port (zbrain-parity) against fresh isolated homes and diff their
# manifests byte-for-byte.
#
# Usage: scripts/parity.sh [workspace-name]   (default: research)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
workspace="${1:-research}"

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

(cd "$root" && go run ./crates/tools/fixture-gen --home "$go_home" --workspace "$workspace") > "$tmp/go.json"
(cd "$root" && cargo run -q -p zbrain --bin parity -- --home "$rs_home" --workspace "$workspace") > "$tmp/rs.json"

if diff -u "$tmp/go.json" "$tmp/rs.json" > "$tmp/diff.txt"; then
  echo "parity: OK (workspace=$workspace)"
else
  echo "parity: DIFF (workspace=$workspace)" >&2
  cat "$tmp/diff.txt" >&2
  exit 1
fi
