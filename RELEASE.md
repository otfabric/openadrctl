# openadrctl Releases

Releases are cut with the `Release` workflow (semantic versioning, annotated
tag, GitHub release with binaries for every platform). The section of the
version below, up to the `---` that ends it, is the release's notes.

## v0.7.2

**Date:** 2026-11-13
**Previous release:** v0.7.1

### Changed

- Built on go-openadr v0.7.2 (was v0.7.1), whose interop suite pins openadr-interop v0.8.2 and the latest MQTT brokers; the library does not change. `openadrctl` code is unchanged.
- `scripts/interop.sh` and `scripts/interop301.sh` default to openadr-interop v0.8.2 (were v0.6.3). Both pass against it: 25 checks against OpenLEADR v0.2.8, and 37 with `--spec 3.0.1` against OpenLEADR v0.1.4.

---

## v0.7.1

**Date:** 2026-11-12
**Previous release:** v0.7.0

### Changed

- Built on go-openadr v0.7.1 (was v0.7.0), whose interop suite pins openadr-interop v0.8.1; the library does not change. `openadrctl` code is unchanged.

---

## v0.7.0

**Date:** 2026-11-08
**Previous release:** v0.6.1

### Changed

- Built on go-openadr v0.7.0, whose VTN and token issuer were checked against recorded exchanges with OpenLEADR, cB-Sigma and ElaadNL.
- `vtn`: a request body declared as anything other than JSON (for example `text/plain` or a form) is refused with 415; it used to be read as JSON. A body without a Content-Type is still read as JSON.
- `vtn`: every 401 from the token endpoint carries `WWW-Authenticate: Basic realm="openadr"`, also when the client sent its credentials in the form.

---

## v0.6.1

**Date:** 2026-11-08
**Previous release:** v0.6.0

### Changed

- Release binaries are built with the latest Go 1.26 patch release; v0.6.0 was built with go1.26.8.
- Requires Go 1.26.0 or later (was 1.26.8), the organisation floor; nothing needed the higher one.

---

## v0.6.0

**Date:** 2026-11-07
**Previous release:** v0.5.2

### Added

- `version --json` (also `OPENADRCTL_FORMAT=json`): the build information as one JSON object, keys `name`, `version`, `tag`, `commit`, `built`, `libraries`, `go`, `platform`, always present and in this order.
- Shell completion completes the value of every flag that takes one: `--spec` (3.1.0, 3.0.1), `--via`, `--mqtt-client-auth`, `--object`, `--operation`, `--at` and `--start` (`now`), the URL schemes of `--url`, `--token-url`, `--public-url`, `--callback-url`, `--broker` and `--mqtt`, `--child` (`resource:`, `group:`), PEM files for `--ca`, `--cert`, `--key`, `--mqtt-ca`, `--tls-cert` and `--tls-key`; the topic sets of `topics`, JSON files for the file arguments of `validate` and `timeline`; numbers, durations, IDs and names no longer complete file names.

### Changed

- **Breaking for scripts that read `version`:** `version` prints one line, `openadrctl 0.6.0 (tag v0.6.0, commit <sha>, built <date>, go-openadr 0.6.0, go1.26.10, linux/amd64)`, instead of a block of `version:`, `tag:`, `commit:`, `build date:`, `openadr:`, `go version:` and `platform:` lines. The OpenADR revisions are no longer listed there (they are in the README); go-openadr's version is. The version has no leading `v`. A development build reports `dev` and the commit from the Go VCS stamp (`-dirty` for a modified tree); `go install …@vX.Y.Z` reports that version.
- `make build` stamps a version and tag only when HEAD is a tag (a build off a tag reports `dev`), and the full commit SHA.

---

## v0.5.2

**Date:** 2026-11-07
**Previous release:** v0.5.1

### Fixed

- The help of the event commands said "a event" (`Create a event`, `Read a event`, `Delete a event`, `Change a event`); it now says "an event".

### Documentation

- `docs/commands.md`: a reference of every command and its flags (docsync).

---

## v0.5.1

**Date:** 2026-11-05
**Previous release:** v0.5.0

### Fixed

