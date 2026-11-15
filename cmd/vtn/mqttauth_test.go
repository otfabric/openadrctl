// SPDX-License-Identifier: MIT

package vtn

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// With OPENADRCTL_VTN_MQTT_AUTH_SECRET only a request with the broker's
// secret is answered; the others are answered 401, and are summed up in one
// record instead of one each.
func TestMQTTAuthWithTheBrokersSecret(t *testing.T) {
	t.Setenv("OPENADRCTL_VTN_MQTT_PASSWORD", "publisher-password")
	t.Setenv("OPENADRCTL_VTN_MQTT_AUTH_SECRET", "broker-secret")
	v := startServer(t, "--mqtt", "mqtt://127.0.0.1:1", "--mqtt-client-auth", "token", "--mqtt-username", "vtn")
	hc := httpClient(t)
	post := func(authorization, password string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, v.url+"/auth/mqtt", strings.NewReader(`{"username":"vtn","password":"`+password+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for range 50 {
		if got := post("", "publisher-password"); got != http.StatusUnauthorized {
			t.Fatalf("without the secret: %d", got)
		}
	}
	for _, a := range []string{"Bearer wrong", "Basic broker-secret", "Bearer", "broker-secret", "Bearer broker-secret2"} {
		if got := post(a, "publisher-password"); got != http.StatusUnauthorized {
			t.Errorf("Authorization %q: %d", a, got)
		}
	}
	if got := post("Bearer broker-secret", "publisher-password"); got != http.StatusOK {
		t.Errorf("the broker, the publisher's password: %d", got)
	}
	if got := post("bearer broker-secret", "wrong"); got != http.StatusForbidden {
		t.Errorf("the broker, a wrong password: %d", got)
	}
	out := v.out.String()
	if n := strings.Count(out, `"event":"mqtt_login"`); n != 2 {
		t.Errorf("%d mqtt_login records, want 2:\n%s", n, out)
	}
	if n := strings.Count(out, "without the broker's secret"); n != 1 {
		t.Errorf("%d records of the requests without the secret, want 1:\n%s", n, out)
	}
}

func TestRefusalsAreSummedUp(t *testing.T) {
	var q refusals
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after time.Duration
		n     int
		ok    bool
	}{
		{0, 1, true},
		{time.Second, 0, false},
		{59 * time.Second, 0, false},
		{time.Minute, 3, true}, // the two before and this one
		{time.Minute + time.Second, 0, false},
		{3 * time.Minute, 2, true},
	} {
		if n, ok := q.note(at.Add(c.after)); n != c.n || ok != c.ok {
			t.Errorf("after %s: %d %v, want %d %v", c.after, n, ok, c.n, c.ok)
		}
	}
}

func TestBrokersSecretNeedsTokenLogins(t *testing.T) {
	for _, mode := range []string{"", "anonymous"} {
		c := startConfig{Listen: "127.0.0.1:0", TokenTTL: time.Hour, MQTT: "mqtt://127.0.0.1:1", MQTTClientAuth: mode, MQTTAuthSecret: "s"}
		if err := c.validate(); err == nil {
			t.Errorf("--mqtt-client-auth %q with a secret: no error", mode)
		}
	}
	c := startConfig{Listen: "127.0.0.1:0", TokenTTL: time.Hour, MQTT: "mqtt://127.0.0.1:1", MQTTClientAuth: "token", MQTTAuthSecret: "s"}
	if err := c.validate(); err != nil {
		t.Error(err)
	}
}

// Wrong passwords for the publisher's user name: after the tenth, that user
// name is answered 429 for a minute, also with the right password, and the
// clients' tokens are not affected.
func TestPublisherLockout(t *testing.T) {
	t.Setenv("OPENADRCTL_VTN_MQTT_PASSWORD", "publisher-password")
	v := startServer(t, "--mqtt", "mqtt://127.0.0.1:1", "--mqtt-client-auth", "token", "--mqtt-username", "vtn")
	hc := httpClient(t)
	post := func(username, password string) *http.Response {
		t.Helper()
		resp, err := hc.Post(v.url+"/auth/mqtt", "application/json", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	for i := range maxPublisherFailures {
		if got := post("vtn", "wrong").StatusCode; got != http.StatusForbidden {
			t.Fatalf("wrong password %d: %d", i+1, got)
		}
	}
	for range 3 {
		resp := post("vtn", "publisher-password")
		if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
			t.Fatalf("after %d wrong passwords, the right one: %d, Retry-After %q", maxPublisherFailures, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	}
	tok, err := client(t, v.url, "site-42", "site-42-secret").Tokens.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := post("site-42", tok).StatusCode; got != http.StatusOK {
		t.Errorf("a client's token meanwhile: %d", got)
	}
	out := v.out.String()
	if n := strings.Count(out, `"event":"mqtt_login","client":"vtn"`); n != maxPublisherFailures {
		t.Errorf("%d mqtt_login records of vtn, want %d:\n%s", n, maxPublisherFailures, out)
	}
	if n := strings.Count(out, "answered 429"); n != 1 {
		t.Errorf("%d records of the 429 answers, want 1:\n%s", n, out)
	}
}

func TestLockoutWindow(t *testing.T) {
	var l lockout
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Nine in a minute, then the minute is over: they no longer count.
	for i := range maxPublisherFailures - 1 {
		l.failed(at.Add(time.Duration(i) * time.Second))
	}
	l.failed(at.Add(publisherLockout))
	if _, locked := l.locked(at.Add(publisherLockout)); locked {
		t.Fatal("locked by failures more than a window apart")
	}
	// Ten within a minute: locked for a minute from the tenth.
	start := at.Add(2 * publisherLockout)
	for i := range maxPublisherFailures {
		l.failed(start.Add(time.Duration(i) * time.Second))
	}
	last := start.Add(time.Duration(maxPublisherFailures-1) * time.Second)
	if wait, locked := l.locked(last); !locked || wait != publisherLockout {
		t.Errorf("after ten: %v %v", wait, locked)
	}
	if _, locked := l.locked(last.Add(publisherLockout - time.Nanosecond)); !locked {
		t.Error("unlocked before the minute is over")
	}
	if _, locked := l.locked(last.Add(publisherLockout)); locked {
		t.Error("still locked after the minute")
	}
}
