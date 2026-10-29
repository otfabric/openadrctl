// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	mqttauth "github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

// result is what a run of openadrctl wrote and returned.
type result struct {
	stdout, stderr string
	code           int
}

// run runs openadrctl with args, as the process does.
func run(t *testing.T, args ...string) result {
	t.Helper()
	return runIn(t, "", args...)
}

// runIn is run with stdin.
func runIn(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	code := Run(ctx, args, strings.NewReader(stdin), &out, &errb)
	return result{out.String(), errb.String(), code}
}

// must runs args and fails the test unless it exits with code.
func must(t *testing.T, code int, args ...string) result {
	t.Helper()
	r := run(t, args...)
	if r.code != code {
		t.Fatalf("openadrctl %s: exit %d, want %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), r.code, code, r.stdout, r.stderr)
	}
	return r
}

// decode decodes the JSON of a result.
func decode[T any](t *testing.T, r result) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.stdout)
	}
	return v
}

// lines decodes JSON Lines.
func lines(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not a JSON line: %v\n%s", err, line)
		}
		out = append(out, m)
	}
	return out
}

// envelope is the JSON error object.
type envelope struct {
	Error struct {
		Kind, Code, Message string
	} `json:"error"`
	OpenADR map[string]any `json:"openadr"`
}

// syncBuffer is a buffer a running command writes to while a test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitFor waits until s holds want.
func waitFor(t *testing.T, b *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(b.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no %q in:\n%s", want, b.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// background is a command running in the background.
type background struct {
	out, err *syncBuffer
	done     chan int
	cancel   context.CancelFunc
}

// start runs args in the background until stop.
func start(t *testing.T, args ...string) *background {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b := &background{out: &syncBuffer{}, err: &syncBuffer{}, done: make(chan int, 1), cancel: cancel}
	go func() { b.done <- Run(ctx, args, strings.NewReader(""), b.out, b.err) }()
	t.Cleanup(func() { b.stop() })
	return b
}

// stop interrupts the command and returns its exit code.
func (b *background) stop() int {
	b.cancel()
	select {
	case code := <-b.done:
		b.done <- code
		return code
	case <-time.After(20 * time.Second):
		return -1
	}
}

// wait waits for the command to end by itself.
func (b *background) wait(t *testing.T) int {
	t.Helper()
	select {
	case code := <-b.done:
		b.done <- code
		return code
	case <-time.After(20 * time.Second):
		t.Fatalf("still running:\nstdout: %s\nstderr: %s", b.out, b.err)
		return -1
	}
}

// testVTN is a vtn start of the tests.
type testVTN struct {
	*background
	url, dir            string
	blSecret, venSecret string
}

// startVTN runs vtn start on a free port, with the example seed unless
// args give one, and waits until it listens.
func startVTN(t *testing.T, args ...string) *testVTN {
	t.Helper()
	dir := t.TempDir()
	v := &testVTN{dir: dir, blSecret: filepath.Join(dir, "bl.secret"), venSecret: filepath.Join(dir, "ven.secret")}
	writeFile(t, v.blSecret, "utility-secret\n")
	writeFile(t, v.venSecret, "site-42-secret\n")
	v.background = start(t, append([]string{"vtn", "start", "--listen", "127.0.0.1:0", "--format", "json"}, args...)...)
	waitFor(t, v.out, "seeded")
	sc := bufio.NewScanner(strings.NewReader(v.out.String()))
	for sc.Scan() {
		var rec struct{ Event, URL string }
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.Event == "listening" {
			v.url = rec.URL
		}
		// The text form: <time>  listening  <url> on <address>
		if f := strings.Fields(sc.Text()); len(f) > 2 && f[1] == "listening" {
			v.url = f[2]
		}
	}
	if v.url == "" {
		t.Fatalf("no listening record:\n%s", v.out)
	}
	return v
}

// bl and ven are the connection flags of the two clients of the seed.
func (v *testVTN) bl(args ...string) []string {
	return append(args, "--url", v.url, "--client-id", "utility", "--client-secret-file", v.blSecret)
}

func (v *testVTN) ven(args ...string) []string {
	return append(args, "--url", v.url, "--client-id", "site-42", "--client-secret-file", v.venSecret)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// startBroker runs an MQTT broker that lets everyone in.
func startBroker(t *testing.T) string {
	t.Helper()
	return serveBroker(t, new(mqttauth.AllowHook))
}

// tokenHook asks an HTTP authenticator whether a client may log in, as
// the HTTP authentication of EMQX does: a form POST of the user name and
// password, allowed on 200.
type tokenHook struct {
	mochi.HookBase
	url atomic.Value // string
}

func (h *tokenHook) ID() string { return "token" }

func (h *tokenHook) Provides(b byte) bool {
	return b == mochi.OnConnectAuthenticate || b == mochi.OnACLCheck
}

func (h *tokenHook) OnConnectAuthenticate(_ *mochi.Client, pk packets.Packet) bool {
	u, _ := h.url.Load().(string)
	if u == "" {
		return false
	}
	resp, err := http.PostForm(u, url.Values{"username": {string(pk.Connect.Username)}, "password": {string(pk.Connect.Password)}})
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (h *tokenHook) OnACLCheck(*mochi.Client, string, bool) bool { return true }

// startTokenBroker runs an MQTT broker that lets in only those its
// authenticator allows; set the authenticator's URL with the hook.
func startTokenBroker(t *testing.T) (string, *tokenHook) {
	t.Helper()
	h := new(tokenHook)
	return serveBroker(t, h), h
}

func serveBroker(t *testing.T, hook mochi.Hook) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	srv := mochi.New(&mochi.Options{InlineClient: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := srv.AddHook(hook, nil); err != nil {
		t.Fatal(err)
	}
	if err := srv.AddListener(listeners.NewTCP(listeners.Config{ID: "t", Address: addr})); err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve() }()
	t.Cleanup(func() { _ = srv.Close() })
	return "mqtt://" + addr
}

// selfSigned writes a certificate for 127.0.0.1 and localhost, and its key.
func selfSigned(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "openadrctl test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	writeFile(t, certFile, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	writeFile(t, keyFile, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder})))
	return certFile, keyFile
}
