// SPDX-License-Identifier: MIT

package errs

// Kind is a coarse failure category for exit-code mapping and JSON error.kind.
type Kind string

const (
	KindUsage        Kind = "usage"
	KindInvalidInput Kind = "invalid_input"
	KindTransport    Kind = "transport"
	KindTimeout      Kind = "timeout"
	KindProtocol     Kind = "protocol"
	// KindNegative is a request the server answered and refused: a 4xx or 5xx.
	KindNegative Kind = "refused"
	KindOutput   Kind = "output"
	KindInternal Kind = "internal"
)
