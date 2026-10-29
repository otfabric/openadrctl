// SPDX-License-Identifier: MIT

package errs

// Stable machine-oriented codes for JSON error.code and scripts (docs/exitcodes.md).
const (
	CodeInvalidFormat          = "invalid_format"
	CodeInvalidInput           = "invalid_input"
	CodeInvalidURL             = "invalid_url"
	CodeInvalidFlagCombination = "invalid_flag_combination"
	CodeInvalidConfig          = "invalid_config"
	CodeInvalidCertificate     = "invalid_certificate"
	// An object does not satisfy the OpenADR 3.1.0 schema: nothing was sent.
	CodeInvalidObject = "invalid_object"
	// A command that changes the VTN was not confirmed with --yes.
	CodeConfirmationRequired = "confirmation_required"

	CodeTransportConnectFailed = "transport_connect_failed"
	// The TLS handshake failed: a certificate either end refused.
	CodeTransportTLSFailed = "transport_tls_failed"
	CodeConnectionLost     = "connection_lost"
	CodeListenFailed       = "listen_failed"

	// No answer in time to a request that does not change state.
	CodeRequestTimeout = "request_timeout"
	// No answer to a request that may have changed state: it is not resent.
	CodeOutcomeUnknown = "outcome_unknown"

	// The VTN answered with an HTTP error: the openadr object has the status
	// and the VTN's problem object.
	CodeBadRequest      = "bad_request"
	CodeUnauthorized    = "unauthorized"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeTooManyRequests = "too_many_requests"
	CodeNotImplemented  = "not_implemented"
	CodeUnavailable     = "unavailable"
	CodeServerError     = "server_error"
	CodeHTTPError       = "http_error"
	// The token endpoint refused the client's credentials.
	CodeTokenRefused = "token_refused"

	// The VTN's answer breaks the schema or the protocol.
	CodeInvalidResponse = "invalid_response"

	CodeContextCanceled = "context_canceled"

	CodeJSONEncodeFailed       = "json_encode_failed"
	CodeTableRenderFailed      = "table_render_failed"
	CodeOutputWriteFailed      = "output_write_failed"
	CodeOutputFileWriteFailed  = "output_file_write_failed"
	CodeOutputFileCreateFailed = "output_file_create_failed"
	CodeInputFileOpenFailed    = "input_file_open_failed"
	CodeInputDecodeFailed      = "input_decode_failed"

	CodeInternalUnexpected = "internal_unexpected"
)