- `--value` and `--attribute` take a number only when it is written in decimal, as JSON writes numbers.
  - `0x1p3` was sent as the number 8; `0x1p3`, `1_000`, `Inf` and `NaN` are now sent as text, which OpenADR allows a value to be. `Inf` and `NaN` used to be refused as an invalid object (exit 3).
  - A decimal too large for a number, such as `1e400`, was sent as the text "1e400". It is now refused with exit 3 `invalid_input`. Before, it exited 0.
  - The same rules hold for each part of an `x:y` point.

---

## v0.5.0

**Date:** 2026-11-05
**Previous release:** v0.4.2

### Summary

Hardening after a SAST review: openadrctl no longer trusts what a VTN, a token endpoint, a broker, a webhook sender or an input file sends it. Built on go-openadr v0.6.0. A few failures now exit with a different code; they are listed below.

### Exit codes

| Case | Old | New | JSON error code |
|---|---|---|---|
| `--token-url` that is not http or https | 4 | 3 | `invalid_url` |
| `listen --via mqtt`: the VTN announces a broker that is not an MQTT URL, or no topics for a set | 3 | 6 | `invalid_response` |
| A token with characters a bearer token does not have | 0 or 4 | 6 | `invalid_response` |
| More than 10 redirects | 4 | 6 | `invalid_response` |
| A token endpoint or `GET /auth/server` that redirects to another origin | followed | 6 | `token_refused` or `http_error`, with the 3xx status |

These cases are new, all exit 6 `invalid_response`:
- a discovered token URL that is not http or https;
- a subscription created without a usable ID;
- a plain `mqtt://` broker announced by an https VTN when the client logs in with a token.

These cases are new, all exit 3:
- input over 16 MiB: an object file or stdin, `--csv`, a token, secret or password file, or a CA bundle (`invalid_input`);
- a seed over 16 MiB (`invalid_config`);
- a certificate or key over 16 MiB (`invalid_certificate`);
- `vtn start --rate` NaN or Inf;
- negative ISO 8601 durations for `--interval` and `--duration`;
- NaN or Inf in a float environment variable.

See [docs/exitcodes.md](docs/exitcodes.md).

### Fixed

- **Credentials:**
  - A token request no longer follows a redirect to another origin, so the client secret is not sent there.
  - Passwords in URLs are shown as `xxxxx` in output, errors and progress lines.
  - `listen --via mqtt` no longer sends the client's token over plain MQTT for an https VTN. `--broker` still overrides this.
- **Hostile tokens and answers:**
  - A bearer token with control characters or spaces is refused; it is no longer printed or put in a header.
  - A huge `expires_in` is capped at 100 years, and a negative one is dropped.
  - A token answer is read up to 1 MiB.
  - Response heads are limited to `--timeout` and 1 MiB.
- **Terminal output:** control characters, bidirectional overrides and invalid UTF-8 from a VTN, a file or a notification are printed as escapes in text and table output and in `error:` lines. Error messages are cut at 1024 bytes. JSON output keeps the original text.
- **`listen` and `events watch`:**
  - The `listen --via webhook` server has read, write and idle timeouts and a 64 KiB header limit, so a sender that never finishes cannot hold a connection open.
  - No records are written after a stream ends.
  - `events watch` stops its poll when it ends.
- **Files:**
  - Every file the user names, and stdin, is read up to 16 MiB. A device or FIFO without end is refused rather than read without limit.
  - `vtn generate-config --output --force` now replaces the file with a new one of mode 0600, as it already did without `--force`; it no longer writes through a symbolic link at that path. The seed holds client secrets.

### Changed

- Go 1.26 code (`go fix`).
- CI workflows otfabric/.github v3.3.1.
- Interop scripts default to openadr-interop v0.6.3.
- New fuzz targets: flag values, environment loading and output escaping.

---

## v0.4.2

**Date:** 2026-10-31
**Previous release:** v0.4.1

### Changed

- Test only: the broker helper no longer hangs in a mochi 2.7.9 deadlock at shutdown.

---

## v0.4.1

**Date:** 2026-10-31
**Previous release:** v0.4.0

