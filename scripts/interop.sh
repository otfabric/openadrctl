#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
#
# Run openadrctl against the OpenLEADR VTN of openadr-interop, as Business
# Logic and as a VEN: objects, reports, WebSocket and MQTT notifications,
# events watch, and errors. Prints one line per check and exits 1 when one
# fails.
#
#   scripts/interop.sh [tag]      # default tag: v0.4.0
#
# Needs Docker and Go. The VTN and its broker are published on loopback only
# (ports 18090 and 18883, or OPENADR_INTEROP_PORT and OPENADR_INTEROP_MQTT_PORT):
# the image's client secrets are public.
set -euo pipefail

TAG="${1:-v0.4.0}"
IMAGE="ghcr.io/otfabric/openadr-interop-openleadr:${TAG}"
PORT="${OPENADR_INTEROP_PORT:-18090}"
MQTT_PORT="${OPENADR_INTEROP_MQTT_PORT:-18883}"
NAME="openadrctl-interop-$$"
WORK="$(mktemp -d)"
BIN="${WORK}/openadrctl"

cleanup() {
  docker rm -f "${NAME}" >/dev/null 2>&1 || true
  rm -rf "${WORK}"
}
trap cleanup EXIT

cd "$(dirname "$0")/.."
GOWORK=off go build -o "${BIN}" .

docker run -d --rm --name "${NAME}" -p "127.0.0.1:${PORT}:8080" -p "127.0.0.1:${MQTT_PORT}:1883" \
  "${IMAGE}" server --fixture /fixtures/baseline/fixture.json --public-url "http://127.0.0.1:${PORT}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${NAME}" test -f /run/openadr-interop/ready 2>/dev/null && break
  sleep 2
done
docker exec "${NAME}" test -f /run/openadr-interop/ready || { docker logs "${NAME}" | tail -20; echo "the VTN did not get ready"; exit 1; }

export OPENADRCTL_URL="http://127.0.0.1:${PORT}"
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
    printf 'PASS  %-40s exit %s\n' "${name}" "${got}"
  else
    printf 'FAIL  %-40s exit %s, want %s\n' "${name}" "${got}" "${want}"
    sed 's/^/      /' "${WORK}/err" | head -5
    failed=1
  fi
}

check "ping" 0 env OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret "${BIN}" ping
check "ping without credentials (401)" 6 "${BIN}" ping
check "bl programs list" 0 bl programs list
check "bl events list" 0 bl events list
check "bl vens list" 0 bl vens list
check "bl resources list" 0 bl resources list
check "bl notifiers" 0 bl notifiers
check "bl topics events" 0 bl topics events

PROGRAM="$(bl programs create --name openadrctl-interop --format json | field '["id"]')"
check "bl programs update" 0 bl programs update "${PROGRAM}" --description-url https://example.com/program
EVENT="$(bl events create --program "${PROGRAM}" --name openadrctl-event --type SIMPLE \
  --start 2030-01-01T00:00:00Z --interval PT1H --value 1 --yes --format json | field '["id"]')"
check "bl events get" 0 bl events get "${EVENT}"
check "bl events timeline" 0 bl events timeline "${EVENT}"
check "bl events create without --yes" 2 bl events create --program "${PROGRAM}" --type SIMPLE --value 1

check "ven programs list" 0 ven programs list
check "ven events list --target group-a" 0 ven events list --target group-a
check "ven vens list" 0 ven vens list
REPORT="$(ven reports create --event "${EVENT}" --client-name ven-1 --type USAGE --value 1.5 --format json | field '["id"]')"
check "ven reports get" 0 ven reports get "${REPORT}"
check "bl reports list --event" 0 bl reports list --event "${EVENT}"
check "a VEN's token on bl programs create (403)" 6 env OPENADRCTL_CLIENT_ID=interop-ven-1 \
  OPENADRCTL_CLIENT_SECRET=interop-ven-1-secret "${BIN}" bl programs create --name not-mine
check "ven events watch" 0 ven events watch --interval 1s --count 1

# WebSocket: a program created while Business Logic listens.
(sleep 3; bl programs create --name openadrctl-ws --format json >"${WORK}/ws.json") &
check "bl listen --via websocket" 0 bl listen --via websocket --object PROGRAM --count 1 --duration 30s
wait
grep -q openadrctl-ws "${WORK}/out" || { echo "FAIL  websocket: no notification of the program"; failed=1; }
bl programs delete "$(field '["id"]' <"${WORK}/ws.json")" --yes >/dev/null

# MQTT: the broker of the image takes the client secret as the password.
(sleep 3; bl programs create --name openadrctl-mqtt --format json >"${WORK}/mqtt.json") &
check "bl listen --via mqtt" 0 env OPENADRCTL_MQTT_PASSWORD=interop-bl-secret \
  OPENADRCTL_CLIENT_ID=interop-bl OPENADRCTL_CLIENT_SECRET=interop-bl-secret "${BIN}" bl listen --via mqtt \
  --broker "mqtt://127.0.0.1:${MQTT_PORT}" --mqtt-username interop-bl --topics programs --count 1 --duration 30s
wait
grep -q openadrctl-mqtt "${WORK}/out" || { echo "FAIL  mqtt: no notification of the program"; failed=1; }
bl programs delete "$(field '["id"]' <"${WORK}/mqtt.json")" --yes >/dev/null

# OpenLEADR keeps an event with reports (409): the report goes first.
check "ven reports delete" 0 ven reports delete "${REPORT}" --yes
check "bl events delete" 0 bl events delete "${EVENT}" --yes
check "bl programs delete" 0 bl programs delete "${PROGRAM}" --yes
check "bl programs get (404)" 6 bl programs get "${PROGRAM}"

if [[ "${failed}" != 0 ]]; then
  echo "openadrctl against OpenLEADR (${TAG}): FAILED"
  exit 1
fi
echo "openadrctl against OpenLEADR (${TAG}): passed"
