// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// What a hostile or broken VTN, token endpoint or webhook sender can do to
// openadrctl's own code.

// secretFile writes a client secret and returns its file.
func secretFile(t *testing.T, secret string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "secret")
	writeFile(t, p, secret)
	return p
}

func TestHostileTokenRedirect(t *testing.T) {
	// The token endpoint redirects with 307 to another origin: net/http would
	// send the form, client secret and all, there again.
	var leaked atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"x","token_type":"Bearer"}`))
	}))
	defer elsewhere.Close()
	vtn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer vtn.Close()
	secret := secretFile(t, "the-secret")
	for _, args := range [][]string{
		{"token", "--url", vtn.URL, "--token-url", vtn.URL + "/auth/token"},
		{"token", "--url", vtn.URL}, // discovery: GET /auth/server is redirected too
		{"ping", "--url", vtn.URL, "--token-url", vtn.URL + "/auth/token"},
	} {
		r := run(t, append(args, "--client-id", "c", "--client-secret-file", secret, "--format", "json")...)
		if r.code != 6 {
			t.Errorf("%v: exit %d\n%s%s", args, r.code, r.stdout, r.stderr)
		}
	}
	if n := leaked.Load(); n != 0 {
		t.Errorf("%d requests reached the other origin", n)
	}
}

func TestHostileAPIRedirect(t *testing.T) {
	// A static token does not follow a redirect to another port.
	var auth atomic.Value
	auth.Store("")
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`[]`))
	}))
	defer elsewhere.Close()
	vtn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusFound)
	}))
	defer vtn.Close()
	tok := filepath.Join(t.TempDir(), "token")
	writeFile(t, tok, "static-token")
	e := decode[envelope](t, must(t, 6, "bl", "programs", "list", "--url", vtn.URL, "--token-file", tok, "--format", "json"))
	if e.Error.Code != "http_error" || e.OpenADR["status"] != float64(302) {
		t.Errorf("redirect: %+v", e)
	}
	if got := auth.Load().(string); got != "" {
		t.Errorf("the token went to another origin: %q", got)
	}
}

func TestHostileMessages(t *testing.T) {
	// A problem whose detail drives the terminal: escaped in the message on
	// stderr, kept as sent in the JSON detail.
	const detail = "\x1b]0;owned\x07\x1b[2Jgone\nerror: forged line"
	vtn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"title":"Bad Request","status":400,"detail":"\u001b]0;owned\u0007\u001b[2Jgone\nerror: forged line"}`))
	}))
	defer vtn.Close()
	r := must(t, 6, "bl", "programs", "list", "--url", vtn.URL)
	if strings.ContainsAny(r.stderr, "\x1b\x07") || strings.Count(r.stderr, "\n") != 1 || !strings.Contains(r.stderr, `\x1b[2Jgone\nerror: forged line`) {
		t.Errorf("stderr: %q", r.stderr)
	}
	e := decode[envelope](t, must(t, 6, "bl", "programs", "list", "--url", vtn.URL, "--format", "json"))
	if strings.ContainsRune(e.Error.Message, 0x1b) || e.OpenADR["problem"].(map[string]any)["detail"] != detail {
		t.Errorf("json: %+v", e)
	}
	// The same in a stage of ping.
	p := decode[Ping](t, must(t, 6, "ping", "--url", vtn.URL, "--format", "json"))
	if s := p.Stages[3]; s.Error == nil || strings.ContainsRune(s.Error.Message, 0x1b) || s.Error.Code != "bad_request" {
		t.Errorf("ping: %+v", p.Stages)
	}
}

