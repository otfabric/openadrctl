#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
#
# Run openadrctl with --spec 3.0.1 against the OpenLEADR v0.1.4 VTN of
# openadr-interop (adapter openleadr301), an OpenADR 3.0.1 VTN, as Business
# Logic and as a VEN: typed targets, resources through their VEN, reports
# that name their program, events watch, and what 3.0.1 does not have.
# Prints one line per check and exits 1 when one fails.
#
#   scripts/interop301.sh [image]  # default: ghcr.io/otfabric/openadr-interop-openleadr301:v0.5.0
#
# Needs Docker, Go and curl. The VTN is published on loopback only (port 18091, or
# OPENADR_INTEROP_PORT): the image's client secrets are public.
set -euo pipefail

IMAGE="${1:-ghcr.io/otfabric/openadr-interop-openleadr301:v0.5.0}"
PORT="${OPENADR_INTEROP_PORT:-18091}"
NAME="openadrctl-interop301-$$"
WORK="$(mktemp -d)"
BIN="${WORK}/openadrctl"

cleanup() {
  docker rm -f "${NAME}" >/dev/null 2>&1 || true
  rm -rf "${WORK}"
}
trap cleanup EXIT

cd "$(dirname "$0")/.."
GOWORK=off go build -o "${BIN}" .

docker run -d --rm --name "${NAME}" -p "127.0.0.1:${PORT}:8080" \
  "${IMAGE}" server --fixture /fixtures/baseline-301/fixture.json --public-url "http://127.0.0.1:${PORT}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${NAME}" test -f /run/openadr-interop/ready 2>/dev/null && break
  sleep 2
done
docker exec "${NAME}" test -f /run/openadr-interop/ready || { docker logs "${NAME}" | tail -20; echo "the VTN did not get ready"; exit 1; }

export OPENADRCTL_URL="http://127.0.0.1:${PORT}" OPENADRCTL_SPEC=3.0.1
bl() { OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret "${BIN}" bl "$@"; }
ven() { OPENADRCTL_CLIENT_ID=interop-ven-1 OPENADRCTL_CLIENT_SECRET=interop-ven-1-secret "${BIN}" ven "$@"; }
field() { python3 -c "import json,sys; print(json.load(sys.stdin)$1)"; }

failed=0
# check <name> <expected exit code> <command...>
check() {
  local name="$1" want="$2"
  shift 2
  local got=0
  "$@" >"${WORK}/out" 2>"${WORK}/err" || got=$?
  if [[ "${got}" == "${want}" ]]; then
    printf 'PASS  %-46s exit %s\n' "${name}" "${got}"
  else
    printf 'FAIL  %-46s exit %s, want %s\n' "${name}" "${got}" "${want}"
    sed 's/^/      /' "${WORK}/err" | head -5
    failed=1
  fi
}
# has <name> <text>: the output of the last check holds text.
has() {
  if grep -q -- "$2" "${WORK}/out"; then
    printf 'PASS  %-46s\n' "$1"
  else
    printf 'FAIL  %-46s no %s in the output\n' "$1" "$2"
    failed=1
  fi
}

check "ping" 0 env OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret "${BIN}" ping
has "ping: token from /auth/token" "/auth/token"
has "ping: no notifiers in 3.0.1" "not in OpenADR 3.0.1"
check "ping without credentials (401)" 6 "${BIN}" ping
check "bl programs list" 0 bl programs list
has "bl programs list: typed targets" "VEN_NAME=site-1"
check "bl programs list --target site-1 (no type)" 3 bl programs list --target site-1
check "bl events list" 0 bl events list
check "bl vens list" 0 bl vens list
VEN="$(bl vens list --name site-1 --format json | field '["vens"][0]["id"]')"
check "bl resources list --ven" 0 bl resources list --ven "${VEN}"
has "bl resources list --ven: battery" "battery"
check "bl resources list without --ven" 3 bl resources list
check "bl notifiers (not in 3.0.1)" 3 bl notifiers
check "bl resource-groups list (not in 3.0.1)" 3 bl resource-groups list

