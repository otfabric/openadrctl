// SPDX-License-Identifier: MIT

package cmd

import (
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// tokenVTN is a VTN whose clients log in to the broker with their token, as
// in TestMQTTToken, and the hook of that broker.
func tokenVTN(t *testing.T, args ...string) (*testVTN, string, *tokenHook) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	broker, hook := startTokenBroker(t)
	hook.url.Store("http://" + addr + "/auth/mqtt")
	t.Setenv("OPENADRCTL_VTN_MQTT_PASSWORD", "vtn-secret")
	v := startVTN(t, append([]string{"--listen", addr, "--mqtt", broker, "--mqtt-client-auth", "token", "--mqtt-username", "vtn"}, args...)...)
	return v, broker, hook
}

// waitForAfter waits until b holds first and, after it, then.
func waitForAfter(t *testing.T, b *syncBuffer, first, then string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		s := b.String()
		if i := strings.Index(s, first); i >= 0 && strings.Contains(s[i+len(first):], then) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %q after %q in:\n%s", then, first, s)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// After a connection to the broker was lost, listen logs in again with a new
// token when the broker refuses the one it had, as it does once that token
// has expired, and receives what comes after.
func TestListenMQTTRenewsARefusedToken(t *testing.T) {
	v, broker, hook := tokenVTN(t)
	l := start(t, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--mqtt-client-id", "cli-renew", "--count", "1", "--format", "json")...)
	waitFor(t, l.err, "listening: "+broker+", ")
	first := hook.passwords("cli-renew")
	if len(first) != 1 {
		t.Fatalf("logins before the drop: %d", len(first))
	}
	hook.deny.Store(func(_, password string) bool { return password == first[0] })
	hook.drop(t, "cli-renew")
	waitForAfter(t, l.err, "refused the token, logging in with a new one", "listening: "+broker+" again")
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "after-the-drop", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := l.wait(t); code != 0 {
		t.Fatalf("listen: exit %d\n%s", code, l.err)
	}
	if recs := lines(t, l.out.String()); len(recs) != 1 || recs[0]["name"] != "after-the-drop" {
		t.Errorf("records: %v", recs)
	}
	stderr := l.err.String()
	for _, want := range []string{
		"warning: mqtt: connection to the broker lost",
		"warning: mqtt: cannot connect to the broker: mqtt: the broker refused the connection",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("no %q on stderr:\n%s", want, stderr)
		}
	}
	if p := hook.passwords("cli-renew"); p[len(p)-1] == first[0] {
		t.Errorf("the last login used the first token: %d logins", len(p))
	}
}

// A token near its expiry when the connection is lost is renewed before the
// login: the broker is not first given a token about to expire.
func TestListenMQTTRenewsATokenNearItsExpiry(t *testing.T) {
	// Tokens shorter than the margin before expiry are renewed at each use.
	v, broker, hook := tokenVTN(t, "--token-ttl", "5s")
	l := start(t, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--mqtt-client-id", "cli-expiry", "--count", "1", "--format", "json")...)
	waitFor(t, l.err, "listening: "+broker+", ")
	hook.drop(t, "cli-expiry")
	waitForAfter(t, l.err, "reconnecting: "+broker+" with a new token", "listening: "+broker+" again")
	p := hook.passwords("cli-expiry")
	if len(p) < 2 || p[len(p)-1] == p[0] {
		t.Errorf("logins: %d, the last with the first token: %v", len(p), len(p) > 1 && p[len(p)-1] == p[0])
	}
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "renewed", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := l.wait(t); code != 0 {
		t.Fatalf("listen: exit %d\n%s", code, l.err)
	}
}

// A login the broker refuses after a reconnect ends listen with exit 4 and
// says so, also when a new token is refused too: listen does not stay
// without a connection.
func TestListenMQTTEndsWhenTheLoginIsRefused(t *testing.T) {
	v, broker, hook := tokenVTN(t)
	for _, c := range []struct {
		id, want string
		env      []string
	}{
		{"cli-token", "refused the login again after a reconnect, also with a new token", nil},
		// The VTN's own password: nothing to renew.
		{"cli-password", "refused the login again after a reconnect: no longer subscribed", []string{"--mqtt-username", "vtn"}},
	} {
		if c.env != nil {
			t.Setenv("OPENADRCTL_MQTT_PASSWORD", "vtn-secret")
		}
		l := start(t, v.bl(append([]string{"bl", "listen", "--via", "mqtt", "--topics", "events", "--mqtt-client-id", c.id, "--format", "json"}, c.env...)...)...)
		waitFor(t, l.err, "listening: "+broker+", ")
		hook.deny.Store(func(id, _ string) bool { return id == c.id })
		hook.drop(t, c.id)
		if code := l.wait(t); code != 4 {
			t.Fatalf("%s: exit %d, want 4\n%s", c.id, code, l.err)
		}
		if e := decode[envelope](t, result{stdout: l.out.String()}); e.Error.Kind != "transport" || e.Error.Code != "transport_connect_failed" || !strings.Contains(e.Error.Message, c.want) {
			t.Errorf("%s: %+v", c.id, e)
		}
		if !strings.Contains(l.err.String(), "warning: mqtt: cannot connect to the broker: mqtt: the broker refused the connection") {
			t.Errorf("%s: stderr:\n%s", c.id, l.err)
		}
	}
	// At the start: at once, without waiting for --timeout.
	t.Setenv("OPENADRCTL_MQTT_PASSWORD", "not-a-token")
	r := must(t, 4, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--timeout", "1m", "--format", "json")...)
	if e := decode[envelope](t, r); !strings.Contains(e.Error.Message, "refused the login: not subscribed") {
		t.Errorf("at the start: %+v", e)
	}
}

// cutter forwards TCP connections to a server and can cut them all, as a
// network that fails does.
type cutter struct {
	addr  string
	mu    sync.Mutex
	conns []net.Conn
}

func startCutter(t *testing.T, target string) *cutter {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &cutter{addr: l.Addr().String()}
	t.Cleanup(func() { _ = l.Close(); c.cut() })
	go func() {
		for {
			in, err := l.Accept()
			if err != nil {
				return
			}
			out, err := net.Dial("tcp", target)
			if err != nil {
				_ = in.Close()
				continue
			}
			c.mu.Lock()
			c.conns = append(c.conns, in, out)
			c.mu.Unlock()
			go func() { _, _ = io.Copy(out, in); _ = out.Close() }()
			go func() { _, _ = io.Copy(in, out); _ = in.Close() }()
		}
	}()
	return c
}

// cut closes every connection made so far.
func (c *cutter) cut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cn := range c.conns {
		_ = cn.Close()
	}
	c.conns = nil
}

// A WebSocket that was lost is a warning on stderr, is opened again, and
// the notifications come on it.
func TestListenWebSocketReconnects(t *testing.T) {
	v := startVTN(t, "--websocket")
	via := startCutter(t, strings.TrimPrefix(v.url, "http://"))
	l := start(t, "ven", "listen", "--via", "websocket", "--object", "EVENT", "--count", "1", "--format", "json",
		"--url", "http://"+via.addr, "--client-id", "site-42", "--client-secret-file", v.venSecret)
	waitFor(t, l.err, "listening: WebSocket open\n")
	via.cut()
	waitFor(t, l.err, "listening: WebSocket open again")
	if !strings.Contains(l.err.String(), "warning: websocket: connection to the VTN lost") {
		t.Errorf("stderr:\n%s", l.err)
	}
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "after-the-cut", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := l.wait(t); code != 0 {
		t.Fatalf("listen: exit %d\n%s", code, l.err)
	}
	if recs := lines(t, l.out.String()); len(recs) != 1 || recs[0]["name"] != "after-the-cut" {
		t.Errorf("records: %v", recs)
	}
}
