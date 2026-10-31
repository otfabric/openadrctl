// SPDX-License-Identifier: MIT

package seed

import (
	"testing"
)

// FuzzParse feeds seed files. Parse reports every problem it finds, so it
// must return a seed or an error, never both, and a seed it accepts must not
// depend on anything but its input.
func FuzzParse(f *testing.F) {
	f.Add([]byte(Example))
	for _, s := range []string{
		"",
		"clients: 1",
		"clients:\n- id: a\n  secret: s\n  role: bl\n",
		"clients:\n- id: a\n  secret: s\n  secretEnv: X\n  role: nobody\n",
		"programs:\n- programName: p\n",
		"programs:\n- id: x\n  programName: p\nevents:\n- programID: x\n  intervals: []\n",
		"vens:\n- venName: v\nresources:\n- resourceName: r\n",
		"resourceGroups:\n- {}\n",
		"unknown: 1\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := Parse(data)
		if err != nil {
			if s != nil {
				t.Fatalf("a seed and an error: %v", err)
			}
			return
		}
		if s == nil {
			t.Fatal("no seed and no error")
		}
		again, err := Parse(data)
		if err != nil {
			t.Fatalf("second parse failed: %v", err)
		}
		if len(again.Clients) != len(s.Clients) || len(again.Programs) != len(s.Programs) ||
			len(again.VENs) != len(s.VENs) || len(again.Resources) != len(s.Resources) ||
			len(again.Events) != len(s.Events) || len(again.ResourceGroups) != len(s.ResourceGroups) {
			t.Fatal("two parses of one document differ")
		}
	})
}
