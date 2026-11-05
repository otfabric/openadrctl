// SPDX-License-Identifier: MIT

package format

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Cell makes a string safe to print in one cell of a table or one line of
// text: control characters (newlines and tabs included), DEL, C1 controls,
// the bidirectional overrides and line separators of Unicode, and bytes that
// are not UTF-8 are written as Go escapes (\n, \t, \x1b, \u202e). Names and
// messages come from VTNs, notifications and files: printed raw, an ESC could
// drive the terminal and a newline or tab forge a row.
func Cell(s string) string { return escape(s, false) }

// Text is Cell for text that has lines and columns of its own: newlines and
// tabs are kept, everything else Cell escapes is escaped.
func Text(s string) string { return escape(s, true) }

func escape(s string, keepLayout bool) string {
	if !needsEscape(s, keepLayout) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			_, _ = fmt.Fprintf(&b, `\x%02x`, s[i])
		case keepLayout && (r == '\n' || r == '\t'):
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case unsafeRune(r) && r < 0x100:
			_, _ = fmt.Fprintf(&b, `\x%02x`, r)
		case unsafeRune(r):
			_, _ = fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

func needsEscape(s string, keepLayout bool) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			return true
		}
		if unsafeRune(r) && (!keepLayout || (r != '\n' && r != '\t')) {
			return true
		}
		i += size
	}
	return false
}

// unsafeRune reports a rune a terminal may act on rather than show.
func unsafeRune(r rune) bool {
	if unicode.Is(unicode.Cc, r) { // C0, DEL and C1
		return true
	}
	switch {
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069: // bidirectional embeddings, overrides, isolates
		return true
	case r == 0x2028 || r == 0x2029: // line and paragraph separators
		return true
	}
	return false
}
