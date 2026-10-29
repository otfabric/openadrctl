// SPDX-License-Identifier: MIT

package conn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"

	"github.com/otfabric/openadrctl/internal/errs"
)

// HTTPDetail is the openadr object of a JSON error for an answer of a VTN:
// see docs/json-output.md.
type HTTPDetail struct {
	Method     string   `json:"method,omitempty"`
	Path       string   `json:"path,omitempty"`
	Status     int      `json:"status,omitempty"`
	Problem    *Problem `json:"problem,omitempty"`
	RetryAfter string   `json:"retry_after,omitempty"`
	// OAuthError is the RFC 6749 error code of a token endpoint.
	OAuthError string `json:"oauth_error,omitempty"`
	// Field is the property a validation failed on.
	Field string `json:"field,omitempty"`
}

// Problem is the problem object (RFC 9457) a VTN answered with.
type Problem struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// Classify maps what a call to a VTN returned to a typed error of the tool.
// write says whether the call may have changed the VTN, which turns a
// timeout into an unknown outcome.
func Classify(err error, write bool) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.As(err); ok {
		return err
	}
	if errors.Is(err, ErrNoToken) {
		// The request was not sent: a timeout leaves nothing unknown.
		write = false
	}
	var apiErr *openadr.Error
	var tokErr *client.TokenError
	var valErr *openadr.ValidationError
	switch {
	case errors.Is(err, context.Canceled):
		return errs.New(errs.KindInternal, errs.CodeContextCanceled, "interrupted", err)
	case errors.As(err, &tokErr):
		d := HTTPDetail{Status: tokErr.StatusCode, OAuthError: tokErr.Code}
		code := errs.CodeTokenRefused
		switch {
		case errors.Is(err, openadr.ErrTooManyRequests):
			code = errs.CodeTooManyRequests
		case errors.Is(err, openadr.ErrServer):
			code = errs.CodeServerError
		}
		return errs.New(errs.KindNegative, code, "the token endpoint refused the client: "+strings.TrimPrefix(tokErr.Error(), "openadr: "), err).WithDetail(d)
	case errors.As(err, &apiErr):
		return apiError(err, apiErr)
	case errors.Is(err, openadr.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		if write {
			return errs.New(errs.KindTimeout, errs.CodeOutcomeUnknown, "no answer in time; the VTN may have carried out the request, which is not sent again: "+short(err), err)
		}
		return errs.New(errs.KindTimeout, errs.CodeRequestTimeout, "no answer in time: "+short(err), err)
	case errors.Is(err, openadr.ErrProtocol):
		return errs.New(errs.KindProtocol, errs.CodeInvalidResponse, short(err), err)
	case errors.As(err, &valErr):
		return errs.InvalidInput(errs.CodeInvalidObject, short(err)+" (nothing was sent)", err).WithDetail(HTTPDetail{Field: valErr.Field})
	case errors.Is(err, openadr.ErrConfiguration):
		return errs.InvalidInput(errs.CodeInvalidConfig, short(err), err)
	case errors.Is(err, errors.ErrUnsupported):
		return errs.InvalidInput(errs.CodeNotInRevision, short(err)+" (nothing was sent)", err)
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) ||
		strings.Contains(err.Error(), "tls: ") || strings.Contains(err.Error(), "x509: ") {
		return errs.New(errs.KindTransport, errs.CodeTransportTLSFailed, "the TLS handshake failed: "+short(err)+" (--ca names the CA of the VTN)", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Classify(fmt.Errorf("%w: %w", openadr.ErrTimeout, err), write)
	}
	return errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, short(err), err)
}

func apiError(err error, e *openadr.Error) error {
	d := HTTPDetail{Method: e.Method, Path: e.Path, Status: e.StatusCode}
	if e.Problem != (openadr.Problem{}) {
		p := Problem(e.Problem)
		d.Problem = &p
	}
	if e.RetryAfter > 0 {
		d.RetryAfter = e.RetryAfter.String()
	}
	code := errs.CodeHTTPError
	switch {
	case errors.Is(err, openadr.ErrBadRequest):
		code = errs.CodeBadRequest
	case errors.Is(err, openadr.ErrUnauthorized):
		code = errs.CodeUnauthorized
	case errors.Is(err, openadr.ErrForbidden):
		code = errs.CodeForbidden
	case errors.Is(err, openadr.ErrNotFound):
		code = errs.CodeNotFound
	case errors.Is(err, openadr.ErrConflict):
		code = errs.CodeConflict
	case errors.Is(err, openadr.ErrTooManyRequests):
		code = errs.CodeTooManyRequests
	case errors.Is(err, openadr.ErrNotImplemented):
		code = errs.CodeNotImplemented
	case errors.Is(err, openadr.ErrUnavailable):
		code = errs.CodeUnavailable
	case errors.Is(err, openadr.ErrServer):
		code = errs.CodeServerError
	}
	msg := fmt.Sprintf("%s %s: the VTN answered %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	if e.Problem.Detail != "" {
		msg += ": " + e.Problem.Detail
	} else if e.Problem.Title != "" && e.Problem.Title != http.StatusText(e.StatusCode) {
		msg += ": " + e.Problem.Title
	}
	switch code {
	case errs.CodeUnauthorized:
		msg += " (give --client-id and a secret, or a token)"
	case errs.CodeNotImplemented:
		msg += " (the VTN does not offer this)"
	}
	return errs.New(errs.KindNegative, code, msg, err).WithDetail(d)
}

// short is the text of err without the package prefix.
func short(err error) string { return strings.TrimPrefix(err.Error(), "openadr: ") }
