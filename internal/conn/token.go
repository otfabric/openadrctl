// SPDX-License-Identifier: MIT

package conn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
)

// TokenSource obtains tokens with the OAuth2 client-credentials grant, as
// the clients of go-openadr do, and keeps the one it has until shortly
// before it expires. It is the client's token source, and what token and
// listen --via mqtt (for a broker that wants the token) read.
type TokenSource struct {
	Base         string
	HTTP         *http.Client
	ClientID     string
	ClientSecret string
	Scopes       []string
	// TokenURL is the token endpoint; discovered with GET /auth/server
	// when empty, or with OpenADR 3.0.1, which has no such operation,
	// /auth/token of the VTN.
	TokenURL string
	Version  openadr.Version

	mu      sync.Mutex
	current *Token
}

// Token is an access token as the token endpoint issued it.
type Token struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   float64   `json:"expires_in,omitempty"`
	Scope       string    `json:"scope,omitempty"`
	TokenURL    string    `json:"token_url"`
	Expires     time.Time `json:"expires,omitzero"`
}

// MarshalTextOutput prints the token alone, for $(openadrctl token).
func (t *Token) MarshalTextOutput() (string, error) { return t.AccessToken, nil }

// TableHeaders implements format.TableMarshaler.
func (t *Token) TableHeaders() []string { return []string{"TOKEN_URL", "TYPE", "EXPIRES_IN", "SCOPE"} }

// TableRows implements format.TableMarshaler.
func (t *Token) TableRows() [][]string {
	exp := "-"
	if t.ExpiresIn > 0 {
		exp = Lifetime(t.ExpiresIn).String()
	}
	return [][]string{{t.TokenURL, t.TokenType, exp, t.Scope}}
}

// expirySlack is how long before its expiry a token stops being used.
const expirySlack = 10 * time.Second

// maxTokenLifetime is the longest a token is taken to live, as go-openadr
// takes it: an expires_in beyond it, which a Duration may not even hold, is
// taken as this.
const maxTokenLifetime = 100 * 365 * 24 * time.Hour

// maxTokenResponse is the most of a token endpoint's answer that is read.
const maxTokenResponse = 1 << 20

// Lifetime is how long a token that expires in the given number of seconds
// lives, with an expires_in no Duration holds taken as 100 years.
func Lifetime(expiresIn float64) time.Duration {
	if expiresIn >= maxTokenLifetime.Seconds() {
		return maxTokenLifetime
	}
	return time.Duration(expiresIn * float64(time.Second)).Truncate(time.Second)
}

// validToken tells whether a token can go in an Authorization header and
// be printed: visible ASCII only (RFC 6750 b64token is a subset).
func validToken(t string) bool {
	for i := 0; i < len(t); i++ {
		if t[i] < 0x21 || t[i] > 0x7e {
			return false
		}
	}
	return t != ""
}

// client is the HTTP client of the token requests: one that follows a
// redirect only within the origin, so that the client secret does not
// follow a 307 elsewhere.
func (s *TokenSource) client() *http.Client {
	hc := s.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	if hc.CheckRedirect == nil {
		copied := *hc
		copied.CheckRedirect = SameOrigin
		hc = &copied
	}
	return hc
}

// Token implements client.TokenSource.
func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil && (s.current.Expires.IsZero() || time.Now().Before(s.current.Expires.Add(-expirySlack))) {
		return s.current.AccessToken, nil
	}
	t, err := s.fetch(ctx)
	if err != nil {
		return "", noToken{err}
	}
	s.current = t
	return t.AccessToken, nil
}

// ErrNoToken marks an error of Token: no token could be had, so the request
// it was for was not sent.
var ErrNoToken = errors.New("no token")

type noToken struct{ err error }

func (e noToken) Error() string   { return e.err.Error() }
func (e noToken) Unwrap() []error { return []error{ErrNoToken, e.err} }

// Invalidate drops the token the VTN rejected.
func (s *TokenSource) Invalidate(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil && s.current.AccessToken == token {
		s.current = nil
	}
}

// Fetch asks the token endpoint for a new token.
func (s *TokenSource) Fetch(ctx context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.fetch(ctx)
	if err == nil {
		s.current = t
	}
	return t, err
}

