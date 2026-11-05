// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/otfabric/openadrctl/internal/oadr"
)

type errorEnvelope struct {
	Error struct{ Kind, Code, Message string }
}

// More than the limit on stdin is refused with exit 3, not read without end.
func TestValidateRefusesHugeStdin(t *testing.T) {
	stdin := `{"programName":"p","x":"` + strings.Repeat("a", oadr.MaxInputSize) + `"}`
	r := runIn(t, stdin, "validate", "program", "-", "--format", "json")
	if r.code != 3 {
		t.Fatalf("exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	env := decode[errorEnvelope](t, r)
	if env.Error.Kind != "invalid_input" || env.Error.Code != "invalid_input" || !strings.Contains(env.Error.Message, "larger than 16 MiB") {
		t.Errorf("error: %+v", env.Error)
	}
}

// The example seed holds client secrets: --force over a 0644 file leaves it 0600.
func TestGenerateConfigForceKeepsSecretsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file modes")
	}
	path := filepath.Join(t.TempDir(), "seed.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	must(t, 3, "vtn", "generate-config", "--output", path)
	must(t, 0, "vtn", "generate-config", "--output", path, "--force")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestVTNStartRefusesNonFiniteRate(t *testing.T) {
	for _, rate := range []string{"NaN", "Inf"} {
		r := must(t, 3, "vtn", "start", "--listen", "127.0.0.1:0", "--rate", rate, "--duration", "1ms")
		if !strings.Contains(r.stderr, "--rate") {
			t.Errorf("--rate %s: %s", rate, r.stderr)
		}
	}
}

func TestVTNStartRefusesHugeSeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.yaml")
	if err := os.WriteFile(path, []byte(strings.Repeat("#", oadr.MaxInputSize+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	r := must(t, 3, "vtn", "start", "--listen", "127.0.0.1:0", "--config", path, "--format", "json")
	if env := decode[errorEnvelope](t, r); env.Error.Code != "invalid_config" || !strings.Contains(env.Error.Message, "larger than 16 MiB") {
		t.Errorf("error: %+v", env.Error)
	}
}

// What a file names is printed escaped in text and table output.
func TestOutputEscapesControlCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	if err := os.WriteFile(path, []byte(`{"programName":"p\u001b]0;pwned\u0007\tTAB\nNL"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"text", "table"} {
		r := must(t, 0, "bl", "programs", "create", "--file", path, "--dry-run", "--format", f)
		if strings.ContainsAny(r.stdout, "\x1b\a") || !strings.Contains(r.stdout, `p\x1b]0;pwned\x07\tTAB\nNL`) {
			t.Errorf("%s: %q", f, r.stdout)
		}
	}
	r := must(t, 0, "validate", "program", path)
	if strings.ContainsAny(r.stdout, "\x1b\a") {
		t.Errorf("validate: %q", r.stdout)
	}
	// JSON keeps the name as it is.
	r = must(t, 0, "bl", "programs", "create", "--file", path, "--dry-run", "--format", "json")
	if p := decode[map[string]any](t, r); p["programName"] != "p\x1b]0;pwned\a\tTAB\nNL" {
		t.Errorf("json: %q", p["programName"])
	}
}