func TestHostileTokens(t *testing.T) {
	var body atomic.Value
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/server":
			_, _ = w.Write([]byte(`{"tokenURL":"/token"}`))
		case "/token":
			_, _ = w.Write([]byte(body.Load().(string)))
		case "/programs":
			_, _ = w.Write([]byte(`[]`))
		case "/notifiers":
			_, _ = w.Write([]byte(`{"WEBHOOK":true}`))
		}
	}))
	defer idp.Close()
	secret := secretFile(t, "s")
	conn := []string{"--url", idp.URL, "--client-id", "c", "--client-secret-file", secret}
	// A token that would forge a header, or drive the terminal of $(openadrctl token).
	for _, tok := range []string{`a\r\nX-Evil: 1`, `\u001b]0;x\u0007`, `a b`} {
		body.Store(`{"access_token":"` + tok + `","token_type":"Bearer"}`)
		e := decode[envelope](t, must(t, 6, append([]string{"token", "--format", "json"}, conn...)...))
		if e.Error.Kind != "protocol" {
			t.Errorf("token %s: %+v", tok, e)
		}
		must(t, 6, append([]string{"bl", "programs", "list"}, conn...)...)
	}
	// An expires_in no Duration holds: 100 years, not a negative time.
	body.Store(`{"access_token":"a","token_type":"Bearer","expires_in":1e300}`)
	if r := must(t, 0, append([]string{"token", "--format", "table"}, conn...)...); !strings.Contains(r.stdout, "876000h0m0s") {
		t.Errorf("table: %s", r.stdout)
	}
	if r := must(t, 0, append([]string{"ping"}, conn...)...); !strings.Contains(r.stdout, "valid 876000h0m0s") {
		t.Errorf("ping: %s", r.stdout)
	}
	// The password of --token-url is not printed.
	body.Store(`{"access_token":"a","token_type":"Bearer"}`)
	withPass := strings.Replace(idp.URL, "http://", "http://user:hunter2@", 1) + "/token"
	if r := must(t, 0, append([]string{"token", "--format", "json", "--token-url", withPass}, conn...)...); strings.Contains(r.stdout, "hunter2") {
		t.Errorf("token_url: %s", r.stdout)
	}
	// A token endpoint that is not http(s): refused before anything is sent.
	e := decode[envelope](t, must(t, 3, append([]string{"token", "--format", "json", "--token-url", "file:///etc/passwd"}, conn...)...))
	if e.Error.Code != "invalid_url" {
		t.Errorf("file token url: %+v", e)
	}
}

func TestHostileWebhookSender(t *testing.T) {
	// A sender that opens a request and never sends its body is cut off
	// after --timeout: it does not hold a connection of listen for ever.
	v := startVTN(t, "--insecure-webhooks")
	wh := start(t, v.bl("bl", "listen", "--via", "webhook", "--listen", "127.0.0.1:0", "--timeout", "1s", "--object", "EVENT")...)
	waitFor(t, wh.err, "subscribed:")
	var addr string
	for line := range strings.SplitSeq(wh.err.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "listening: webhook on "); ok {
			addr, _, _ = strings.Cut(rest, ",")
		}
	}
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_, _ = io.WriteString(c, "POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
	// The record: the server ends the connection. 15 s is far beyond the 1 s
	// it gets; without a read timeout it never does.
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, err = io.Copy(io.Discard, bufio.NewReader(c))
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatal("the webhook server kept a request without a body open")
	}
	if code := wh.stop(); code != 0 {
		t.Errorf("listen: exit %d\n%s", code, wh.err)
	}
}