func (s *TokenSource) fetch(ctx context.Context) (*Token, error) {
	if s.TokenURL == "" && s.Version == openadr.Version301 {
		s.TokenURL = s.Base + "/auth/token"
	}
	if s.TokenURL == "" {
		u, err := s.discover(ctx)
		if err != nil {
			return nil, err
		}
		s.TokenURL = u
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {s.ClientID}, "client_secret": {s.ClientSecret}}
	if len(s.Scopes) > 0 {
		form.Set("scope", strings.Join(s.Scopes, " "))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer drain(resp.Body)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxTokenResponse {
		return nil, fmt.Errorf("%w: the token endpoint answered with more than %d bytes", openadr.ErrProtocol, maxTokenResponse)
	}
	var r struct {
		AccessToken      string          `json:"access_token"`
		TokenType        string          `json:"token_type"`
		ExpiresIn        json.RawMessage `json:"expires_in"`
		Scope            string          `json:"scope"`
		Error            string          `json:"error"`
		ErrorDescription string          `json:"error_description"`
	}
	// The body of an error response is not always JSON; the status decides.
	decodeErr := json.Unmarshal(body, &r)
	if resp.StatusCode != http.StatusOK {
		return nil, &client.TokenError{StatusCode: resp.StatusCode, Code: r.Error, Description: r.ErrorDescription}
	}
	var expires float64
	if decodeErr == nil {
		expires, decodeErr = expiresIn(r.ExpiresIn)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("%w: the token endpoint answered 200 with what is not a token response: %v", openadr.ErrProtocol, decodeErr)
	}
	if r.AccessToken == "" {
		return nil, fmt.Errorf("%w: the token endpoint answered 200 without an access_token", openadr.ErrProtocol)
	}
	if !strings.EqualFold(r.TokenType, "Bearer") {
		return nil, fmt.Errorf("%w: the token endpoint issued a %q token, not a bearer token", openadr.ErrProtocol, r.TokenType)
	}
	if !validToken(r.AccessToken) {
		// Neither sent in a header nor printed: it could forge either.
		return nil, fmt.Errorf("%w: the token endpoint issued an access_token with characters a bearer token does not have", openadr.ErrProtocol)
	}
	t := Token{AccessToken: r.AccessToken, TokenType: r.TokenType, Scope: r.Scope, TokenURL: redacted(s.TokenURL)}
	if expires > 0 {
		t.ExpiresIn = expires
		t.Expires = time.Now().Add(Lifetime(expires)).UTC().Truncate(time.Second)
	}
	return &t, nil
}

// expiresIn reads the expires_in of a token response: a number of seconds
// (RFC 6749 section 5.1), or a string of digits, as some identity providers
// send it, as go-openadr's client reads it. None, null, and a number that is
// not positive are 0: no expiry known. A number no float64 holds is taken as
// the longest lifetime.
func expiresIn(raw json.RawMessage) (float64, error) {
	s := string(raw)
	switch {
	case len(raw) == 0 || s == "null":
		return 0, nil
	case raw[0] == '"':
		if err := json.Unmarshal(raw, &s); err != nil {
			return 0, err
		}
		if s == "" || strings.Trim(s, "0123456789") != "" {
			return 0, fmt.Errorf("expires_in %q is not a number of seconds", clip(s))
		}
	case raw[0] != '-' && (raw[0] < '0' || raw[0] > '9'):
		return 0, fmt.Errorf("expires_in %s is not a number of seconds", clip(s))
	}
	// A JSON number or digits: only the range can fail.
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("expires_in %q is not a number of seconds", clip(s))
	}
	if math.IsInf(f, 1) {
		return maxTokenLifetime.Seconds(), nil
	}
	return max(f, 0), nil
}

// clip shortens a value quoted in a message to 32 bytes.
func clip(s string) string {
	const n = 32
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// discover asks the VTN for its token endpoint.
func (s *TokenSource) discover(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Base+"/auth/server", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp.Body)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponse))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &openadr.Error{Method: http.MethodGet, Path: "/auth/server", StatusCode: resp.StatusCode}
		var p struct{ Title, Detail, Type string }
		if json.Unmarshal(body, &p) == nil {
			apiErr.Problem = openadr.Problem{Title: p.Title, Detail: p.Detail, Type: p.Type, Status: resp.StatusCode}
		}
		return "", fmt.Errorf("discovering the token endpoint (give --token-url to skip it): %w", apiErr)
	}
	var info struct {
		TokenURL string `json:"tokenURL"`
	}
	if err := json.Unmarshal(body, &info); err != nil || info.TokenURL == "" {
		return "", fmt.Errorf("%w: GET /auth/server answered without a tokenURL", openadr.ErrProtocol)
	}
	base, _ := url.Parse(s.Base + "/")
	ref, err := url.Parse(info.TokenURL)
	if err != nil {
		return "", fmt.Errorf("%w: GET /auth/server named %q", openadr.ErrProtocol, info.TokenURL)
	}
	resolved := base.ResolveReference(ref)
	if (resolved.Scheme != "https" && resolved.Scheme != "http") || resolved.Host == "" {
		return "", fmt.Errorf("%w: the VTN named the token endpoint %q, which is not an http or https URL", openadr.ErrProtocol, info.TokenURL)
	}
	// The secret goes there: not in the clear when the VTN is reached over TLS.
	if base.Scheme == "https" && resolved.Scheme != "https" {
		return "", fmt.Errorf("%w: the VTN named the token endpoint %q, which is not https", openadr.ErrProtocol, info.TokenURL)
	}
	return resolved.String(), nil
}

// redacted is a URL with the password of its user information, if any,
// replaced by xxxxx: what may be printed of it.
func redacted(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil {
		return s
	}
	return u.Redacted()
}
