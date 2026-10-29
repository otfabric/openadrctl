# openadrctl — OpenADR 3 Command-Line Tool: Act as a VEN or as Business Logic, Serve a Test VTN, Listen for Notifications

[![Go Version](https://img.shields.io/badge/Go-1.26.8%2B-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![CI Status](https://github.com/otfabric/openadrctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/openadrctl/actions/workflows/ci.yml)
[![Code Coverage](https://codecov.io/gh/otfabric/openadrctl/graph/badge.svg)](https://codecov.io/gh/otfabric/openadrctl)
[![Latest Release](https://img.shields.io/github/v/release/otfabric/openadrctl?label=release)](https://github.com/otfabric/openadrctl/releases)

openadrctl speaks [OpenADR 3](https://www.openadr.org/specification), revision
3.1.0, the demand-response protocol between a utility or aggregator and the
sites that respond to its prices and load-shed requests, in every role: as a
**VEN** (a site), as **Business Logic** (the utility's side), and as a
**VTN** (the server between them). It is built on
[go-openadr](https://github.com/otfabric/go-openadr), and is one static binary
with no runtime dependencies.

**Docs:** [guide](docs/guide.md) (common scenarios, with real output) ·
[exit codes](docs/exitcodes.md) · [JSON output](docs/json-output.md) ·
[VTN and seed file](docs/server-config.md) · [RELEASE.md](RELEASE.md)

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

- [Features](#features)
- [Installation](#installation)
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
| **VTN** (`vtn start`) | The OpenADR 3.1.0 API from a seed file, an OAuth2 token endpoint with `bl` and `ven` clients, webhooks, MQTT (to your broker), WebSockets, resource groups, anonymous reads, rate limits, a store kept in a file, DNS-SD advertisement, https, one record per request and notification |
| **Tools** | `ping` (connect, TLS, token, API, notifiers), `token`, `discover` (DNS-SD), `validate` (objects against the schema, misspelt properties included), `timeline` (an event file, offline) |
| **Connection** | Client credentials with token discovery (`GET /auth/server`) or a bearer token, custom CAs, mutual TLS, retries with backoff for what another attempt can cure, a timeout per command |

## Installation

Download a pre-built binary from the [GitHub Releases page](https://github.com/otfabric/openadrctl/releases). No Go toolchain is required.

| Platform | CPU | Download |
|---|---|---|
| Linux | x86-64 | `openadrctl-<version>-linux-amd64.tar.gz` |
| Linux | 64-bit ARM | `openadrctl-<version>-linux-arm64.tar.gz` |
| Linux | 32-bit ARM | `openadrctl-<version>-linux-armv7.tar.gz` |
| macOS | Apple Silicon | `openadrctl-<version>-darwin-arm64.tar.gz` |
| macOS | Intel | `openadrctl-<version>-darwin-amd64.tar.gz` |
| Windows | x86-64 | `openadrctl-<version>-windows-amd64.zip` |
| Windows | ARM64 | `openadrctl-<version>-windows-arm64.zip` |

Each release has `openadrctl-<version>-checksums.txt` (SHA-256).

### Linux

```bash
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/openadrctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in
  x86_64)         ARCH=amd64 ;;
  aarch64|arm64)  ARCH=arm64 ;;
  armv7l)         ARCH=armv7 ;;
esac
FILE="openadrctl-${VERSION}-linux-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/openadrctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}" && curl -fsSLO "${BASE}/openadrctl-${VERSION}-checksums.txt"
grep "${FILE}" "openadrctl-${VERSION}-checksums.txt" | sha256sum -c -
mkdir -p openadrctl-dl && tar -xzf "${FILE}" -C openadrctl-dl
sudo install -m 0755 "$(find openadrctl-dl -type f -name 'openadrctl*' | head -n1)" /usr/local/bin/openadrctl
```

### macOS

```bash
VERSION=$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/otfabric/openadrctl/releases/latest | sed 's|.*/v||')
case "$(uname -m)" in arm64) ARCH=arm64 ;; x86_64) ARCH=amd64 ;; esac
FILE="openadrctl-${VERSION}-darwin-${ARCH}.tar.gz"
BASE="https://github.com/otfabric/openadrctl/releases/download/v${VERSION}"
curl -fsSLO "${BASE}/${FILE}" && curl -fsSLO "${BASE}/openadrctl-${VERSION}-checksums.txt"
grep "${FILE}" "openadrctl-${VERSION}-checksums.txt" | shasum -a 256 -c -
mkdir -p openadrctl-dl && tar -xzf "${FILE}" -C openadrctl-dl
sudo install -m 0755 "$(find openadrctl-dl -type f -name 'openadrctl*' | head -n1)" /usr/local/bin/openadrctl
```

### Windows (PowerShell)

```powershell
$version = (Invoke-RestMethod https://api.github.com/repos/otfabric/openadrctl/releases/latest).tag_name.TrimStart('v')
$arch    = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$file = "openadrctl-$version-windows-$arch.zip"
$base = "https://github.com/otfabric/openadrctl/releases/download/v$version"
Invoke-WebRequest "$base/$file" -OutFile "$env:TEMP\$file"
Invoke-WebRequest "$base/openadrctl-$version-checksums.txt" -OutFile "$env:TEMP\openadrctl-checksums.txt"
$expected = ((Select-String -Path "$env:TEMP\openadrctl-checksums.txt" -Pattern $file).Line -split '\s+')[0]
if ($expected -ne (Get-FileHash "$env:TEMP\$file" -Algorithm SHA256).Hash) { throw "Checksum mismatch for $file" }
$dir = "$env:LOCALAPPDATA\Programs\openadrctl"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Expand-Archive "$env:TEMP\$file" -DestinationPath "$env:TEMP\openadrctl-dl" -Force
$exe = Get-ChildItem "$env:TEMP\openadrctl-dl" -Recurse -Filter 'openadrctl*.exe' | Select-Object -First 1
Copy-Item $exe.FullName "$dir\openadrctl.exe" -Force
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$dir*") { [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User') }
```

Open a new terminal for the `Path` change to apply. If Windows SmartScreen
warns about an unrecognised app, choose **More info → Run anyway**.

### Manual download

1. Open the [latest release](https://github.com/otfabric/openadrctl/releases/latest).
2. Download the archive for your platform from the table above.
3. Extract it, rename the binary to `openadrctl` (`openadrctl.exe` on
   Windows) and move it to a folder on your `PATH`.

### Update and uninstall

Run the same commands again to update: they fetch the latest release and
overwrite the previous binary. To uninstall, delete the binary
(`sudo rm /usr/local/bin/openadrctl`, or the folder
`%LOCALAPPDATA%\Programs\openadrctl` on Windows). openadrctl writes no
configuration of its own; `vtn start --store` writes only the file you name.

### Build from source

Go 1.26.8 or later:

```bash
go install github.com/otfabric/openadrctl@latest
```

or `git clone`, then `make build` (runs the checks and writes
`bin/openadrctl`).

Check that it works: `openadrctl version`.

### Shell completion

```bash
openadrctl completion bash > /etc/bash_completion.d/openadrctl    # bash
openadrctl completion zsh > "${fpath[1]}/_openadrctl"             # zsh
openadrctl completion fish > ~/.config/fish/completions/openadrctl.fish
```

PowerShell: `openadrctl completion powershell | Out-String | Invoke-Expression`.

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
VTN, `--cert` and `--key` a client certificate for mutual TLS.

Every command has `--help` with examples, and every command with a result
`--format text|table|json`.
`--debug` shows the causes of an error. Commands that change the VTN print
what they did (`created event 919fe864… (evening-peak)`); `events create`,
`events update` and every `delete` need `--yes`.

The scenarios, with their real output, are in the **[guide](docs/guide.md)**:
a VTN for development, reading as a VEN, publishing prices and limits,
reports, VENs and resources, notifications by WebSocket, webhook and MQTT,
polling, a VTN with TLS, a store and MQTT, scripting with JSON, and
troubleshooting.

## Commands

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
| `completion`, `version` | Shell completion; the version, with go-openadr's |

## Environment variables

Every connection flag can come from the environment, so that a shell session
sets the VTN and the client once.

<details>
<summary>All environment variables</summary>

| Variable | Flag |
|----------|------|
| `OPENADRCTL_URL` | `--url` |
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
| `OPENADRCTL_FORMAT` | `--format` |
| `OPENADRCTL_VIA`, `OPENADRCTL_LISTEN`, `OPENADRCTL_CALLBACK_URL` | `--via`, `--listen`, `--callback-url` of `listen` |
| `OPENADRCTL_MQTT_BROKER`, `OPENADRCTL_MQTT_CLIENT_ID`, `OPENADRCTL_MQTT_USERNAME`, `OPENADRCTL_MQTT_PASSWORD_FILE`, `OPENADRCTL_MQTT_CA` | the `--broker` and `--mqtt-*` flags of `listen` |
| `OPENADRCTL_MQTT_PASSWORD` | (no flag: the MQTT password of `listen`) |
| `OPENADRCTL_DISCOVER_WAIT` | `--wait` of `discover` |
| `OPENADRCTL_VTN_LISTEN`, `OPENADRCTL_VTN_PUBLIC_URL`, `OPENADRCTL_VTN_CONFIG`, `OPENADRCTL_VTN_STORE`, `OPENADRCTL_VTN_TLS_CERT`, `OPENADRCTL_VTN_TLS_KEY` | `--listen`, `--public-url`, `--config`, `--store`, `--tls-cert`, `--tls-key` of `vtn start` |
| `OPENADRCTL_VTN_MQTT`, `OPENADRCTL_VTN_MQTT_PUBLIC_URL`, `OPENADRCTL_VTN_MQTT_USERNAME` | `--mqtt`, `--mqtt-public-url`, `--mqtt-username` of `vtn start` |
| `OPENADRCTL_VTN_MQTT_PASSWORD` | (no flag: the password of `vtn start` at its broker) |

</details>

A flag given on the command line wins. A malformed value (a duration or a
number that does not parse) exits 3.

## Output and exit codes

Results go to stdout: text by default, `--format table`, or `--format json`
([fields](docs/json-output.md)). Objects are JSON as OpenADR 3.1.0 has them,
so that `get --format json` can be edited and sent back with `--file`.
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
says how). openadrctl has not been tested against a 3.0.1 VTN. OpenADR 2.0
(XML) is a different protocol and is not supported.

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
openadrctl itself is run against the OpenLEADR VTN of openadr-interop v0.4.0
by `scripts/interop.sh`: 25 checks in both roles, among them programs,
events, reports, a VEN's token refused (403), events watch, and notifications
by WebSocket and by MQTT on the image's broker. The [guide](docs/guide.md)
shows part of that run.

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
└── docs/                 guide, exit codes, JSON output, VTN and seed file
```

## Development

```bash
make check               # format, vet, lint, tests with the race detector, coverage (minimum 85%)
make build               # checks, then bin/openadrctl
make build-all           # every release platform
scripts/interop.sh       # against the OpenLEADR VTN (Docker)
```

The tests run the real command tree (`cmd.Run`) against `vtn start` on a free
port, with an MQTT broker in the process. See [CONTRIBUTING.md](CONTRIBUTING.md).

Release notes are in [RELEASE.md](RELEASE.md). Please report security
vulnerabilities privately, as [SECURITY.md](SECURITY.md) describes.

## License

[MIT](LICENSE)