# OpenLEADR v0.1.4 keeps VEN_NAME targets apart from the program (they say
# who sees it), and searches the others: the program has one of each.
PROGRAM="$(bl programs create --name openadrctl-301 --target VEN_NAME=site-1 --target GROUP=group-a --format json | field '["id"]')"
check "bl programs list --target GROUP=group-a" 0 bl programs list --target GROUP=group-a
has "the search finds the program" "openadrctl-301"
check "bl programs list --target GROUP=group-b" 0 bl programs list --target GROUP=group-b
if grep -q openadrctl-301 "${WORK}/out"; then echo "FAIL  the search for group-b finds the program"; failed=1; fi
check "bl programs get" 0 bl programs get "${PROGRAM}" --format json
cp "${WORK}/out" "${WORK}/program.json"
check "validate --spec 3.0.1 of what get wrote" 0 "${BIN}" validate program "${WORK}/program.json"
check "bl programs update" 0 bl programs update "${PROGRAM}" --description-url https://example.com/program
EVENT="$(bl events create --program "${PROGRAM}" --name openadrctl-event --type SIMPLE \
  --start 2030-01-01T00:00:00Z --interval PT1H --value 1 --yes --format json | field '["id"]')"
check "bl events get" 0 bl events get "${EVENT}"
check "bl events timeline" 0 bl events timeline "${EVENT}"

check "ven programs list" 0 ven programs list
has "ven-1 sees the program targeted at it" "openadrctl-301"
check "ven events list" 0 ven events list
REPORT="$(ven reports create --event "${EVENT}" --client-name ven-1 --type USAGE --value 1.5 --format json | field '["id"]')"
check "ven reports get" 0 ven reports get "${REPORT}" --format json
has "the report names its program" "\"programID\": \"${PROGRAM}\""
check "bl reports list --event" 0 bl reports list --event "${EVENT}"
check "a VEN's token on bl programs create (403)" 6 env OPENADRCTL_CLIENT_ID=interop-ven-1 \
  OPENADRCTL_CLIENT_SECRET=interop-ven-1-secret "${BIN}" bl programs create --name not-mine
check "ven events watch" 0 ven events watch --interval 1s --count 1

RESOURCE="$(bl resources create --ven "${VEN}" --name openadrctl-meter --format json | field '["id"]')"
check "bl resources get --ven" 0 bl resources get "${RESOURCE}" --ven "${VEN}"
check "bl resources update --ven" 0 bl resources update "${RESOURCE}" --ven "${VEN}" --attribute MAX_POWER_CONSUMPTION=11
check "bl resources delete --ven" 0 bl resources delete "${RESOURCE}" --ven "${VEN}" --yes

# OpenLEADR v0.1.4 deletes a report only for a business user, against the
# specification, and openadrctl's Business Logic has no report delete: the
# VEN is refused, and the report goes with curl, before its event.
check "ven reports delete (403 from OpenLEADR v0.1.4)" 6 ven reports delete "${REPORT}" --yes
BL_TOKEN="$(OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret "${BIN}" token)"
curl -sf -X DELETE -H "Authorization: Bearer ${BL_TOKEN}" "${OPENADRCTL_URL}/reports/${REPORT}" >/dev/null ||
  { echo "FAIL  deleting the report as Business Logic"; failed=1; }
check "bl events delete" 0 bl events delete "${EVENT}" --yes
check "bl programs delete" 0 bl programs delete "${PROGRAM}" --yes
check "bl programs get (404)" 6 bl programs get "${PROGRAM}"

if [[ "${failed}" != 0 ]]; then
  echo "openadrctl --spec 3.0.1 against OpenLEADR v0.1.4 (${IMAGE}): FAILED"
  exit 1
fi
echo "openadrctl --spec 3.0.1 against OpenLEADR v0.1.4 (${IMAGE}): passed"
