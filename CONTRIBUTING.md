# Contributing

Thank you for your interest in contributing to openadrctl.

## Development

```bash
# Run tests (with the race detector)
make test

# Run linter
make lint

# Build (runs the checks first; writes bin/openadrctl)
make build

# Run all checks: format, vet, lint, tests, coverage (minimum 85%)
make check

# Against the OpenLEADR VTN of openadr-interop (needs Docker)
scripts/interop.sh
```

The tests run the real command tree (`cmd.Run`) against `openadrctl vtn
start` on a free port, with an MQTT broker in the process; they need no
network. A test that waits for something waits for the record that says it
happened, never for a fixed time.

## Pull Requests

1. Fork the repository.
2. Create a feature branch from `main`.
3. Make your changes with tests.
4. Run `make check` to verify everything passes.
5. Open a pull request against `main`.

## Code Style

- Follow standard Go conventions.
- Run `make fmt` before committing.
- Every Go file starts with `// SPDX-License-Identifier: MIT`.
- All exported symbols must have doc comments.
- Exit codes and JSON field names are a contract
  ([docs/exitcodes.md](docs/exitcodes.md), [docs/json-output.md](docs/json-output.md)):
  a change to them is called out in [RELEASE.md](RELEASE.md).
- Documentation shows real output: run the command, and paste what it printed.
