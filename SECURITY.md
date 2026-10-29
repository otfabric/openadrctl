# Security Policy

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
