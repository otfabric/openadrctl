// SPDX-License-Identifier: MIT

package format

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCellAndText(t *testing.T) {
	for in, want := range map[string]string{
		"plain":                   "plain",
		"héllo wörld €":           "héllo wörld €",
		"\x1b]0;pwned\a":          `\x1b]0;pwned\x07`,
		"\x1b[31mred":             `\x1b[31mred`,
		"a\tb\nc\rd":              `a\tb\nc\rd`,
		"del\x7f":                 `del\x7f`,
		"c1\u009bcsi":             `c1\x9bcsi`,
		"bidi\u202eevil\u2066x":   `bidi\u202eevil\u2066x`,
		"line\u2028sep":           `line\u2028sep`,
		"bad\xffutf8":             `bad\xffutf8`,
		"\ufffd is a real rune":   "\ufffd is a real rune",
		"":                        "",
		`back\slash stays as is`:  `back\slash stays as is`,
		"nul\x00":                 `nul\x00`,
		"truncated \xe2\x82 rune": `truncated \xe2\x82 rune`,
	} {
		if got := Cell(in); got != want {
			t.Errorf("Cell(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Text("a\tb\nc\x1b[2Jd\r"); got != "a\tb\nc\\x1b[2Jd\\r" {
		t.Errorf("Text keeps tabs and newlines only: %q", got)
	}
	if got := Text("lines\nand\ttabs"); got != "lines\nand\ttabs" {
		t.Errorf("Text of safe text: %q", got)
	}
}

type hostile struct{ name string }

func (h hostile) MarshalTextOutput() (string, error) { return "name: " + h.name + "\n", nil }
func (h hostile) TableHeaders() []string             { return []string{"NAME", "OTHER"} }
func (h hostile) TableRows() [][]string              { return [][]string{{h.name, "x"}} }

// A name from a VTN cannot drive the terminal nor add a row or a column.
func TestOutputEscapesHostileNames(t *testing.T) {
	h := hostile{name: "p\x1b]0;pwned\a\x1b[31m\tTAB\nNL"}
	for _, f := range []OutputFormat{FormatText, FormatTable} {
		var b bytes.Buffer
		if err := Write(&b, f, h); err != nil {
			t.Fatal(err)
		}
		out := b.String()
		if strings.ContainsAny(out, "\x1b\a") {
			t.Errorf("%s: control characters reach the terminal: %q", f, out)
		}
		if f == FormatTable && strings.Count(out, "\n") != 3 {
			t.Errorf("table: a newline in a cell made a row: %q", out)
		}
	}
	var b bytes.Buffer
	s := NewStreamWriter(&b, FormatTable)
	if err := s.Write(h); err != nil {
		t.Fatal(err)
	}
	if out := b.String(); strings.ContainsAny(out, "\x1b\a\t") || strings.Count(out, "\n") != 3 {
		t.Errorf("stream table: %q", out)
	}
	b.Reset()
	if err := NewStreamWriter(&b, FormatText).Write(h); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(b.String(), "\x1b\a") {
		t.Errorf("stream text: %q", b.String())
	}
	// JSON is left to encoding/json, which escapes control characters itself.
	b.Reset()
	if err := Write(&b, FormatJSON, map[string]string{"name": h.name}); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(b.String(), "\x1b\a") || !strings.Contains(b.String(), `\u001b`) {
		t.Errorf("json: %q", b.String())
	}
}

// FuzzCell checks that nothing a terminal acts on survives Cell, that Text
// keeps the lines and tabs of its input, and that both are idempotent.
func FuzzCell(f *testing.F) {
	for _, s := range []string{"", "plain", "\x1b[31m", "a\tb\nc", "\xff\xfe", "\u202e", "\u009b", "\ufffd"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c := Cell(s)
		if !utf8.ValidString(c) {
			t.Fatalf("Cell(%q) = %q is not UTF-8", s, c)
		}
		for _, r := range c {
			if unsafeRune(r) {
				t.Fatalf("Cell(%q) = %q keeps %U", s, c, r)
			}
		}
		if Cell(c) != c {
			t.Fatalf("Cell is not idempotent on %q", s)
		}
		x := Text(s)
		for _, r := range x {
			if unsafeRune(r) && r != '\n' && r != '\t' {
				t.Fatalf("Text(%q) = %q keeps %U", s, x, r)
			}
		}
		if strings.Count(x, "\n") != strings.Count(s, "\n") || strings.Count(x, "\t") != strings.Count(s, "\t") {
			t.Fatalf("Text(%q) = %q changed the layout", s, x)
		}
		if Text(x) != x {
			t.Fatalf("Text is not idempotent on %q", s)
		}
	})
}
