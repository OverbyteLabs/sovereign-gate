#!/usr/bin/env bash
# The full demo: local deploy, signed records, the gate on both cloud targets.
# Set ONPREM_HOST (user@server or "local") to include the on-premise step.
set -euo pipefail
cd "$(dirname "$0")/../.."

export GATE_KEY_DIR="${GATE_KEY_DIR:-$(mktemp -d)}"
gate=bin/gate
step() { printf '\n== %s\n' "$*"; }

# expect CODE CMD...: run CMD and fail the demo unless it exits with CODE.
expect() {
  local want=$1; shift
  local got=0
  "$@" || got=$?
  if [ "$got" != "$want" ]; then
    echo "expected exit $want, got $got: $*" >&2
    exit 1
  fi
}

go build -o "$gate" ./cmd/gate
[ -f "$GATE_KEY_DIR/gate.key" ] || $gate keygen
rm -rf .gate/records .gate/current

step "1. Deploy locally"
$gate deploy local -f example/score.yaml -build example/app
trap 'docker compose -f compose.yaml down -v >/dev/null 2>&1 || true' EXIT

step "2. Generate the on-premise Quadlet units"
$gate deploy onprem -f example/score.yaml -dry-run

if [ -n "${ONPREM_HOST:-}" ]; then
  step "3. Deploy on-premise to $ONPREM_HOST"
  $gate deploy onprem -f example/score.yaml -host "$ONPREM_HOST"

  step "4. Gate: US database is blocked"
  expect 1 $gate check -f example/score.yaml -target example/targets/cloud-us.yaml

  step "5. Gate: EU database is allowed"
  expect 0 $gate check -f example/score.yaml -target example/targets/cloud-eu.yaml
else
  step "3. Gate without an on-premise record: both targets are blocked"
  expect 1 $gate check -f example/score.yaml -target example/targets/cloud-us.yaml
  expect 1 $gate check -f example/score.yaml -target example/targets/cloud-eu.yaml
fi

step "Records verify"
$gate verify

step "A tampered record is rejected"
rec=$(ls .gate/records/*/local.json)
cp "$rec" "$rec.bak"
sed -i 's/"healthy": true/"healthy": false/' "$rec"
expect 2 $gate verify
mv "$rec.bak" "$rec"

echo
echo "demo passed"
