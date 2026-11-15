// SPDX-License-Identifier: MIT

package vtn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/internal/conn"
)

// lockedBuffer is a buffer the command writes while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForRecord waits until out holds want.
func waitForRecord(t *testing.T, out *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(out.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q in:\n%s", want, out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// running is a vtn start of these tests.
type running struct {
	url, addr string
	out       *lockedBuffer
	stop      func() error
}

// startServer runs vtn start on a free port with the example seed and args,
// and waits until it listens.
func startServer(t *testing.T, args ...string) *running {
	t.Helper()
	cmd := newStartCmd()
	out := &lockedBuffer{}
	cmd.SetArgs(append([]string{"--listen", "127.0.0.1:0", "--format", "json"}, args...))
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	var once sync.Once
	var err error
	stop := func() error {
		once.Do(func() {
			cancel()
			select {
			case err = <-done:
			case <-time.After(30 * time.Second):
				err = errors.New("vtn start did not stop")
			}
		})
		return err
	}
	t.Cleanup(func() { _ = stop() })
	waitForRecord(t, out, `"event":"seeded"`)
	r := &running{out: out, stop: stop}
	for line := range strings.SplitSeq(out.String(), "\n") {
		var rec Record
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Event == "listening" {
			r.url, r.addr = rec.URL, rec.Address
		}
	}
	if r.addr == "" {
		t.Fatalf("no listening record:\n%s", out)
	}
	return r
}

// set gives a limit another value for one test.
func set[T any](v *T, to T) func() {
	old := *v
	*v = to
	return func() { *v = old }
}

// dial opens a connection to the VTN that fails the test's reads after a
// guard of 10 seconds, so that a test of something that does not happen
// ends.
func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	return c
}

// closedByServer reads until the server closes c, and fails the test when
// the guard of dial ends the read instead.
func closedByServer(t *testing.T, c net.Conn, r io.Reader) {
	t.Helper()
	_, err := io.Copy(io.Discard, r)
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatal("the VTN kept the connection open")
	}
}

func TestServerClosesIdleConnections(t *testing.T) {
	defer set(&idleTimeout, 200*time.Millisecond)()
	v := startServer(t)
	c := dial(t, v.addr)
	if _, err := io.WriteString(c, "GET /auth/server HTTP/1.1\r\nHost: vtn\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Close {
		t.Fatalf("GET /auth/server: %s, close %v", resp.Status, resp.Close)
	}
	closedByServer(t, c, br)
}

func TestServerEndsRequestsThatDoNotArrive(t *testing.T) {
	defer set(&requestTimeout, 200*time.Millisecond)()
	v := startServer(t, "--mqtt", "mqtt://127.0.0.1:1", "--mqtt-client-auth", "token", "--mqtt-username", "vtn")
	for _, path := range []string{"/auth/token", "/auth/mqtt", "/programs"} {
		c := dial(t, v.addr)
		// A body of 1000 bytes announced, 10 sent.
		_, err := fmt.Fprintf(c, "POST %s HTTP/1.1\r\nHost: vtn\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 1000\r\n\r\ngrant_type", path)
		if err != nil {
			t.Fatal(err)
		}
		closedByServer(t, c, c)
	}
}

// httpClient is a client of its own for a test, whose connections are
// closed before the VTN of the test stops: one it keeps open would hold up
// the stop.
func httpClient(t *testing.T) *http.Client {
	t.Helper()
	hc := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	t.Cleanup(hc.CloseIdleConnections)
	return hc
}

func TestServerRefusesHugeRequestHeads(t *testing.T) {
	v := startServer(t)
	hc := httpClient(t)
	req, err := http.NewRequest(http.MethodGet, v.url+"/programs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Padding", strings.Repeat("a", 100<<10))
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("a head of 100 KiB: %s", resp.Status)
	}
	// One within the limit is served.
	req.Header.Set("X-Padding", strings.Repeat("a", 32<<10))
	resp, err = hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusRequestHeaderFieldsTooLarge {
		t.Errorf("a head of 32 KiB: %s", resp.Status)
	}
}

// client is a connection of a client of the example seed.
func client(t *testing.T, url, id, secret string) *conn.Conn {
	t.Helper()
	cfg := conn.Defaults()
	cfg.URL, cfg.ClientID, cfg.ClientSecret = url, id, secret
	cn, err := cfg.Open()
	if err != nil {
		t.Fatal(err)
	}
	return cn
}

// A WebSocket outlives the limits of the requests: its connection is the
// stream's, with pings of its own.
func TestWebSocketOutlivesRequestLimits(t *testing.T) {
	defer set(&requestTimeout, 200*time.Millisecond)()
	defer set(&idleTimeout, 400*time.Millisecond)()
	v := startServer(t, "--websocket")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ven, err := client(t, v.url, "site-42", "site-42-secret").VEN()
	if err != nil {
		t.Fatal(err)
	}
	_, err = ven.CreateSubscription(ctx, openadr.Subscription{ClientName: "t", ObjectOperations: []openadr.ObjectOperation{{
		Objects: []openadr.ObjectType{openadr.ObjectProgram}, Operations: []openadr.Operation{openadr.OperationCreate},
		Mechanism: openadr.MechanismWebSocket,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	st, err := ven.NotificationStream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	guard := time.AfterFunc(20*time.Second, func() { _ = st.Close() })
	defer guard.Stop()
	// An idle connection is closed after the idle limit, which is longer
	// than the request limit: the stream has been open past both.
	c := dial(t, v.addr)
	if _, err := io.WriteString(c, "GET /auth/server HTTP/1.1\r\nHost: vtn\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	closedByServer(t, c, c)
	bl, err := client(t, v.url, "utility", "utility-secret").BL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bl.CreateProgram(ctx, openadr.Program{Name: "after-the-limits"}); err != nil {
		t.Fatal(err)
	}
	n, err := st.Next()
	if err != nil {
		t.Fatalf("the stream ended: %v\n%s", err, v.out)
	}
	if p, ok := n.Object.(openadr.Program); !ok || p.Name != "after-the-limits" {
		t.Errorf("notification %+v", n)
	}
}

// Stopping closes the connections of the requests that are still in
// progress after the wait, and says so.
func TestStopClosesRequestsInProgress(t *testing.T) {
	defer set(&shutdownTimeout, 200*time.Millisecond)()
	v := startServer(t)
	c := dial(t, v.addr)
	if _, err := io.WriteString(c, "POST /auth/token HTTP/1.1\r\nHost: vtn\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 1000\r\nExpect: 100-continue\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	// The VTN asks for the body once it reads it: the request is in progress.
	br := bufio.NewReader(c)
	if line, err := br.ReadString('\n'); err != nil || !strings.Contains(line, "100 Continue") {
		t.Fatalf("no 100 Continue: %q, %v", line, err)
	}
	if _, err := io.WriteString(c, "grant_type"); err != nil {
		t.Fatal(err)
	}
	if err := v.stop(); err != nil {
		t.Fatal(err)
	}
	closedByServer(t, c, br)
	if out := v.out.String(); !strings.Contains(out, "requests still in progress") || !strings.Contains(out, `"event":"stopped"`) {
		t.Errorf("records:\n%s", out)
	}
}