// fakeVTN answers GET /notifiers and the topics with what the test says.
func fakeVTN(t *testing.T, tls bool, notifiers, topics string) *httptest.Server {
	t.Helper()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/notifiers":
			_, _ = w.Write([]byte(notifiers))
		case strings.HasPrefix(r.URL.Path, "/notifiers/mqtt/topics/"):
			_, _ = w.Write([]byte(topics))
		case r.URL.Path == "/subscriptions" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"not an id!","clientName":"openadrctl","objectOperations":[{"objects":["EVENT"],"operations":["CREATE"],"mechanism":"WEBSOCKET"}]}`))
		default:
			http.NotFound(w, r)
		}
	})
	var srv *httptest.Server
	if tls {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)
	return srv
}

// silentBroker accepts TCP connections and never answers.
func silentBroker(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	return "mqtt://" + l.Addr().String()
}

func TestHostileMQTTAnnouncement(t *testing.T) {
	broker := silentBroker(t)
	tok := filepath.Join(t.TempDir(), "token")
	writeFile(t, tok, "the-token")
	topics := `{"topics":{"CREATE":"events/create","UPDATE":"events/update","DELETE":"events/delete"}}`
	bearer := `{"WEBHOOK":false,"MQTT":{"URIS":["` + broker + `"],"serialization":"JSON","authentication":{"method":"OAUTH2_BEARER_TOKEN"}}}`
	listen := func(srv *httptest.Server, code int, extra ...string) envelope {
		t.Helper()
		args := append([]string{"bl", "listen", "--via", "mqtt", "--topics", "events", "--url", srv.URL, "--insecure",
			"--token-file", tok, "--timeout", "500ms", "--format", "json"}, extra...)
		return decode[envelope](t, must(t, code, args...))
	}
	// A VTN reached over https names a broker without TLS for a login with
	// the client's token: the token is not sent in the clear.
	if e := listen(fakeVTN(t, true, bearer, topics), 6); e.Error.Code != "invalid_response" || !strings.Contains(e.Error.Message, "not sent in the clear") {
		t.Errorf("plain broker: %+v", e)
	}
	// --broker is the user's choice: tried, and the silent broker never
	// subscribes (exit 4).
	if e := listen(fakeVTN(t, true, bearer, topics), 4, "--broker", broker); e.Error.Code != "transport_connect_failed" {
		t.Errorf("--broker: %+v", e)
	}
	// A broker that is no MQTT URL: the VTN's fault, exit 6, not a flag error.
	notMQTT := `{"WEBHOOK":false,"MQTT":{"URIS":["http://user:hunter2@broker.example"],"serialization":"JSON","authentication":{"method":"ANONYMOUS"}}}`
	if e := listen(fakeVTN(t, false, notMQTT, topics), 6); e.Error.Code != "invalid_response" || strings.Contains(e.Error.Message, "hunter2") {
		t.Errorf("not mqtt: %+v", e)
	}
	// No topics for a set: exit 6, not a flag error.
	anon := `{"WEBHOOK":false,"MQTT":{"URIS":["` + broker + `"],"serialization":"JSON","authentication":{"method":"ANONYMOUS"}}}`
	if e := listen(fakeVTN(t, false, anon, `{"topics":{"UPDATE":"","DELETE":""}}`), 6); e.Error.Kind != "protocol" {
		t.Errorf("no topics: %+v", e)
	}
	// The password of --broker is not printed.
	withPass := strings.Replace(broker, "mqtt://", "mqtt://user:hunter2@", 1)
	if e := listen(fakeVTN(t, false, anon, topics), 4, "--broker", withPass); strings.Contains(e.Error.Message, "hunter2") {
		t.Errorf("broker password: %+v", e)
	}
}

func TestHostileSubscription(t *testing.T) {
	// A subscription created with an ID that is none: nothing to delete it
	// by, and nothing to print; exit 6 before anything is opened.
	srv := fakeVTN(t, false, `{"WEBHOOK":true,"WEBSOCKET":true}`, `{}`)
	e := decode[envelope](t, must(t, 6, "ven", "listen", "--via", "websocket", "--url", srv.URL, "--timeout", "2s", "--format", "json"))
	if e.Error.Kind != "protocol" {
		t.Errorf("subscription ID: %+v", e)
	}
}

// A subscription the VTN will not delete is reported as a warning, with
// what the VTN said escaped.
func TestHostileSubscriptionNotDeleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/notifiers":
			_, _ = w.Write([]byte(`{"WEBHOOK":true,"WEBSOCKET":true}`))
		case r.URL.Path == "/subscriptions" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sub-1","clientName":"openadrctl","objectOperations":[{"objects":["EVENT"],"operations":["CREATE"],"mechanism":"WEBSOCKET"}]}`))
		case r.URL.Path == "/subscriptions/sub-1" && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"title":"Internal Server Error","status":500,"detail":"\u001b[2Jgone"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	r := run(t, "ven", "listen", "--via", "websocket", "--url", srv.URL, "--timeout", "1s", "--format", "json")
	if r.code == 0 || !strings.Contains(r.stderr, "warning: subscription sub-1 was not deleted") {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stderr, "\x1b") || !strings.Contains(r.stderr, `\x1b[2Jgone`) {
		t.Errorf("the VTN's detail is not escaped:\n%q", r.stderr)
	}
}
