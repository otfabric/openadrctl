# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly by
emailing security@otfabric.io. Do not open a public issue.

We will acknowledge receipt within 48 hours and aim to provide a fix within
7 days for critical issues.

## Scope

openadrctl handles OAuth2 client secrets, bearer tokens and MQTT passwords.
It reads them from the environment or from files, never from flags, so that
they stay out of process lists and shell history, and it never prints a
secret (`token` prints the token you asked for). `vtn start` is a VTN for
development and tests: its clients and secrets come from the seed file, and
`--insecure-webhooks`, `--anonymous` and plain http are for test networks.

## Output

What a VTN, token endpoint or broker sends is printed as text on one line:
control characters, bidirectional overrides and bytes that are not UTF-8 are
written as Go escapes (`\n`, `\x1b`, `\u202e`), in error messages, in the
`cause:` lines of `--debug`, and in the text records of `listen`,
`events watch`, `notifiers`, `topics` and `ping`. `--format json` keeps the
values as they were sent, JSON-encoded.

## Limits

- A search of the client commands (`list` without `--limit`, each search of
  `events watch`) keeps at most 1000000 objects, and at most 1 GiB of them as
  JSON, and by default at most 100000 objects (`--max-objects`; 0 for up to
  the bound). One past that ends the command with exit 6. An answer of the VTN is 16 MiB at most.
- `vtn start` gives a request head 5 seconds and 64 KiB, and a request a
  minute to arrive and be answered; it closes a connection without a request
  after two minutes. Stopping waits 5 seconds for the requests in progress,
  then closes their connections.

## MQTT logins by token

With `vtn start --mqtt-client-auth token`, `POST /auth/mqtt` tells the
broker whether a token, or the VTN's own publisher password, may log in.
Set `OPENADRCTL_VTN_MQTT_AUTH_SECRET`, and have the broker's HTTP
authenticator send it as `Authorization: Bearer <secret>`: the VTN then
answers only the broker, and any other request 401. Give the publisher a
long random password in `OPENADRCTL_VTN_MQTT_PASSWORD`. After 10 wrong
passwords within a minute for the publisher's user name, that user name is
answered 429 for a minute.
