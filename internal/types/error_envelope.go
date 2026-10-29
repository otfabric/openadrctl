// SPDX-License-Identifier: MIT

package types

// ErrorInfo is the structured error object for JSON output and embedded per-row failures.
type ErrorInfo struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorEnvelope is the top-level JSON shape for fatal errors (--format json).
type ErrorEnvelope struct {
	Error ErrorInfo `json:"error"`
	// OpenADR is protocol detail: see docs/json-output.md.
	OpenADR any `json:"openadr,omitempty"`
}
