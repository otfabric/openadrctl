# openadrctl — OpenADR 3 Command-Line Tool: Act as a VEN or as Business Logic, Serve a Test VTN, Listen for Notifications

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI Status](https://github.com/otfabric/openadrctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/openadrctl/actions/workflows/ci.yml)
[![Code Coverage](https://codecov.io/gh/otfabric/openadrctl/graph/badge.svg)](https://codecov.io/gh/otfabric/openadrctl)
[![Latest Release](https://img.shields.io/github/v/release/otfabric/openadrctl?label=release)](https://github.com/otfabric/openadrctl/releases)

openadrctl speaks [OpenADR 3](https://www.openadr.org/specification), revision
3.1.0, the demand-response protocol between a utility or aggregator and the
sites that respond to its prices and load-shed requests, in every role: as a
**VEN** (a site), as **Business Logic** (the utility's side), and as a
**VTN** (the server between them). As a VEN and as Business Logic it also
speaks revision 3.0.1, with `--spec 3.0.1`. It is built on
[go-openadr](https://github.com/otfabric/go-openadr), and is one static binary
with no runtime dependencies.

**Docs:** [guide](docs/guide.md) (common scenarios, with real output) ·
[exit codes](docs/exitcodes.md) · [JSON output](docs/json-output.md) ·
[VTN and seed file](docs/server-config.md) ·
[command reference](docs/commands.md) (every command and flag) · [RELEASE.md](RELEASE.md)

## Install

```bash
curl -fsSL https://get.otfabric.io/openadrctl | sh
```

```powershell
irm https://get.otfabric.io/openadrctl.ps1 | iex
```

Linux (amd64, arm64, armv7), macOS (Intel, Apple silicon) and Windows (amd64, arm64). The installer verifies the release's SHA-256 checksum, installs per user without `sudo` (`~/.local/bin`, Windows `%LOCALAPPDATA%\OTFabric\bin`), and upgrades in place when run again. Pin a version with `OTF_VERSION=x.y.z`; for CI, containers and device images use `OTF_UNMANAGED_INSTALL=<dir>`; all options: [otfabric/install](https://github.com/otfabric/install#options). For offline machines, the archives and checksums are on [Releases](https://github.com/otfabric/openadrctl/releases).

From source (Go 1.26+): `go install github.com/otfabric/openadrctl@latest`

### Shell completion

Completion knows the commands and flags, the values of enum flags such as `--format`, `--spec`, `--via`, `--object`, `--operation` and `--mqtt-client-auth`, the topic sets of `topics`, the kinds of `validate`, and PEM, JSON and YAML files for `--cert`, `--key`, `--ca`, `--file`, `--config` and the file arguments.

```bash
# bash (needs the bash-completion package)
mkdir -p ~/.local/share/bash-completion/completions
openadrctl completion bash > ~/.local/share/bash-completion/completions/openadrctl

# zsh: a directory on your fpath, then restart the shell
mkdir -p ~/.zsh/completions && openadrctl completion zsh > ~/.zsh/completions/_openadrctl
# and once in ~/.zshrc:  fpath=(~/.zsh/completions $fpath); autoload -U compinit && compinit

# fish
mkdir -p ~/.config/fish/completions && openadrctl completion fish > ~/.config/fish/completions/openadrctl.fish
```

```powershell
# PowerShell: add this line to your profile ($PROFILE)
openadrctl completion powershell | Out-String | Invoke-Expression
```

## Why openadrctl?

- **Talk to a VTN as a VEN** from a shell: read the programs and events meant
  for the site, lay out an event's intervals in time, send reports, manage the
  VEN object and its resources.
- **Talk to it as Business Logic**: create programs and events (price
  signals, load limits), read what VENs report, manage VENs, resources and
  resource groups.
- **Hear what changes**: `listen` subscribes and prints notifications as they
  arrive, by **webhook**, **MQTT** or **WebSocket**; `events watch` polls any
  VTN and prints what was created, updated or deleted.
- **Serve a test VTN** (`vtn start`) from a YAML seed of clients, programs,
  VENs and events, with its own OAuth2 token endpoint, webhooks, MQTT and
  WebSockets, kept in memory or in a file.
- **Diagnose**: `ping` checks connect, TLS, token, API and notifiers one by
  one; errors carry the VTN's problem object and a hint.
- **Script it**: `--format json` on every command with a result, JSON Lines for streams,
  [stable exit codes](docs/exitcodes.md), environment variables for the
  connection.

## Quick start

```bash
# Terminal 1: a VTN with the example seed (clients utility and site-42)
openadrctl vtn start --websocket

# Terminal 2: the site, as a VEN
export OPENADRCTL_URL=http://127.0.0.1:8080
export OPENADRCTL_CLIENT_ID=site-42 OPENADRCTL_CLIENT_SECRET=site-42-secret
openadrctl ping                                        # every step, timed
openadrctl ven events list                             # what the VTN asks
openadrctl ven events timeline price-2030-01-01        # interval by interval
openadrctl ven listen --via websocket --object EVENT   # wait for new events

# Terminal 3: the utility, as Business Logic
OPENADRCTL_CLIENT_ID=utility OPENADRCTL_CLIENT_SECRET=utility-secret \
  openadrctl bl events create --url http://127.0.0.1:8080 --program dynamic-price \
    --name evening-peak --type PRICE --start 2030-01-01T17:00:00Z --interval PT1H \
    --value 0.35 --value 0.42 --yes
```

```console
$ openadrctl ping
connect    ok           0ms  127.0.0.1:8080
tls        skipped      0ms  plain http
token      ok           3ms  from http://127.0.0.1:8080/auth/token, valid 1h0m0s
api        ok           0ms  GET /programs: dynamic-price, ...
notifiers  ok           0ms  websocket
http://127.0.0.1:8080 answers

$ openadrctl ven events timeline price-2030-01-01
INTERVAL  START                 END                   TYPE   VALUES
--------  -----                 ---                   ----   ------
0         2030-01-01T00:00:00Z  2030-01-01T01:00:00Z  PRICE  0.17
1         2030-01-01T01:00:00Z  2030-01-01T02:00:00Z  PRICE  0.21
2         2030-01-01T02:00:00Z  2030-01-01T03:00:00Z  PRICE  0.25

$ openadrctl ven listen --via websocket --object EVENT
subscribed: subscription b3ae5c775ac0ad0ab81f98239a6c4af0, CREATE,UPDATE,DELETE of EVENT
listening: WebSocket open
2026-10-28T23:58:33Z  websocket EVENT        CREATE 919fe864521e897f30c7b9a2ffed4491 (evening-peak)
```

The [guide](docs/guide.md) walks through every scenario with its output.

---

## How it compares

OpenADR 3 is a REST API, so `curl` reaches every endpoint, and the open-source
implementations (go-openadr, [OpenLEADR](https://github.com/OpenLEADR/openleadr-rs),
the [ElaadNL client](https://github.com/ElaadNL/openadr3-client)) do
everything once you have written a program. openadrctl sits in between.

| | openadrctl | curl and jq | OpenADR libraries | A VTN product |
|---|:---:|:---:|:---:|:---:|
| Fetches and renews the OAuth2 token by itself | yes | — | yes | — |
| Builds programs, events and reports from flags, schema-checked before sending | yes | — | yes (it is code) | in its UI |
| Every page of a search, interval timelines, readable tables | yes | — | with code | in its UI |
| Webhook, MQTT and WebSocket notifications from one command | yes | — | with code | — |
| A VTN to point a VEN at, from a seed file | yes | — | with code | yes |
| Works over SSH, in a container, in CI; JSON, exit codes | yes | yes | yes | rarely |
| Certification of a product | — | — | — | — |

openadrctl is a test, development and diagnostic tool. It is not an OpenADR
Alliance certification test, and `vtn start` is a test VTN, not a utility's
production server.

## Use cases

- **VEN development** — Point the VEN at `vtn start`, create events as
  Business Logic from another terminal, and watch the VEN's requests with
  `--log-requests`.
- **VTN development and regression tests** — Drive your VTN in both roles
  from a CI job: `--format json`, exit codes, `scripts/interop.sh` as a
  template.
- **Commissioning** — Can this site reach the utility's VTN with these
  credentials? Which programs and events does it see, and how does the VTN
  notify? `ping`, `ven programs list`, `ven notifiers`.
- **Operations** — Publish a price curve from a CSV file, read back the
  reports VENs sent, follow events as they change.
- **Learning OpenADR 3** — The objects, targets, scopes and notifications of
  the protocol, with a VTN on your laptop.

---

## Table of Contents

- [Install](#install)
- [Features](#features)
- [OpenADR 3 in two minutes](#openadr-3-in-two-minutes)
- [Usage](#usage)
- [Commands](#commands)
- [Environment variables](#environment-variables)
- [Output and exit codes](#output-and-exit-codes)
- [FAQ](#faq)
- [Interoperability](#interoperability)
- [Limitations](#limitations)
- [Project structure](#project-structure)
- [Development](#development)
- [License](#license)

## Features

| Area | What openadrctl does |
|------|----------------------|
| **VEN** (`ven`) | Programs and events the VEN may see (`--target`, `--active`), event timelines, reports (create, update, delete), its VEN object and resources, subscriptions, notifiers and MQTT topics |
| **Business Logic** (`bl`) | Programs and events (create, update, delete; events need `--yes`), reports, VENs and resources with targets and client IDs, resource groups (extension), subscriptions |
| **Objects** | From flags (names, targets, payload descriptors, intervals with `--type`, `--value`, `--start`, `--interval` or a CSV file, attributes) or from a JSON file (`--file`, `-` for stdin); `--dry-run` shows the object without sending it; `update` changes only what is given |
| **Searches** | Every page by default, or one (`--skip`, `--limit`); filters as the API has them (`--program`, `--event`, `--target`, `--client-name`, `--name`, `--ven`) |
| **Notifications** | `listen --via webhook` (a listener of its own, http or https), `--via mqtt` (broker and topics from the VTN, 12 topic sets), `--via websocket`; the subscription is deleted at the end unless `--keep` |
| **Polling** | `events watch`: present, created, updated and deleted events, for any VTN |
| **VTN** (`vtn start`) | The OpenADR 3.1.0 API from a seed file, an OAuth2 token endpoint with `bl` and `ven` clients, webhooks, MQTT (to your broker, with clients logging in by token if you like: `--mqtt-client-auth token` and `POST /auth/mqtt` for the broker), WebSockets, resource groups, anonymous reads, rate limits, a store kept in a file, DNS-SD advertisement, https, one record per request and notification |
| **Tools** | `ping` (connect, TLS, token, API, notifiers), `token`, `discover` (DNS-SD), `validate` (objects against the schema, misspelt properties included), `timeline` (an event file, offline) |
| **Connection** | Client credentials with token discovery (`GET /auth/server`) or a bearer token, custom CAs, mutual TLS, retries with backoff for what another attempt can cure, a timeout per command |

## OpenADR 3 in two minutes

- **REST and JSON over HTTPS.** A **VTN** (Virtual Top Node) serves the API;
  everything else is a client of it. OpenADR 3 is not compatible with
  OpenADR 2.0 (XML, SOAP-like services).
- **Two kinds of client.** **Business Logic** (BL) is the utility's or
  aggregator's side: it writes programs and events. A **VEN** (Virtual End
  Node) is a site, or an aggregator acting for sites: it reads what is meant
  for it and reports back.
- **OAuth2.** Clients authenticate with the client-credentials grant; the VTN
  names its token endpoint at `GET /auth/server`. The token's **scopes**
  (`read_all`, `write_programs`, `write_events`, `read_targets`,
  `write_reports`, `write_vens`, …) decide what a client may do.
- **Programs and events.** A **program** is an offer: dynamic prices, a
  flexibility contract. An **event** belongs to a program and carries
  **intervals**: from a start, each with **payloads** such as `PRICE=0.21`,
  `IMPORT_CAPACITY_LIMIT=5` or `SIMPLE=1`. Events may ask for **reports**
  with report descriptors.
- **Reports.** A VEN reports on an event: readings per resource and interval
  (`USAGE`, `DEMAND`, …).
- **VENs and resources.** Business Logic (or the VEN itself) keeps a **VEN**
  object per client, with the **resources** behind it (meters, batteries,
  chargers).
- **Targets.** Programs and events can be meant for some VENs only: targets
  such as `group-a`, granted to a VEN by Business Logic. A VEN sees a targeted
  object when it searches with one of its targets.
- **Notifications.** A **subscription** names objects and operations
  (`CREATE`, `UPDATE`, `DELETE`); the VTN notifies by **webhook** (a callback
  URL), or by **MQTT** on topics it names (`GET /notifiers`). WebSockets are
  an extension of OpenLEADR and go-openadr.

## Usage

```text
openadrctl <role> <object> <action> [flags]
openadrctl <tool> [flags]
```

A client command connects to `--url`, the base URL of the VTN's API (the one
`/programs` is appended to, such as `https://vtn.example/openadr3/3.1.0`). It
fetches a token with `--client-id` and the secret from `OPENADRCTL_CLIENT_SECRET`
or `--client-secret-file` (never a flag, so that it stays out of the process
list and shell history), or sends a bearer token from `OPENADRCTL_TOKEN` or
`--token-file`. Without either it sends no token. `--ca` names the CA of the
VTN, `--cert` and `--key` a client certificate for mutual TLS. `--spec 3.0.1`
talks to a VTN of OpenADR 3.0.1 (see [OpenADR 3.0.1](#which-openadr-version-does-it-speak)).

Every command has `--help` with examples, and every command with a result
`--format text|table|json`.
`--debug` shows the causes of an error. Commands that change the VTN print
what they did (`created event 919fe864… (evening-peak)`); `events create`,
`events update` and every `delete` need `--yes`.

The scenarios, with their real output, are in the **[guide](docs/guide.md)**:
a VTN for development, reading as a VEN, publishing prices and limits,
reports, VENs and resources, notifications by WebSocket, webhook and MQTT,
polling, a VTN with TLS, a store and MQTT, OpenADR 3.0.1 against OpenLEADR
v0.1.4, scripting with JSON, and troubleshooting.

## Commands

Every command with all its flags: **[docs/commands.md](docs/commands.md)**.

| Command | Does |
|---------|------|
| `ven programs list`, `get` | The programs the VEN may see (`--target`, `--name`) |
| `ven events list`, `get`, `timeline`, `watch` | Its events (`--program`, `--target`, `--active`); the intervals of one in time; poll for changes |
| `ven reports list`, `get`, `create`, `update`, `delete` | Its reports on events |
| `ven vens …`, `ven resources …` | Its VEN object and resources |
| `ven resource-groups list`, `get` | Resource groups (extension) |
| `bl programs list`, `get`, `create`, `update`, `delete` | Programs: names, targets, payload descriptors, attributes |
| `bl events list`, `get`, `create`, `update`, `delete`, `timeline`, `watch` | Events: intervals from flags or CSV, priorities, targets, report descriptors |
| `bl reports list`, `get` | The reports VENs sent (`--event`, `--program`, `--client-name`) |
| `bl vens …`, `bl resources …`, `bl resource-groups …` | VENs and resources with targets and client IDs; resource groups |
| `ven\|bl subscriptions list`, `get`, `create`, `update`, `delete` | Subscriptions: objects, operations, callback URL or WebSocket |
| `ven\|bl notifiers` | How the VTN notifies: webhook, WebSocket, MQTT broker |
| `ven\|bl topics <set>` | The MQTT topics of a set: `events`, `program-events:<id>`, `ven-events:<id>`, … |
| `ven\|bl listen --via webhook\|mqtt\|websocket` | Subscribe, and print notifications as they arrive |
| `vtn start` | Serve a VTN from a seed; one record per thing that happens |
| `vtn generate-config` | Write the example seed |
| `ping` | Check a VTN step by step |
| `token` | Fetch an OAuth2 token (text: the token only) |
| `discover` | Find VTNs on the local network with DNS-SD |
| `validate <kind> <file>` | Check an object against the schema, offline |
| `timeline <event-file>` | Lay out the intervals of an event, offline |
| `completion`, `version` | Shell completion; the version on one line, with go-openadr's (`--json`: one object) |

## Environment variables

Every connection flag can come from the environment, so that a shell session
sets the VTN and the client once.

<details>
<summary>All environment variables</summary>

| Variable | Flag |
|----------|------|
| `OPENADRCTL_URL` | `--url` |
| `OPENADRCTL_SPEC` | `--spec` (3.1.0 or 3.0.1; also of `validate`) |
| `OPENADRCTL_CLIENT_ID` | `--client-id` |
| `OPENADRCTL_CLIENT_SECRET` | (no flag: the secret itself) |
| `OPENADRCTL_CLIENT_SECRET_FILE` | `--client-secret-file` |
| `OPENADRCTL_TOKEN` | (no flag: a bearer token) |
| `OPENADRCTL_TOKEN_FILE` | `--token-file` |
| `OPENADRCTL_TOKEN_URL` | `--token-url` |
| `OPENADRCTL_SCOPE` | `--scope` (comma-separated) |
| `OPENADRCTL_CA`, `OPENADRCTL_CERT`, `OPENADRCTL_KEY` | `--ca`, `--cert`, `--key` |
| `OPENADRCTL_INSECURE` | `--insecure` |
| `OPENADRCTL_TIMEOUT`, `OPENADRCTL_RETRIES` | `--timeout`, `--retries` |
| `OPENADRCTL_FORMAT` (`OPENADRCTL_OUTPUT_FORMAT` stays accepted) | `--format` |
| `OPENADRCTL_VIA`, `OPENADRCTL_LISTEN`, `OPENADRCTL_CALLBACK_URL` | `--via`, `--listen`, `--callback-url` of `listen` |
| `OPENADRCTL_MQTT_BROKER`, `OPENADRCTL_MQTT_CLIENT_ID`, `OPENADRCTL_MQTT_USERNAME`, `OPENADRCTL_MQTT_PASSWORD_FILE`, `OPENADRCTL_MQTT_CA` | the `--broker` and `--mqtt-*` flags of `listen` |
| `OPENADRCTL_MQTT_PASSWORD` | (no flag: the MQTT password of `listen`) |
| `OPENADRCTL_DISCOVER_WAIT` | `--wait` of `discover` |
| `OPENADRCTL_VTN_LISTEN`, `OPENADRCTL_VTN_PUBLIC_URL`, `OPENADRCTL_VTN_CONFIG`, `OPENADRCTL_VTN_STORE`, `OPENADRCTL_VTN_TLS_CERT`, `OPENADRCTL_VTN_TLS_KEY` | `--listen`, `--public-url`, `--config`, `--store`, `--tls-cert`, `--tls-key` of `vtn start` |
| `OPENADRCTL_VTN_MQTT`, `OPENADRCTL_VTN_MQTT_PUBLIC_URL`, `OPENADRCTL_VTN_MQTT_USERNAME`, `OPENADRCTL_VTN_MQTT_CLIENT_AUTH` | `--mqtt`, `--mqtt-public-url`, `--mqtt-username`, `--mqtt-client-auth` of `vtn start` |
| `OPENADRCTL_VTN_MQTT_PASSWORD` | (no flag: the password of `vtn start` at its broker) |

</details>

A flag given on the command line wins. A malformed value (a duration or a
number that does not parse) exits 3.

## Output and exit codes

Results go to stdout: text by default, `--format table`, or `--format json`
([fields](docs/json-output.md)). Objects are JSON as OpenADR 3.1.0 has them,
or 3.0.1 with `--spec 3.0.1`, so that `get --format json` can be edited and sent back with `--file`.
Streams (`listen`, `events watch`, `vtn start`, `discover`) write a record per
event, JSON Lines with `--format json`. Progress and diagnostics go to stderr.

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage: an unknown command or flag, a missing argument, a change without `--yes` |
| 3 | Invalid input: nothing was sent (a flag, file, seed or object that is wrong) |
| 4 | Transport: no connection, a TLS handshake that failed, a port that cannot be opened |
| 5 | Timeout: no answer in `--timeout` |
| 6 | The VTN refused (an HTTP error, a token refused) or broke the protocol |
| 10 | Output or internal error |
| 130 | Interrupted before a result |

Details, error codes and the JSON error object: [docs/exitcodes.md](docs/exitcodes.md).

```console
$ openadrctl bl programs delete dynamic-price --yes --format json
{
  "error": {
    "kind": "refused",
    "code": "conflict",
    "message": "DELETE /programs/dynamic-price: the VTN answered 409 Conflict: program dynamic-price still has events"
  },
  "openadr": {
    "method": "DELETE",
    "path": "/programs/dynamic-price",
    "status": 409,
    "problem": {
      "title": "Conflict",
      "status": 409,
      "detail": "program dynamic-price still has events"
    }
  }
}
```

## FAQ

### Which OpenADR version does it speak?

OpenADR 3.1.0, the revision go-openadr implements
([CONFORMANCE.md](https://github.com/otfabric/go-openadr/blob/main/docs/CONFORMANCE.md)
says how), in every role. As a VEN and as Business Logic also OpenADR 3.0.1,
with `--spec 3.0.1` (or `OPENADRCTL_SPEC=3.0.1`). What changes then:

| | With `--spec 3.0.1` |
|---|---|
| `--target` | `TYPE=value`, such as `--target GROUP=group-a`: 3.0.1 targets have a type. A search names targets of one type |
| Token | from `/auth/token` of the VTN unless `--token-url` says otherwise: 3.0.1 has no `GET /auth/server` |
| Resources | reached through their VEN: `--ven` on `resources list`, `get`, `update` and `delete` |
| Reports | name their program, which openadrctl reads from the event |
| `listen` | `--via webhook` and `--program` only: 3.0.1 has no MQTT or WebSockets, and a subscription names its program |
| Not there | `notifiers`, `topics` and resource groups exit 3 with `not_in_revision`, without sending anything; `ping` skips its `notifiers` step |
| JSON | `--format json`, `--file` and `validate --spec 3.0.1` are the JSON of 3.0.1 (`programLongName`, `country`, … as properties; openadrctl shows them as attributes, such as `COUNTRY`) |

`vtn start` serves 3.1.0 only. OpenADR 2.0 (XML) is a different protocol and
is not supported.

### The VTN answers 401. What should I check?

`ping` shows which step fails. Without `--client-id` and a secret (or a
token), openadrctl sends no token. With them, the `token` step shows the
token endpoint it used: `--token-url` overrides what `GET /auth/server` says.
`openadrctl token --format json` shows the scopes the VTN granted.

### The VTN answers 403.

The token lacks a scope: a VEN's credentials on a `bl` command, or a client
the VTN gave fewer scopes. The message names the scope when the VTN does.

### Why does a VEN not see a program I created?

The program has targets the VEN did not search with. `ven programs list
--target group-a` names one; Business Logic grants targets to the VEN object
(`bl vens update <id> --target group-a`).

### How do I receive notifications behind NAT or a firewall?

The VTN must reach a webhook's `--callback-url`. When it cannot, use `--via
mqtt` (the client connects out to the broker) or `--via websocket` (out to
the VTN, if the VTN offers it), or poll with `events watch`.

### A create timed out. Did it happen?

Perhaps: a request that changes the VTN and gets no answer exits 5 with
`outcome_unknown`, and is never sent again, also with `--retries`. List the
objects before you send it again.

### Can I turn off TLS verification?

`--insecure` does, for tests only. Prefer `--ca` with the VTN's CA.

### Is it safe to point openadrctl at a production VTN?

Reading is. Commands that change the VTN print what they did, events and
deletes need `--yes`, and `--dry-run` shows an object without sending it. A
created event reaches real sites: use a test program, or a test VTN.

## Interoperability

openadrctl is go-openadr, whose client and VTN are tested against
[OpenLEADR](https://github.com/OpenLEADR/openleadr-rs) and the
[ElaadNL client](https://github.com/ElaadNL/openadr3-client) through the images
of [openadr-interop](https://github.com/otfabric/openadr-interop) (see its
[INTEROP.md](https://github.com/otfabric/go-openadr/blob/main/INTEROP.md)).
openadrctl itself is run against the OpenLEADR VTN of openadr-interop v0.8.2
by `scripts/interop.sh`: 25 checks in both roles, among them programs,
events, reports, a VEN's token refused (403), events watch, and notifications
by WebSocket and by MQTT on the image's broker. The [guide](docs/guide.md)
shows part of that run.

With `--spec 3.0.1`, `scripts/interop301.sh` runs it against OpenLEADR v0.1.4,
an OpenADR 3.0.1 VTN (openadr-interop v0.8.2, `openleadr301`): 37 checks in
both roles, among them typed targets in objects and searches, resources
through their VEN, a report that names its program, `validate --spec 3.0.1`
of what `get` wrote, and the operations 3.0.1 does not have, refused before
anything is sent.

`scripts/mqtt-token.sh` runs MQTT with the clients' OAuth2 tokens on
[EMQX](https://www.emqx.io/) 6.3.1, whose HTTP authenticator checks every
login with `vtn start` (5 checks: Business Logic and a VEN receive with their
tokens, a password that is not a token is refused).

Webhook delivery is tested against openadrctl's own VTN only. OpenLEADR
v0.2.7 accepts a subscription with a callback URL but never posts to it (its
VTN has no webhook sender, and does not announce `WEBHOOK` in
`GET /notifiers`), and the ElaadNL client is a client only.

## Limitations

- **`vtn start` is a test VTN.** Its clients and secrets come from the seed;
  it has no administration API and no user interface. Business Logic commands
  are how you change it while it runs.
- **No OpenADR 2.0**, and no certification: passing a check says what you
  checked, not that a product conforms.
- **MQTT needs a broker of your own** for `vtn start --mqtt`; openadrctl does
  not embed one.
- **Webhooks need a reachable callback.** `listen --via webhook` serves the
  callback, but the VTN must be able to reach it; many VTNs accept only https
  on a public address (`vtn start` does too, unless `--insecure-webhooks`).
- **WebSockets and resource groups are extensions** of OpenLEADR and
  go-openadr, not part of OpenADR 3.1.0.
- **OpenADR 3.0.1 as a client only.** `vtn start` serves 3.1.0. With
  `--spec 3.0.1` an event's `--duration` is refused (3.0.1 events have none),
  and the 3.1.0 searches 3.0.1 does not have (`events list --active`) exit 3.
- **DNS-SD** needs multicast on the local network; it finds nothing across
  routers or in most container networks.

## Project structure

```
openadrctl/
├── main.go               build metadata, Execute
├── cmd/                  the command tree: root, ping, token, discover, validate, timeline
│   ├── client/           ven and bl: objects, notifiers, topics, listen, events watch
│   └── vtn/              vtn start and generate-config
├── internal/
│   ├── conn/             connection flags, the OAuth2 token, the classification of errors
│   ├── oadr/             objects as JSON, text and tables; event timelines
│   ├── seed/             the seed file of vtn start, and the example
│   ├── types/            the JSON error object
│   ├── errs/             error kinds, codes and exit codes
│   ├── format/           text, table, JSON and streams
│   ├── runner/           running a command, and rendering a fatal error
│   └── config/           flags and environment variables from struct tags
├── scripts/interop.sh    openadrctl against the OpenLEADR VTN of openadr-interop
└── docs/                 guide, commands, exit codes, JSON output, VTN and seed file
```

## Development

```bash
make check               # format, vet, lint, tests with the race detector, coverage (minimum 85%)
make build               # checks, then bin/openadrctl
make build-all           # every release platform
scripts/interop.sh       # against the OpenLEADR VTN (Docker)
scripts/mqtt-token.sh    # MQTT logins by token, on EMQX (Docker)
```

The tests run the real command tree (`cmd.Run`) against `vtn start` on a free
port, with an MQTT broker in the process. See [CONTRIBUTING.md](CONTRIBUTING.md).

Release notes are in [RELEASE.md](RELEASE.md). Please report security
vulnerabilities privately, as [SECURITY.md](SECURITY.md) describes.

## License

[MIT](LICENSE)
