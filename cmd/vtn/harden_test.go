// SPDX-License-Identifier: MIT

package vtn

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/seed"
)

func TestBurstDoesNotOverflow(t *testing.T) {
	for rate, want := range map[float64]int{0.5: 2, 1: 3, 10: 21, 1e300: math.MaxInt32, math.MaxFloat64: math.MaxInt32} {
		if got := burst(rate); got != want {
			t.Errorf("burst(%g) = %d, want %d", rate, got, want)
		}
	}
}

func TestValidateRefusesNonFiniteRate(t *testing.T) {
	for _, r := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := startConfig{Listen: "127.0.0.1:0", TokenTTL: time.Hour, Rate: r}
		e, ok := errs.As(c.validate())
		if !ok || e.Code != errs.CodeInvalidInput || !strings.Contains(e.Message, "--rate") {
			t.Errorf("--rate %v: %v", r, c.validate())
		}
	}
	c := startConfig{Listen: "127.0.0.1:0", TokenTTL: time.Hour, Rate: 1e300}
	if err := c.validate(); err != nil {
		t.Errorf("--rate 1e300: %v", err)
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// The example seed holds client secrets: it is written 0600, also over a
// file that was 0644 before --force.
func TestWriteSecretFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.yaml")
	if err := writeSecretFile(path, seed.Example, false); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && mode(t, path) != 0o600 {
		t.Errorf("new file: mode %v", mode(t, path))
	}
	err := writeSecretFile(path, "x", false)
	if e, ok := errs.As(err); !ok || e.Code != errs.CodeOutputFileCreateFailed || e.Kind != errs.KindInvalidInput {
		t.Fatalf("existing file without --force: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSecretFile(path, "short", true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "short" {
		t.Errorf("--force did not truncate: %q", data)
	}
	if runtime.GOOS != "windows" && mode(t, path) != 0o600 {
		t.Errorf("--force over a 0644 file: mode %v", mode(t, path))
	}
	// A dangling symbolic link is a file that exists: refused without --force.
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), link); err == nil {
		if err := writeSecretFile(link, "x", false); err == nil {
			t.Error("wrote through a dangling symlink")
		}
		if _, err := os.Stat(filepath.Join(dir, "elsewhere")); !errors.Is(err, os.ErrNotExist) {
			t.Error("the symlink target was created")
		}
	}
	// With --force a symbolic link is replaced, not written through.
	target := filepath.Join(dir, "target")
	link2 := filepath.Join(dir, "link2.yaml")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link2); err == nil {
		if err := writeSecretFile(link2, "x", true); err != nil {
			t.Fatal(err)
		}
		if data, _ := os.ReadFile(target); string(data) != "keep" {
			t.Errorf("--force wrote through the link: %q", data)
		}
		if fi, err := os.Lstat(link2); err != nil || !fi.Mode().IsRegular() {
			t.Errorf("--force left %v, %v", fi, err)
		}
	}
	// What cannot be removed is not overwritten.
	full := filepath.Join(dir, "full")
	if err := os.MkdirAll(filepath.Join(full, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	if e, ok := errs.As(writeSecretFile(full, "x", true)); !ok || e.Code != errs.CodeOutputFileWriteFailed {
		t.Errorf("a directory that is not empty: %v", e)
	}
	err = writeSecretFile(filepath.Join(dir, "no", "such", "dir.yaml"), "x", true)
	if e, ok := errs.As(err); !ok || e.Code != errs.CodeOutputFileWriteFailed {
		t.Fatalf("missing directory: %v", err)
	}
}

func TestLoadKeyPairRefusesHugeFiles(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.pem")
	if err := os.WriteFile(big, bytes.Repeat([]byte("A"), oadr.MaxInputSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(dir, "small.pem")
	if err := os.WriteFile(small, []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := oadr.LoadKeyPair(big, small); !errors.Is(err, oadr.ErrTooLarge) {
		t.Errorf("huge certificate: %v", err)
	}
	if _, err := oadr.LoadKeyPair(small, big); !errors.Is(err, oadr.ErrTooLarge) {
		t.Errorf("huge key: %v", err)
	}
	if _, err := oadr.LoadKeyPair(small, small); err == nil {
		t.Error("not PEM accepted")
	}
	if _, err := oadr.LoadKeyPair(filepath.Join(dir, "missing"), small); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing certificate: %v", err)
	}
}

// A record is one line: a socket close reason, a log message or a client
// name cannot add lines or drive the terminal.
func TestRecordTextEscapes(t *testing.T) {
	r := Record{Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Event: "socket_closed", Message: "bye\n2026-01-01T00:00:00Z  stopped\x1b[2J"}
	s, _ := r.MarshalTextOutput()
	if strings.ContainsAny(s, "\n\x1b") {
		t.Errorf("text %q", s)
	}
	c := Record{Event: "client", Client: "evil\x1b]0;x\a", Role: "bl"}
	if s, _ := c.MarshalTextOutput(); strings.ContainsAny(s, "\x1b\a") {
		t.Errorf("client %q", s)
	}
}

// failing fails its write or its close.
type failing struct{ write, close error }

func (f failing) Write(p []byte) (int, error) {
	if f.write != nil {
		return 0, f.write
	}
	return len(p), nil
}

func (f failing) Close() error { return f.close }

func TestWriteAndClose(t *testing.T) {
	boom := errors.New("boom")
	for name, f := range map[string]failing{"write": {write: boom}, "close": {close: boom}, "both": {write: boom, close: errors.New("other")}} {
		err := writeAndClose(f, "x")
		if e, ok := errs.As(err); !ok || e.Code != errs.CodeOutputFileWriteFailed || !errors.Is(err, boom) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := writeAndClose(failing{}, "x"); err != nil {
		t.Errorf("no failure: %v", err)
	}
}
