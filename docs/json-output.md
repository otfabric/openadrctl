# `--format json` field conventions

These conventions are a **stable contract** for scripting. A breaking change
to a name or shape is called out in the release notes.

## General

| Concept | JSON |
|---------|------|
| OpenADR objects | As OpenADR 3.1.0 has them: `id`, `createdDateTime`, `programName`, `intervalPeriod`, … Whatever a `get` writes, `--file` and `validate` read back |
| Names openadrctl adds | snake_case (`token_url`, `requires_auth`, `randomize_start`); the names of OpenADR keep its spelling (`objectType`, `operation`) |
| Times | RFC 3339 in UTC (`2030-01-01T00:00:00Z`); records of streams have sub-second precision |
| Durations | ISO 8601 inside OpenADR objects (`PT1H`), as OpenADR has them; Go durations elsewhere (`retry_after`: `1s`) |
| Lists | **arrays**, empty rather than null |
| Fatal error | one object `{"error": {"kind", "code", "message"}, "openadr": {…}}` on stdout, nothing else; see [exitcodes.md](exitcodes.md) |

Fields that do not apply are omitted, not null.

## Objects

`get`, `create`, `update` and `delete` write the object, as the VTN returned
it (for `delete`: the object as it was). `--dry-run` writes the object that
would be sent; that of a create has no `id` and no times.

`list` writes `{"<kinds>": [...]}`, every page unless `--skip` or `--limit`:

| Command | Key |
|---------|-----|
| `programs list` | `programs` |
| `events list` | `events` |
| `reports list` | `reports` |
| `vens list` | `vens` |
| `resources list` | `resources` |
| `subscriptions list` | `subscriptions` |
| `resource-groups list` | `resource_groups` |

```console
$ openadrctl bl events list --format json | jq '.events | length'
2
```

## Results

### `ping`

| Field | Meaning |
|-------|---------|
| `url` | The base URL checked |
| `ok` | Whether every step passed |
| `stages` | `{"stage", "status", "ms", "detail", "error"}` per step: `connect`, `tls`, `token`, `api`, `notifiers`; `status` is `ok`, `failed` or `skipped`; `error` is `{"kind", "code", "message"}` of a failed step |

### `token`

`access_token`, `token_type`, `expires_in` (seconds), `scope` (as the token
endpoint gave it), `token_url`, `expires` (when it expires, RFC 3339).

### `notifiers`

`webhook` and `websocket` (booleans), and `mqtt` when the VTN announces a
broker: `{"uris", "serialization", "authentication", "username"}`.

### `topics`

`set` (as given), and the topics `create`, `update`, `delete` and `all`
the VTN named.

### `discover`

`vtns`: `{"instance", "url", "host", "port", "addresses", "version",
"requires_auth", "programs", "openapi_url", "txt"}` per VTN; `txt` is the
DNS-SD TXT record as advertised.

### `validate`

`kind`, `file`, and the `id` and `name` of the object when it has them.

### `timeline`, `events timeline`

| Field | Meaning |
|-------|---------|
| `event` | The event's ID, when it has one |
| `rows` | `{"interval", "sub", "start", "end", "type", "values", "randomize_start"}` per payload of an interval; `sub` numbers the sub-intervals of `--sub-intervals`; `values` as in the event (numbers, strings, booleans, points `{"x", "y"}`); `end` is absent for an interval without a duration |

`--csv` writes the same rows as CSV, with the header
`interval,sub,start,end,type,values,randomize_start`; `values` is the value
when there is one number or string, and JSON otherwise (`[1,2]`,
`{"x":1,"y":2}`). `events create --csv` reads this form.

## Streams (JSON Lines)

Streams write one JSON object per line, as each record happens.

### `listen`

| Field | Meaning |
|-------|---------|
| `time` | When openadrctl received it |
| `via` | `webhook`, `mqtt` or `websocket` |
| `topic` | The MQTT topic (MQTT only) |
| `objectType`, `operation` | As in the notification: `EVENT`, `CREATE`, … |
| `id`, `name` | Of the object |
| `object` | The object, as OpenADR has it |

The `subscribed:`, `listening:` and `unsubscribed:` lines go to stderr, and
are not records.

### `events watch`

`time`, `change` (`present`, `created`, `updated`, `deleted`), `id`, `name`,
and `event`: the event (for `deleted`, as it was last seen).

### `vtn start`

Every record has `time` and `event`:

| `event` | Fields |
|---------|--------|
| `listening` | `address` (where it listens), `url` (the public URL) |
| `client` | `client`, `role` (`bl` or `ven`): one per client of the seed |
| `seeded` | `count`: the objects added from the seed (0 for those already in `--store`) |
| `advertised` | `message`: the DNS-SD service and instance |
| `request` | `op` (`GET /programs`), `status`, `ms` (with `--log-requests`) |
| `notification` | `mechanism` (`webhook`, `mqtt`, `websocket`), `objectType`, `operation` |
| `socket_opened`, `socket_closed` | A WebSocket of a client; `message`: why it closed |
| `log` | `level`, `message`: a warning or error of the VTN |
| `mqtt_login` | `client` (the user name), `status` (200 allowed, 403 denied): a broker asked `POST /auth/mqtt` (`--mqtt-client-auth token`) |
| `stopped` | The end |

```console
$ openadrctl vtn start --log-requests --format json
{"time":"2026-10-28T23:53:28.642811Z","event":"listening","address":"127.0.0.1:8080","url":"http://127.0.0.1:8080"}
{"time":"2026-10-28T23:53:28.642912Z","event":"client","client":"utility","role":"bl"}
{"time":"2026-10-28T23:53:28.642927Z","event":"client","client":"site-42","role":"ven"}
{"time":"2026-10-28T23:53:28.642935Z","event":"seeded","count":6}
{"time":"2026-10-28T23:53:29.665895Z","event":"request","op":"GET /auth/server","status":200,"ms":0}
{"time":"2026-10-28T23:53:29.666493Z","event":"request","op":"POST /auth/token","status":200,"ms":0}
{"time":"2026-10-28T23:53:29.666984Z","event":"request","op":"GET /programs","status":200,"ms":0}
{"time":"2026-10-28T23:53:29.684818Z","event":"request","op":"GET /programs","status":401,"ms":0}
{"time":"2026-10-28T23:53:31.643789Z","event":"stopped"}
```
