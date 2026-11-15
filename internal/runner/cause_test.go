// SPDX-License-Identifier: MIT

package runner

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
)

// plain reports whether s, but for the newlines that end its lines, holds
// only printable UTF-8 without bidirectional controls.
func plain(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range strings.ReplaceAll(s, "\n", "") {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return false
		}
	}
	return true
}

// With --debug every cause is one line, escaped as the message is, and cut
// after 1024 bytes.
func TestDebugCausesAreOneLineEach(t *testing.T) {
	const peer = "detail\x1b]0;title\x07\nerror: another line\t\u202eevil\xff"
	inner := errors.New(peer)
	outer := fmt.Errorf("the VTN said %w", inner)
	long := fmt.Errorf("%s: %w", strings.Repeat("\x1b", 2000), outer)
	typed := errs.New(errs.KindNegative, errs.CodeBadRequest, "refused", long)
	for _, f := range []format.OutputFormat{format.FormatText, format.FormatJSON} {
		inv := &Invocation{}
		inv.SetOutputFormat(f)
		var stdout, stderr bytes.Buffer
		RenderFatal(inv, typed, &stdout, &stderr, true)
		out := stderr.String()
		if !plain(out) {
			t.Errorf("%s: a control character on stderr: %q", f, out)
		}
		if n := strings.Count(out, "cause: "); n != 3 || strings.Count(out, "\n") != n+boolInt(f == format.FormatText) {
			t.Errorf("%s: %d causes in %q", f, n, out)
		}
		if !strings.Contains(out, `\x1b]0;title\x07\nerror: another line\t\u202eevil\xff`) {
			t.Errorf("%s: the escapes are not shown: %q", f, out)
		}
		for line := range strings.SplitSeq(strings.TrimSuffix(out, "\n"), "\n") {
			if len(line) > len("cause: ")+maxCause+len("...") {
				t.Errorf("%s: a line of %d bytes", f, len(line))
			}
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestCauseLineCutsAtACharacter(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{
		{strings.Repeat("a", maxCause), maxCause},
		{strings.Repeat("a", maxCause+1), maxCause + 3},
		{strings.Repeat("a", maxCause-1) + "é", maxCause - 1 + 3},
		{"", 0},
	} {
		got := causeLine(c.in)
		if len(got) != c.want || !utf8.ValidString(got) {
			t.Errorf("%d bytes: %d bytes, valid %v", len(c.in), len(got), utf8.ValidString(got))
		}
	}
}
