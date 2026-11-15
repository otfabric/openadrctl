// SPDX-License-Identifier: MIT

package conn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	openadr "github.com/otfabric/go-openadr"
)

// The expires_in of a token response: a number or a string of digits gives a
// token; anything else is refused as not a token response, which is not
// the message of a missing access_token.
func TestExpiresIn(t *testing.T) {
	const hundredYears = 100 * 365 * 24 * time.Hour
	for _, c := range []struct {
		name, expiresIn string // the JSON of expires_in; "" for none
		life            time.Duration
		refused         string // in the message of a refusal
	}{
		{name: "number", expiresIn: `3600`, life: time.Hour},
		{name: "fraction", expiresIn: `90.5`, life: 90 * time.Second},
		{name: "exponent", expiresIn: `1e3`, life: 1000 * time.Second},
		{name: "digits", expiresIn: `"3600"`, life: time.Hour},
		{name: "zeros first", expiresIn: `"0060"`, life: time.Minute},
		{name: "none", expiresIn: ``},
		{name: "null", expiresIn: `null`},
		{name: "zero", expiresIn: `0`},
		{name: "negative", expiresIn: `-5`},
		{name: "zero digits", expiresIn: `"0"`},
		{name: "huge", expiresIn: `1e300`, life: hundredYears},
		{name: "beyond float64", expiresIn: `1e400`, life: hundredYears},
		{name: "huge digits", expiresIn: `"` + strings.Repeat("9", 400) + `"`, life: hundredYears},
		{name: "empty string", expiresIn: `""`, refused: `expires_in "" is not a number of seconds`},
		{name: "exponent string", expiresIn: `"1e3"`, refused: `expires_in "1e3" is not a number of seconds`},
		{name: "negative string", expiresIn: `"-5"`, refused: `expires_in "-5" is not a number of seconds`},
		{name: "space", expiresIn: `"3600 "`, refused: `is not a number of seconds`},
		{name: "long string", expiresIn: `"` + strings.Repeat("x", 100) + `"`, refused: `expires_in "` + strings.Repeat("x", 32) + `..."`},
		{name: "boolean", expiresIn: `true`, refused: `expires_in true is not a number of seconds`},
		{name: "object", expiresIn: `{}`, refused: `expires_in {} is not a number of seconds`},
		{name: "malformed JSON", expiresIn: `}`, refused: "invalid character"},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := `{"access_token":"a","token_type":"Bearer"`
			if c.expiresIn != "" {
				body += `,"expires_in":` + c.expiresIn
			}
			body += `}`
			src, _ := tokenServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
			before := time.Now()
			tok, err := src.Fetch(context.Background())
			if c.refused != "" {
				if !errors.Is(err, openadr.ErrProtocol) || !strings.Contains(err.Error(), "not a token response") ||
					!strings.Contains(err.Error(), c.refused) || strings.Contains(err.Error(), "without an access_token") {
					t.Fatalf("%s: %+v, %v", body, tok, err)
				}
				return
			}
			if err != nil || tok.AccessToken != "a" {
				t.Fatalf("%s: %+v, %v", body, tok, err)
			}
			if c.life == 0 {
				if tok.ExpiresIn != 0 || !tok.Expires.IsZero() {
					t.Errorf("%s: no expiry known, got %v, %v", body, tok.ExpiresIn, tok.Expires)
				}
				return
			}
			// Expires is in whole seconds, and the clock may be coarse.
			if lo, hi := before.Add(c.life).Add(-2*time.Second), time.Now().Add(c.life).Add(time.Second); tok.Expires.Before(lo) || tok.Expires.After(hi) {
				t.Errorf("%s: expires %v, want about %v from now", body, tok.Expires, c.life)
			}
			if Lifetime(tok.ExpiresIn) != c.life.Truncate(time.Second) {
				t.Errorf("%s: expires_in %v", body, tok.ExpiresIn)
			}
			// The token prints, as token --format json does.
			if _, err := json.Marshal(tok); err != nil {
				t.Errorf("%s: %v", body, err)
			}
		})
	}
}
