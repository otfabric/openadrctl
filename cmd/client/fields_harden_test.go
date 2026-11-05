// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
)

func TestParseDurationRefusesNegative(t *testing.T) {
	for _, s := range []string{"-PT1H", "-1h", "-P1D", "PT99999999999999999999H", "1h99999999999h", "x"} {
		if _, err := parseDuration("interval", s); err == nil {
			t.Errorf("%q accepted", s)
		} else if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidInput || !strings.HasPrefix(e.Message, "--interval: ") {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"PT1H", "P1DT12H", "1h30m", "0s", "PT0S"} {
		if _, err := parseDuration("interval", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestIntervalsCSVErrors(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.csv")
	row := []byte("0,2030-01-01T00:00:00Z,2030-01-01T01:00:00Z,PRICE,1\n")
	data := append([]byte("interval,start,end,type,values\n"), bytes.Repeat(row, oadr.MaxInputSize/len(row)+1)...)
	if err := os.WriteFile(big, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := (&intervals{csv: big}).build()
	if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidInput || !errors.Is(err, oadr.ErrTooLarge) {
		t.Fatalf("a CSV over the limit: %v", err)
	}
	bad := filepath.Join(dir, "bad.csv")
	if err := os.WriteFile(bad, []byte("no,header\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&intervals{csv: bad}).build(); err == nil {
		t.Error("a CSV without the columns accepted")
	} else if e, _ := errs.As(err); e.Code != errs.CodeInputDecodeFailed {
		t.Errorf("bad CSV: %v", err)
	}
	if _, _, err := (&intervals{csv: filepath.Join(dir, "missing.csv")}).build(); err == nil {
		t.Error("a missing CSV accepted")
	} else if e, _ := errs.As(err); e.Code != errs.CodeInputFileOpenFailed {
		t.Errorf("missing CSV: %v", err)
	}
	if _, _, err := (&intervals{csv: bad, values: []string{"1"}}).build(); err == nil {
		t.Error("--csv and --value accepted")
	}
	for name, iv := range map[string]intervals{
		"no values":     {typ: "PRICE"},
		"no type":       {values: []string{"1"}},
		"bad start":     {typ: "PRICE", values: []string{"1"}, start: "yesterday"},
		"bad interval":  {typ: "PRICE", values: []string{"1"}, duration: "-PT1H"},
		"good, no time": {typ: "PRICE", values: []string{"1"}},
	} {
		_, out, err := iv.build()
		if (err == nil) != (name == "good, no time") || (err == nil && len(out) != 1) {
			t.Errorf("%s: %v, %v", name, out, err)
		}
	}
}

// FuzzFlagValues feeds the parsers of --value, --attribute, --start,
// --interval and --duration: they return a value or an invalid-input error,
// never panic, and never yield a negative duration.
func FuzzFlagValues(f *testing.F) {
	for _, s := range []string{"1", "true", "1:2", "x", "NaN", "1e400", "0x1p3", "PT1H", "-PT1H", "1h", "-1h",
		"P1Y2M3W4DT5H6M7.5S", "TYPE=1,2,x", "=1", "now", "2030-01-01T00:00:00Z", "PT9223372036854775807H"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_ = parseValue(s)
		if m, err := parseValuesMaps("attribute", []string{s}); err != nil {
			if _, ok := errs.As(err); !ok {
				t.Fatalf("parseValuesMaps(%q): %v is not an errs.Error", s, err)
			}
		} else if len(m) != 1 || m[0].Type == "" {
			t.Fatalf("parseValuesMaps(%q) = %v", s, m)
		}
		if d, err := parseDuration("interval", s); err == nil && d.Negative {
			t.Fatalf("parseDuration(%q) is negative", s)
		} else if err != nil {
			if _, ok := errs.As(err); !ok {
				t.Fatalf("parseDuration(%q): %v is not an errs.Error", s, err)
			}
		}
		if _, err := parseTime("start", s); err != nil {
			if _, ok := errs.As(err); !ok {
				t.Fatalf("parseTime(%q): %v is not an errs.Error", s, err)
			}
		}
	})
}
