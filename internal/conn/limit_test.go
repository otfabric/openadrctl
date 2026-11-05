// SPDX-License-Identifier: MIT

package conn

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
)

// Files the user names are read up to oadr.MaxInputSize bytes: a secret, a
// token or a CA bundle larger than that is invalid input, not read into
// memory.
func TestFilesAreLimited(t *testing.T) {
	big := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(big, make([]byte, oadr.MaxInputSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	check := func(what string, err error) {
		t.Helper()
		e, ok := errs.As(err)
		if !errors.Is(err, oadr.ErrTooLarge) || !ok || e.Code != errs.CodeInvalidInput {
			t.Errorf("%s: %v", what, err)
		}
	}
	_, err := readSecret(big, "--token-file")
	check("token file", err)
	_, err = LoadPool(big)
	check("CA bundle", err)
	cfg := Defaults()
	cfg.URL, cfg.Cert, cfg.Key = "https://vtn.example", big, big
	_, err = cfg.Open()
	if !errors.Is(err, oadr.ErrTooLarge) {
		t.Errorf("client certificate: %v", err)
	}
}
