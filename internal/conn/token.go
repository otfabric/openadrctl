// SPDX-License-Identifier: MIT

package conn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

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
		exp = (time.Duration(t.ExpiresIn) * time.Second).String()
	}
	return [][]string{{t.TokenURL, t.TokenType, exp, t.Scope}}
}

// expirySlack is how long before its expiry a token stops being used.
const expirySlack = 10 * time.Second

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
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer drain(resp.Body)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var r struct {
		Token
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	decodeErr := json.Unmarshal(body, &r)
	if resp.StatusCode != http.StatusOK {
		return nil, &client.TokenError{StatusCode: resp.StatusCode, Code: r.Error, Description: r.ErrorDescription}
	}
	if decodeErr != nil || r.AccessToken == "" {
		return nil, fmt.Errorf("%w: the token endpoint answered 200 without an access_token", openadr.ErrProtocol)
	}
	if !strings.EqualFold(r.TokenType, "Bearer") {
		return nil, fmt.Errorf("%w: the token endpoint issued a %q token, not a bearer token", openadr.ErrProtocol, r.TokenType)
	}
	t := r.Token
	t.TokenURL = s.TokenURL
	if t.ExpiresIn > 0 {
		t.Expires = time.Now().Add(time.Duration(t.ExpiresIn * float64(time.Second))).UTC().Truncate(time.Second)
	}
	return &t, nil
}

// discover asks the VTN for its token endpoint.
func (s *TokenSource) discover(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Base+"/auth/server", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp.Body)
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	// The secret goes there: not in the clear when the VTN is reached over TLS.
	if base.Scheme == "https" && resolved.Scheme != "https" {
		return "", fmt.Errorf("%w: the VTN named the token endpoint %q, which is not https", openadr.ErrProtocol, info.TokenURL)
	}
	return resolved.String(), nil
}
