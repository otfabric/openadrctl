# openadrctl Releases

Releases are cut with the `Release` workflow (semantic versioning, annotated
tag, GitHub release with binaries for every platform). The section of the
version below, up to the `---` that ends it, is the release's notes.

## v0.2.0

**Date:** 2026-10-29  
**Previous release:** v0.1.0

### Summary

MQTT logins by token, tested on a broker that checks them: `vtn start
--mqtt-client-auth token` has clients log in to the broker with their OAuth2
token, and serves `POST /auth/mqtt` for the broker's HTTP authenticator to
check it. `scripts/mqtt-token.sh` runs it on EMQX 6.3.1. Webhook delivery
from OpenLEADR, untested in v0.1.0, turns out not to exist: its VTN has no
webhook sender.

### Added

- **`vtn start --mqtt-client-auth anonymous|token`** (or
  `OPENADRCTL_VTN_MQTT_CLIENT_AUTH`): with `token`, `GET /notifiers` announces
  `OAUTH2_BEARER_TOKEN` with the user name `{clientID}`, and the VTN serves
  **`POST /auth/mqtt`**: a JSON or form login, answered `200
  {"result":"allow"}` for a token the VTN issued to that client ID or for the
  VTN's own publisher (`--mqtt-username`, `OPENADRCTL_VTN_MQTT_PASSWORD`), and
  `403 {"result":"deny"}` otherwise. Each answer is a **`mqtt_login`** record.
  Needs `--mqtt` and a seed with clients (exit 3 otherwise).
- **`scripts/mqtt-token.sh`:** `vtn start` with EMQX 6.3.1 as the broker, its
  HTTP authenticator pointed at `/auth/mqtt`.

### Changed

- **`listen --via mqtt`** reads the authentication method `OAUTH2BEARERTOKEN`,
  as OpenLEADR writes it, as `OAUTH2_BEARER_TOKEN`, and logs in with the
  client's token for it too.

### Verification

- `make check` (coverage 90 %), and the race tests on Linux with Go 1.26.
- A new test runs `vtn start --mqtt-client-auth token` with a broker that asks
  `/auth/mqtt` for every login: Business Logic and a VEN receive
  notifications with their tokens, a password that is not a token exits 4,
  the VTN's publisher logs in as itself, and the endpoint answers JSON, forms,
  another client's token, a wrong method and bad JSON as documented.
- `scripts/mqtt-token.sh` on EMQX 6.3.1: 5 checks passed.
- `scripts/interop.sh` against OpenLEADR (openadr-interop v0.4.0): 25 checks
  passed.
- Webhook delivery from OpenLEADR v0.2.7: a subscription with a callback the
  container reaches is accepted, and nothing is posted to it; its source has
  no webhook sender (its only HTTP client fetches JWKS keys).

### Known limitations

- `/auth/mqtt` checks who logs in, not which topics they subscribe to: keeping
  a VEN to its own topics is for the broker's rules.
- Webhook delivery is tested against openadrctl's own VTN only: no other open
  VTN in openadr-interop sends webhooks.
- Not tested against an OpenADR 3.0.1 VTN. OpenADR 2.0 is not supported.

---

## v0.1.0

**Date:** 2026-10-29  
**Previous release:** none

### Summary

