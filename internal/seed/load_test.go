// SPDX-License-Identifier: MIT

package seed

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/openadrctl/internal/oadr"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "seed.yaml")
	if err := os.WriteFile(good, []byte(Example), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Load(good); err != nil || len(s.Clients) == 0 {
		t.Fatalf("the example: %v", err)
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	big := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(big, bytes.Repeat([]byte("#"), oadr.MaxInputSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(big); !errors.Is(err, oadr.ErrTooLarge) || !strings.Contains(err.Error(), big) {
		t.Fatalf("too large: %v", err)
	}
}

// Hostile seeds fail with an error, in little time and memory: an alias
// bomb, nesting deeper than the decoders allow, and nesting just under it.
func TestParseHostileSeeds(t *testing.T) {
	var bomb strings.Builder
	bomb.WriteString("programs:\n- programName: p\n  attributes:\n  - {type: A0, values: &a0 [1,1,1,1,1,1,1,1,1,1]}\n")
	for i := 1; i < 9; i++ {
		refs := strings.TrimSuffix(strings.Repeat("*a"+string(rune('0'+i-1))+",", 10), ",")
		bomb.WriteString("  - {type: A" + string(rune('0'+i)) + ", values: &a" + string(rune('0'+i)) + " [" + refs + "]}\n")
	}
	for name, data := range map[string]string{
		"alias bomb":      bomb.String(),
		"too deep":        "programs:\n- programName: p\n  x: " + strings.Repeat("[", 20000) + strings.Repeat("]", 20000) + "\n",
		"deep, in limits": "programs:\n- programName: p\n  x: " + strings.Repeat("[", 9990) + strings.Repeat("]", 9990) + "\n",
		"deep maps":       "programs:\n- programName: p\n  x: " + strings.Repeat("{a: ", 9990) + "1" + strings.Repeat("}", 9990) + "\n",
		"not a mapping":   "- 1\n- 2\n",
		"clients a map":   "clients: {a: 1}\n",
	} {
		if s, err := Parse([]byte(data)); err == nil || s != nil {
			t.Errorf("%s: %v, %v", name, s, err)
		}
	}
}
