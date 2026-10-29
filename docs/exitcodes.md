# Process exit codes

Every command of openadrctl exits with one of these codes. They are a
**stable contract** for scripts: a change is called out in the release notes.
They are the codes of the other otfabric tools (ocppctl, ieee20305ctl,
modbusctl, …).

| Code | Kind (`error.kind`) | Meaning |
|------|---------------------|---------|
| 0 | | Success |
| 2 | `usage` | The command line is wrong: an unknown command or flag, a missing argument, a change that needs `--yes` without it |
| 3 | `invalid_input` | Nothing was sent: a flag, file, certificate, seed or object is invalid; `validate` found an invalid object |
| 4 | `transport` | No conversation with the peer: the connection was refused, the TLS handshake failed, a port could not be opened, a WebSocket or MQTT broker could not be reached |
| 5 | `timeout` | No answer in `--timeout` |
| 6 | `refused` | The VTN answered with an HTTP error (4xx or 5xx), or its token endpoint refused the client; the `openadr` object has the status and the VTN's problem object |
| 6 | `protocol` | The VTN broke the protocol: an answer that does not decode as OpenADR 3.1.0, a token endpoint that issues no bearer token |
| 10 | `output`, `internal` | The result could not be written, or openadrctl failed; also an interruption (Ctrl-C) before a one-shot command has its result |

`refused` and `protocol` both exit 6: the VTN was reached and talked to, and
the answer is not what the command needed.

## The error object

With `--format json` a failure is one JSON object on stdout, and nothing
else:

```json
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

With `--format text` or `table` it is one line on stderr: `error: <message>`.
`--debug` adds the causes of the error, one `cause:` line each.

| Field | Meaning |
|-------|---------|
| `error.kind` | One of the kinds above |
| `error.code` | A stable snake_case code; see below |
| `error.message` | For people; it may change between versions |
| `openadr` | Protocol detail, when there is some (omitted otherwise) |
| `openadr.method`, `openadr.path`, `openadr.status` | The request the VTN refused, and its HTTP status |
| `openadr.problem` | The VTN's problem object (RFC 7807): `type`, `title`, `status`, `detail`, `instance`, as far as it gave them |
| `openadr.retry_after` | The wait a 429 or 503 asked for, as a Go duration (`1s`) |
| `openadr.oauth_error` | The OAuth2 error of a refused token request (`invalid_client`, …) |
| `openadr.field` | The property of an invalid object (`programID`, `intervals[0].payloads`) |

## Codes

| Kind | Codes |
|------|-------|
| `usage` | `invalid_input` (what the command-line parser rejects), `confirmation_required` (`events create`, `events update` and every `delete` without `--yes`) |
| `invalid_input` | `invalid_input`, `invalid_format` (`--format`), `invalid_url` (`--url`, `--public-url`, `--callback-url`, `--broker`, `--mqtt`), `invalid_flag_combination` (`--value` and `--csv`, `--tls-cert` without `--tls-key`, `--callback-url` and `--websocket`, …), `invalid_config` (a seed that does not load or validate, a seed without clients and without `--anonymous`, a `--store` that does not open), `invalid_object` (an object that breaks the OpenADR 3.1.0 schema: `validate`, and every create and update before it sends), `invalid_certificate` (a certificate, key or CA file that does not load), `input_file_open_failed`, `input_decode_failed` (a file that is not JSON, a property OpenADR 3.1.0 does not have, a CSV that does not parse), `output_file_create_failed` (`vtn generate-config --output` of a file that exists, without `--force`) |
| `transport` | `transport_connect_failed` (no TCP connection, a broker that never accepted the subscription), `transport_tls_failed` (a certificate either end refused), `connection_lost` (a WebSocket that did not open), `listen_failed` (`vtn start --listen`, `listen --listen`) |
| `timeout` | `request_timeout` (no answer to a request that changes nothing, or one that was never sent because no token could be had), `outcome_unknown` (no answer to a request that may have changed the VTN: it was **not** sent again, also with `--retries`; check before you resend) |
| `refused` | `bad_request` (400), `unauthorized` (401), `forbidden` (403), `not_found` (404), `conflict` (409), `too_many_requests` (429), `not_implemented` (501, or no MQTT broker announced), `unavailable` (503), `server_error` (other 5xx), `http_error` (any other status), `token_refused` (the token endpoint refused the credentials) |
| `protocol` | `invalid_response` |
| `output` | `json_encode_failed`, `table_render_failed`, `output_write_failed`, `output_file_write_failed` |
| `internal` | `context_canceled` (interrupted before a result), `internal_unexpected` |

## Streaming commands

`listen`, `events watch`, `vtn start` and `discover` write one record per
event. Interrupting them (Ctrl-C, SIGTERM), `--count` or the end of
`--duration` ends the stream normally: exit 0.

- `listen` exits with the error's code when it cannot start: the
  subscription is refused (6), the WebSocket or the broker cannot be reached
  (4). A `--duration` that ends before the WebSocket or the broker is ready
  is a failure (4), not an end.
- `events watch` exits with the error's code when a search fails.
- `vtn start` exits 3 when its seed, certificates or flags are invalid
  (every problem of a seed is reported at once), and 4 when it cannot listen.
- `discover` exits 4 when it cannot use multicast; finding no VTN is a
  result (`no VTN answered`), exit 0.

## `ping`

`ping` writes its result, every step with its status, also when a step
fails, and exits with the code of the failure: 4 when the VTN cannot be
reached or the TLS handshake fails, 6 when the token endpoint or the API
refuses, 5 when a step times out. A VTN that answers `GET /notifiers` with
501 has no notifiers; that is not a failure.

## `validate`

A valid object exits 0. An invalid one exits 3 with `invalid_object` and the
property in `openadr.field`; a file that is not JSON, or has a property
OpenADR 3.1.0 does not have, exits 3 with `input_decode_failed`.

## Commands that change the VTN

Creates, updates and deletes send their request once, unless `--retries`
allows another attempt for what one can cure (no answer, 429,
502–504). A create (POST) that got no answer is never sent again: it exits 5
with `outcome_unknown`, and the VTN may have created the object. List before
you resend. An update or delete is the same each time, and is retried.

Events need `--yes` to be created or changed, and every object needs it to
be deleted: exit 2 with `confirmation_required` otherwise. `--dry-run` shows
the object a create or update would send, and sends nothing.
