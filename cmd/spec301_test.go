// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// vtn301 is a VTN of OpenADR 3.0.1, as far as these tests need one: a
// token at /auth/token and no /auth/server, typed targets, resources under
// their VEN. It records what it is asked.
type vtn301 struct {
	url, dir string
	mu       sync.Mutex
	requests []string
	bodies   map[string]string
}

func startVTN301(t *testing.T) *vtn301 {
	v := &vtn301{dir: t.TempDir(), bodies: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		line := r.Method + " " + r.URL.Path
		v.mu.Lock()
		v.requests = append(v.requests, line+"?"+r.URL.RawQuery)
		v.bodies[line] = string(body)
		v.mu.Unlock()
		if r.URL.Path != "/auth/token" && r.Header.Get("Authorization") != "Bearer t301" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch line {
		case "POST /auth/token":
			_, _ = fmt.Fprint(w, `{"access_token":"t301","token_type":"Bearer","expires_in":3600}`)
		case "GET /programs":
			_, _ = fmt.Fprint(w, `[{"id":"p1","programName":"p1","targets":[{"type":"GROUP","values":["g1"]}]}]`)
		case "POST /programs":
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":"p2",`+strings.TrimPrefix(string(body), "{"))
		case "GET /vens/v1/resources/r1":
			_, _ = fmt.Fprint(w, `{"id":"r1","resourceName":"meter","venID":"v1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, `{"title":"Not Found","status":404,"detail":"%s"}`, line)
		}
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL
	writeFile(t, filepath.Join(v.dir, "secret"), "s")
	return v
}

func (v *vtn301) bl(args ...string) []string {
	return append([]string{"bl"}, append(args, "--url", v.url, "--spec", "3.0.1", "--client-id", "c",
		"--client-secret-file", filepath.Join(v.dir, "secret"))...)
}

// compact is JSON without the whitespace of --format json.
func compact(s string) string {
	var b bytes.Buffer
	if json.Compact(&b, []byte(s)) != nil {
		return s
	}
	return b.String()
}

func (v *vtn301) take() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := v.requests
	v.requests = nil
	return out
}

func TestSpec301(t *testing.T) {
	v := startVTN301(t)

	// ping: the token from /auth/token, no /auth/server, and notifiers are
	// not a step of 3.0.1.
	r := must(t, 0, append([]string{"ping"}, v.bl()[1:]...)...)
	for _, want := range []string{"token      ok", "from " + v.url + "/auth/token", "api        ok", "notifiers  skipped", "not in OpenADR 3.0.1", "answers"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("ping lacks %q:\n%s", want, r.stdout)
		}
	}
	for _, req := range v.take() {
		if strings.HasPrefix(req, "GET /auth/server") || strings.HasPrefix(req, "GET /notifiers") {
			t.Errorf("ping asked %s of a 3.0.1 VTN", req)
		}
	}

	// A search by targets names their type; the targets read are shown with it.
	r = must(t, 0, v.bl("programs", "list", "--target", "GROUP=g1", "--target", "GROUP=g2")...)
	if !strings.Contains(r.stdout, "GROUP=g1") {
		t.Errorf("programs list:\n%s", r.stdout)
	}
	if got := strings.Join(v.take(), "; "); !strings.Contains(got, "GET /programs?") || !strings.Contains(got, "targetType=GROUP&targetValues=g1&targetValues=g2") {
		t.Errorf("requests: %s", got)
	}
	r = must(t, 0, v.bl("programs", "list", "--format", "json")...)
	if !strings.Contains(compact(r.stdout), `"targets":[{"type":"GROUP","values":["g1"]}]`) {
		t.Errorf("programs list --format json:\n%s", r.stdout)
	}
	// Targets without a type, or of two types in one search, are refused.
	must(t, 3, v.bl("programs", "list", "--target", "g1")...)
	must(t, 3, v.bl("programs", "list", "--target", "GROUP=g1", "--target", "VEN_NAME=v")...)

	// A program is sent with typed targets.
	must(t, 0, v.bl("programs", "create", "--name", "p2", "--target", "GROUP=g1", "--target", "GROUP=g2")...)
	if body := v.bodies["POST /programs"]; !strings.Contains(body, `"targets":[{"type":"GROUP","values":["g1","g2"]}]`) {
		t.Errorf("program sent: %s", body)
	}
	// --dry-run writes the JSON of 3.0.1 without a VTN.
	r = must(t, 0, v.bl("programs", "create", "--name", "p3", "--target", "GROUP=g1", "--dry-run", "--format", "json")...)
	if !strings.Contains(compact(r.stdout), `"targets":[{"type":"GROUP","values":["g1"]}]`) {
		t.Errorf("dry run:\n%s", r.stdout)
	}

	// A resource is reached through its VEN.
	v.take()
	must(t, 0, v.bl("resources", "get", "r1", "--ven", "v1")...)
	if got := strings.Join(v.take(), "; "); !strings.Contains(got, "GET /vens/v1/resources/r1") {
		t.Errorf("requests: %s", got)
	}
	r = must(t, 3, v.bl("resources", "get", "r1")...)
	if !strings.Contains(r.stderr, "--ven is required") {
		t.Errorf("resources get without --ven: %s", r.stderr)
	}

	// What 3.0.1 does not have is refused, and nothing is sent.
	v.take()
	for _, args := range [][]string{{"notifiers"}, {"resource-groups", "list"}} {
		r = must(t, 3, v.bl(append(args, "--format", "json")...)...)
		if !strings.Contains(r.stdout+r.stderr, "not_in_revision") {
			t.Errorf("%s: %s %s", args, r.stdout, r.stderr)
		}
	}
	for _, req := range v.take() {
		if !strings.HasPrefix(req, "POST /auth/token") {
			t.Errorf("sent %s", req)
		}
	}
	must(t, 3, v.bl("listen", "--via", "websocket")...)
	must(t, 3, v.bl("listen", "--via", "webhook")...)

	// An unknown revision.
	must(t, 3, "bl", "programs", "list", "--url", v.url, "--spec", "3.0.2")

	// validate reads the JSON of the revision of --spec.
	file := filepath.Join(v.dir, "program.json")
	writeFile(t, file, `{"programName":"p","targets":[{"type":"GROUP","values":["g1"]}],"country":"NL"}`)
	must(t, 0, "validate", "program", file, "--spec", "3.0.1")
	must(t, 3, "validate", "program", file)
}