### Changed

- `make build` writes to `bin/`, no longer the repository root. Fuzz targets and a coverage floor added.
- Built on go-openadr v0.5.1.

---

## v0.4.0

**Date:** 2026-10-30
**Previous release:** v0.3.0

### Summary

**Breaking for scripts:** the exit codes follow the OT Fabric organisation table (ADR-0006 of otfabric/engineering). Nothing else changes in the commands.

### Exit codes

| Case | Old | New | JSON error |
|---|---|---|---|
| `context_canceled` (interrupted before a result) | 10 | 130 | kind `interrupted`, was `internal` |

Every other exit code is unchanged. See [docs/exitcodes.md](docs/exitcodes.md).

### Environment

`OPENADRCTL_FORMAT` is now the canonical name of the format variable; `OPENADRCTL_OUTPUT_FORMAT` stays accepted, and `OPENADRCTL_FORMAT` wins when both are set.

---

## v0.3.0

**Date:** 2026-10-29  
**Previous release:** v0.2.0

### Summary

OpenADR 3.0.1, as a VEN and as Business Logic: `--spec 3.0.1` talks to a VTN
of the earlier revision, on go-openadr v0.5.0. `scripts/interop301.sh` runs it
against OpenLEADR v0.1.4, the last OpenLEADR for OpenADR 3.0.

### Added

- **`--spec 3.0.1`** (or `OPENADRCTL_SPEC`) on every client command, `ping`,
  `token` and `validate`. With it:
  - `--target` is `TYPE=value` (`--target GROUP=group-a`), as 3.0.1 targets
    have a type; a search names targets of one type, and targets without a
    type exit 3;
  - the token comes from `/auth/token` of the VTN unless `--token-url` says
    otherwise, as 3.0.1 has no `GET /auth/server`;
  - resources are reached through their VEN: `--ven` on `resources list`,
    `get`, `update` and `delete`;
  - a report names its program, read from its event;
  - objects are read and written as the JSON of 3.0.1 (`--format json`,
    `--file`, `validate`); its program properties (`programLongName`,
    `country`, …) are shown as the attributes 3.1.0 made them (`COUNTRY`);
  - `listen` takes `--via webhook` with `--program` only;
  - `notifiers`, `topics` and resource groups exit 3 with the new code
    **`not_in_revision`**, without sending anything, and `ping` skips its
    `notifiers` step.
- **`scripts/interop301.sh`:** openadrctl with `--spec 3.0.1` against the
  OpenLEADR v0.1.4 VTN of openadr-interop (`openleadr301`).

### Changed

- go-openadr v0.5.0. `version` says `openadr: 3.1.0, and 3.0.1 as a client`.
- `resources get` and `delete` take `--ven` with 3.1.0 too, where it is not
  needed and not sent.
- `make lint-ci` runs with golangci-lint v2, which has no `--out-format`.

### Verification

- `make check` (coverage 90 %), and the race tests on Linux with Go 1.26.
- A new test drives a 3.0.1 VTN double: the token from `/auth/token`, typed
  targets in searches, objects and `--dry-run`, resources through `--ven`,
  `not_in_revision` with nothing sent, `listen` refused but by webhook with a
  program, and `validate --spec 3.0.1`.
- `scripts/interop301.sh` against OpenLEADR v0.1.4 (openadr-interop v0.5.0):
  37 checks passed.
- `scripts/interop.sh` against OpenLEADR v0.2.7 (openadr-interop v0.5.0):
  25 checks passed.

### Known limitations

- **3.0.1 as a client only:** `vtn start` serves 3.1.0.
- With `--spec 3.0.1`, an event's `--duration` and `events list --active`
  exit 3: 3.0.1 has neither.
- **OpenLEADR v0.1.4**, found by `scripts/interop301.sh`: a search for
  `VEN_NAME` targets finds no program (it keeps those targets apart, to
  decide who sees a program), and a VEN cannot delete its report (403; it
  lets only Business Logic, against the specification, and openadrctl's
  Business Logic has no report delete). It serves no subscriptions, so
  `listen` has nothing to subscribe to there.

---

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
