#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
#
# MQTT with the clients' OAuth2 tokens, on a broker that checks them: EMQX,
# whose HTTP authenticator asks POST /auth/mqtt of openadrctl vtn start.
# The VTN publishes as itself (--mqtt-username, OPENADRCTL_VTN_MQTT_PASSWORD);
# Business Logic and a VEN listen with their tokens; a password that is not
# a token is refused. Prints one line per check and exits 1 when one fails.
#
#   scripts/mqtt-token.sh
#
# Needs Docker and Go. Ports on loopback: 18080 (VTN; it listens on all
# addresses so that the container reaches it) and 11883 (EMQX).
set -euo pipefail

# EMQX 6.3.1, the image go-openadr's interop tests pin.
IMAGE="emqx/emqx@sha256:5ecbf93d04e34aaf4096af074e93b773d0838c8ea668cfabdec7976b272d9606"
NAME="openadrctl-emqx-$$"
WORK="$(mktemp -d)"
BIN="${WORK}/openadrctl"
VTN_PID=""

cleanup() {
  docker rm -f "${NAME}" >/dev/null 2>&1 || true
  [[ -n "${VTN_PID}" ]] && kill "${VTN_PID}" 2>/dev/null || true
  rm -rf "${WORK}"
}
trap cleanup EXIT

cd "$(dirname "$0")/.."
GOWORK=off go build -o "${BIN}" .

# The VTN first, so that the broker's authenticator finds it.
OPENADRCTL_VTN_MQTT_PASSWORD=vtn-secret "${BIN}" vtn start --listen 0.0.0.0:18080 \
  --public-url http://127.0.0.1:18080 --mqtt mqtt://127.0.0.1:11883 \
  --mqtt-client-auth token --mqtt-username vtn >"${WORK}/vtn.log" 2>&1 &
VTN_PID=$!

docker run -d --rm --name "${NAME}" --add-host=host.docker.internal:host-gateway -p 127.0.0.1:11883:1883 \
  -e EMQX_AUTHENTICATION__1__MECHANISM=password_based \
  -e EMQX_AUTHENTICATION__1__BACKEND=http \
  -e EMQX_AUTHENTICATION__1__METHOD=post \
  -e EMQX_AUTHENTICATION__1__URL=http://host.docker.internal:18080/auth/mqtt \
  -e 'EMQX_AUTHENTICATION__1__BODY={"username":"${username}","password":"${password}"}' \
  -e 'EMQX_AUTHENTICATION__1__HEADERS={"content-type":"application/json"}' \
  "${IMAGE}" >/dev/null

export OPENADRCTL_URL=http://127.0.0.1:18080
bl() { OPENADRCTL_CLIENT_ID=utility OPENADRCTL_CLIENT_SECRET=utility-secret "${BIN}" "$@"; }
ven() { OPENADRCTL_CLIENT_ID=site-42 OPENADRCTL_CLIENT_SECRET=site-42-secret "${BIN}" "$@"; }

# Until EMQX runs and its authenticator answers: a login with a token.
ready=""
for _ in $(seq 1 60); do
  if bl bl listen --via mqtt --topics events --duration 1s --timeout 3s >/dev/null 2>&1; then
    ready=yes
    break
  fi
  sleep 2
done
[[ -n "${ready}" ]] || { docker logs "${NAME}" | tail -20; echo "EMQX did not let a token in"; exit 1; }

failed=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() {
  printf 'FAIL  %s\n' "$1"
  failed=1
}

# listen_for <name> <event name> <who> <listen args...>: an event created
# while <who> listens arrives.
listen_for() {
  local name="$1" event="$2" who="$3"
  shift 3
  (sleep 3; bl bl events create --program dynamic-price --name "${event}" --type PRICE --value 0.3 --yes >/dev/null) &
  local creator=$!
  if "${who}" "$@" --count 1 --duration 30s >"${WORK}/out" 2>"${WORK}/err" && grep -q "${event}" "${WORK}/out"; then
    pass "${name}"
  else
    fail "${name}"
    sed 's/^/      /' "${WORK}/err" | head -5
  fi
  wait "${creator}"
}

if bl bl notifiers | grep -q OAUTH2_BEARER_TOKEN; then pass "notifiers announce OAUTH2_BEARER_TOKEN"; else fail "notifiers announce OAUTH2_BEARER_TOKEN"; fi
listen_for "bl listen --via mqtt with its token" mqtt-token-bl bl bl listen --via mqtt --topics events
listen_for "ven listen --via mqtt with its token" mqtt-token-ven ven ven listen --via mqtt --topics ven-events:site-42
code=0
OPENADRCTL_MQTT_PASSWORD=guess ven ven listen --via mqtt --topics ven-events:site-42 --timeout 5s >/dev/null 2>&1 || code=$?
if [[ "${code}" == 4 ]]; then pass "a password that is not a token is refused (exit 4)"; else fail "a password that is not a token: exit ${code}, want 4"; fi
if grep -q "mqtt_login     vtn allowed" "${WORK}/vtn.log"; then pass "the VTN's publisher logs in as itself"; else fail "the VTN's publisher logs in as itself"; fi

if [[ "${failed}" != 0 ]]; then
  echo "MQTT with tokens on EMQX: FAILED"
  sed 's/^/vtn: /' "${WORK}/vtn.log" | tail -20
  exit 1
fi
echo "MQTT with tokens on EMQX: passed"
