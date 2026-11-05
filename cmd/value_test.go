// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"
)

// A --value written as a decimal too large for a number exits 3 before
// anything is sent; one written as no decimal is text.
func TestValueNumbers(t *testing.T) {
	r := must(t, 3, "bl", "events", "create", "--url", "http://127.0.0.1:1", "--program", "p1", "--type", "PRICE", "--value", "1e400", "--dry-run")
	if !strings.Contains(r.stderr, `--value: "1e400" is a number larger than a value holds`) {
		t.Errorf("stderr:\n%s", r.stderr)
	}
	r = must(t, 0, "bl", "events", "create", "--url", "http://127.0.0.1:1", "--program", "p1", "--type", "PRICE", "--value", "0x1p3", "--value", "0.5", "--dry-run", "--format", "json")
	if !strings.Contains(strings.Join(strings.Fields(r.stdout), ""), `"values":["0x1p3"]`) {
		t.Errorf("0x1p3 is not text:\n%s", r.stdout)
	}
	if !strings.Contains(strings.Join(strings.Fields(r.stdout), ""), `"values":[0.5]`) {
		t.Errorf("0.5 is not a number:\n%s", r.stdout)
	}
}
