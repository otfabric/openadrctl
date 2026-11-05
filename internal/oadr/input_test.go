// SPDX-License-Identifier: MIT

package oadr

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/internal/errs"
)

// endless is a stdin or device that never ends, as /dev/zero.
type endless struct{}

func (endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestLimitReader(t *testing.T) {
	data, err := io.ReadAll(LimitReader(bytes.NewReader(make([]byte, MaxInputSize))))
	if err != nil || len(data) != MaxInputSize {
		t.Fatalf("exactly MaxInputSize: %d bytes, %v", len(data), err)
	}
	data, err = io.ReadAll(LimitReader(bytes.NewReader(make([]byte, MaxInputSize+1))))
	if !errors.Is(err, ErrTooLarge) || len(data) != MaxInputSize {
		t.Fatalf("one byte more: %d bytes, %v", len(data), err)
	}
	if _, err := ReadAll(endless{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("endless input: %v", err)
	}
	r := LimitReader(endless{})
	_, _ = io.ReadAll(r)
	if n, err := r.Read(make([]byte, 8)); n != 0 || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("read after the limit: %d, %v", n, err)
	}
}

func TestReadFileRefusesTooMuchInput(t *testing.T) {
	_, err := ReadFile[openadr.Program]("-", endless{})
	e, ok := errs.As(err)
	if !ok || e.Kind != errs.KindInvalidInput || e.Code != errs.CodeInvalidInput || !strings.Contains(e.Message, "larger than 16 MiB") {
		t.Fatalf("endless stdin: %v", err)
	}

	big := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(big, bytes.Repeat([]byte(" "), MaxInputSize+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile[openadr.Program](big, nil); err == nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("big file: %v", err)
	}

	_, err = ReadFile[openadr.Program](filepath.Join(t.TempDir(), "missing.json"), nil)
	if e, ok := errs.As(err); !ok || e.Code != errs.CodeInputFileOpenFailed {
		t.Fatalf("missing file: %v", err)
	}
	_, err = ReadFile[openadr.Program]("-", strings.NewReader("not json"))
	if e, ok := errs.As(err); !ok || e.Code != errs.CodeInputDecodeFailed {
		t.Fatalf("not JSON: %v", err)
	}
	p, err := ReadFile[openadr.Program]("-", strings.NewReader(`{"programName":"p"}`))
	if err != nil || p.Name != "p" {
		t.Fatalf("a program: %+v, %v", p, err)
	}
}

// The text of an object a VTN sent cannot drive the terminal or break the
// layout.
func TestOneTextEscapes(t *testing.T) {
	p := openadr.Program{Name: "p\x1b[2J\tx\ny", Targets: []string{"t\x07"}}
	p.ID = "abc"
	out, err := (&One{Kind: Programs, Value: p}).MarshalTextOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out, "\x1b\x07") || !strings.Contains(out, `p\x1b[2J\tx\ny`) {
		t.Errorf("text: %q", out)
	}
	out, _ = (&One{Kind: Programs, Value: p, Action: "created"}).MarshalTextOutput()
	if out != `created program abc (p\x1b[2J\tx\ny)` {
		t.Errorf("action: %q", out)
	}
	r := openadr.Report{Resources: []openadr.ReportResource{{ResourceName: "m\x1b"}}}
	if out, _ := (&One{Kind: Reports, Value: r}).MarshalTextOutput(); strings.Contains(out, "\x1b") {
		t.Errorf("field name: %q", out)
	}
}

// Unknown properties that are empty are let through; others are named.
func TestUnknownPropertiesAndEncodeErrors(t *testing.T) {
	if _, err := Decode[openadr.Program]([]byte(`{"programName":"p","a":null,"b":"","c":false,"d":0,"e":[],"f":{}}`)); err != nil {
		t.Errorf("empty unknown properties: %v", err)
	}
	_, err := Decode[openadr.Program]([]byte(`{"programName":"p","b":"x","c":true,"d":1,"e":[1],"f":{"a":1}}`))
	if err == nil || !strings.Contains(err.Error(), `"b", "c", "d", "e", "f"`) {
		t.Errorf("unknown properties: %v", err)
	}
	if _, err := Encode(42); err == nil {
		t.Error("Encode of an int")
	}
	if got := string(MustEncode(42)); got != "null" {
		t.Errorf("MustEncode of an int: %s", got)
	}
}