First release. openadrctl is a command-line tool for **OpenADR 3.1.0**, built
on [go-openadr](https://github.com/otfabric/go-openadr) v0.4.0, in every
role: a **VEN** that reads the programs and events meant for it and reports
on them, **Business Logic** that publishes prices and limits and manages
VENs, a **test VTN** served from a seed file, and notifications by webhook,
MQTT and WebSocket. The command line, the JSON output and the exit codes
follow the other otfabric tools (ocppctl, ieee20305ctl, modbusctl), and a
guide shows every scenario with real output, against its own VTN and against
the OpenLEADR VTN of openadr-interop v0.4.0.

### Added

- **`ven` and `bl`:** `list`, `get`, `create`, `update` and `delete` of
  programs, events, reports, VENs, resources, subscriptions and resource
  groups, as far as each role may: objects from flags (names, targets,
  payload descriptors, attributes, intervals from `--type`, `--value`,
  `--start` and `--interval`, or from a CSV file) or from a JSON file
  (`--file`, `-` for stdin). Every object is validated against the schema
  before it is sent; `--dry-run` shows it and sends nothing (a create needs
  no VTN for it). `update` changes only what the flags name. Events need
  `--yes` to be created or changed, every object to be deleted.
- **Searches:** every page by default, one with `--skip` and `--limit`; the
  filters of the API (`--program`, `--event`, `--target`, `--client-name`,
  `--name`, `--ven`, `--active`).
- **`events timeline`** and **`timeline`** (offline): the intervals of an
  event in time, the rows in force at a time (`--at`), sub-intervals,
  CSV that `events create --csv` reads back.
- **`listen --via webhook|mqtt|websocket`:** subscribes, prints one record
  per notification, and deletes its subscription at the end (unless
  `--keep`). The webhook listener serves http or https; MQTT reads the broker
  and the topics of 12 topic sets from the VTN and logs in with the client's
  token when the VTN asks for one.
- **`events watch`:** polls any VTN and prints the events present, created,
  updated and deleted.
- **`notifiers`** and **`topics`:** how the VTN notifies, and its MQTT
  topics.
- **`vtn start`:** the OpenADR 3.1.0 API from a seed file of clients
  (`bl` and `ven`, with the scopes the specification gives them) and objects,
  an OAuth2 token endpoint, webhooks (`--webhooks`, `--insecure-webhooks`),
  MQTT to a broker of your own, WebSockets, resource groups, anonymous
  reads, rate limits, a store kept in a file (with notifications that survive
  a restart), https, DNS-SD advertisement, and one record per request,
  notification and WebSocket. The seed is checked before anything listens,
  and every problem is reported at once. **`vtn generate-config`** writes the
  example seed.
- **`ping`:** connect, TLS, token, API and notifiers, each timed; the exit
  code is that of the step that failed.
- **`token`**, **`discover`** (DNS-SD), **`validate`** (offline, misspelt
  properties included), **`completion`**, **`version`** (with go-openadr's).
- **Connection:** client credentials with token discovery
  (`GET /auth/server`) or a bearer token; secrets only from the environment
  or files; `--ca`, mutual TLS (`--cert`, `--key`), `--insecure`;
  `--timeout`; `--retries` with exponential backoff for what another attempt
  can cure, never resending a create whose outcome is unknown.
- **Output:** `--format text|table|json` on every command with a result,
  JSON Lines for streams, objects as OpenADR 3.1.0 has them
  ([docs/json-output.md](docs/json-output.md)); errors as
  `{"error": {"kind", "code", "message"}, "openadr": {…}}` with the VTN's
  problem object; the exit codes of the other otfabric tools
  ([docs/exitcodes.md](docs/exitcodes.md)).
- **Environment:** every connection flag as `OPENADRCTL_*`; a flag wins; a
  malformed value exits 3.
- **Docs:** README with installation for Linux, macOS and Windows, a guide of
  13 scenarios with real output, exit codes, JSON fields, the VTN and its
  seed file.
- **`scripts/interop.sh`:** openadrctl against the OpenLEADR VTN of
  openadr-interop, in both roles.

### Verification

- `make check`: gofmt, go vet, golangci-lint, `go test -race` and coverage
  (90 %, minimum 85 %), on Linux with Go 1.26.
- The tests run the real command tree against `vtn start` on a free port,
  with an MQTT broker in the process: every noun in both roles, the three
  notification mechanisms, events watch, TLS, the store across a restart,
  timeouts, retries, a token endpoint that misbehaves, and the exit codes 2
  to 6.
- `scripts/interop.sh` against the OpenLEADR VTN of openadr-interop v0.4.0:
  25 checks passed (objects, reports, a VEN's token refused, events watch,
  WebSocket and MQTT notifications).

### Known limitations

- `vtn start` is a test VTN: no administration API, clients only from the
  seed, tokens in memory.
- `vtn start --mqtt` needs a broker of your own.
- Webhook delivery from OpenLEADR is not tested: its VTN does not announce
  webhooks.
- The MQTT login with the client's token (a VTN that announces
  `OAUTH2_BEARER_TOKEN`) is not tested against a broker that checks the
  token: the broker of the OpenLEADR image takes the client secret, given
  with `OPENADRCTL_MQTT_PASSWORD`.
- Not tested against an OpenADR 3.0.1 VTN. OpenADR 2.0 is not supported.
- DNS-SD needs multicast on the local network.

---
