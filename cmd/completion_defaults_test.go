// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// complete runs the hidden __complete command and returns the candidates
// (without descriptions) and the directive line.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	r := must(t, 0, append([]string{"__complete"}, args...)...)
	var values []string
	var directive string
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = line
			continue
		}
		v, _, _ := strings.Cut(line, "\t")
		values = append(values, v)
	}
	return values, directive
}

func wantCandidates(t *testing.T, args []string, want ...string) {
	t.Helper()
	got, _ := complete(t, args...)
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("__complete %v = %v, missing %q", args, got, w)
		}
	}
}

func TestCompleteShells(t *testing.T) {
	got, _ := complete(t, "completion", "")
	if want := []string{"bash", "zsh", "fish", "powershell"}; !slices.Equal(got, want) {
		t.Fatalf("completion shells %v, want %v", got, want)
	}
}

// TestCompleteEnumFlagPerGroup checks one enum flag of every command group.
func TestCompleteEnumFlagPerGroup(t *testing.T) {
	wantCandidates(t, []string{"ven", "programs", "list", "--format", ""}, "text", "json", "table")
	wantCandidates(t, []string{"ven", "programs", "list", "--spec", ""}, "3.1.0", "3.0.1")
	wantCandidates(t, []string{"ven", "listen", "--via", ""}, "webhook", "mqtt", "websocket")
	wantCandidates(t, []string{"bl", "subscriptions", "create", "--object", ""}, "PROGRAM", "EVENT", "VEN")
	wantCandidates(t, []string{"bl", "subscriptions", "create", "--operation", ""}, "CREATE", "DELETE")
	wantCandidates(t, []string{"bl", "topics", ""}, "programs", "ven-events:")
	wantCandidates(t, []string{"vtn", "start", "--mqtt-client-auth", ""}, "anonymous", "token")
	wantCandidates(t, []string{"validate", ""}, "event", "program")
	wantCandidates(t, []string{"timeline", "x.json", "--at", ""}, "now")
	wantCandidates(t, []string{"ping", "--url", ""}, "https://")
}

func TestCompleteFilesAndArgs(t *testing.T) {
	ext := fmt.Sprintf(":%d", cobra.ShellCompDirectiveFilterFileExt)
	noFile := fmt.Sprintf(":%d", cobra.ShellCompDirectiveNoFileComp)
	if got, d := complete(t, "validate", "event", ""); !slices.Equal(got, []string{"json"}) || d != ext {
		t.Errorf("validate <kind> <file>: %v %s", got, d)
	}
	if got, d := complete(t, "timeline", ""); !slices.Equal(got, []string{"json"}) || d != ext {
		t.Errorf("timeline <event-file>: %v %s", got, d)
	}
	if got, d := complete(t, "ven", "programs", "list", "--cert", ""); !slices.Contains(got, "pem") || d != ext {
		t.Errorf("--cert: %v %s", got, d)
	}
	if _, d := complete(t, "ven", "programs", "list", "--timeout", ""); d != noFile {
		t.Errorf("--timeout: %s, want no file completion", d)
	}
	if _, d := complete(t, "bl", "programs", "get", ""); d != noFile {
		t.Errorf("get <id>: %s, want no file completion", d)
	}
}

// valueSetUsage matches a usage string that lists the values of a flag.
var valueSetUsage = regexp.MustCompile(`\w+\|\w+|one of|: \w+, \w+,? (or|and) \w+|: [\d.]+ or [\d.]+`)

// TestEveryValueFlagCompletes walks every command: a flag that takes a value
// has a completion function or a file-name annotation, so a flag whose usage
// lists a value set cannot lack one.
func TestEveryValueFlagCompletes(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || f.Name == "help" || hasCompletion(c, f) {
				return
			}
			kind := "value"
			if valueSetUsage.MatchString(f.Usage) {
				kind = "value set"
			}
			t.Errorf("%s --%s (%s flag) has no completion: %s", c.CommandPath(), f.Name, kind, f.Usage)
		}
		c.LocalFlags().VisitAll(check)
		c.InheritedFlags().VisitAll(check)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(NewRootCmd())
}

// TestCompleteNothingAfterLastArg: a command whose positional arguments are all
// given offers nothing more, and no file names either.
func TestCompleteNothingAfterLastArg(t *testing.T) {
	noFile := fmt.Sprintf(":%d", cobra.ShellCompDirectiveNoFileComp)
	for _, args := range [][]string{
		{"bl", "topics", "programs", ""},
		{"ven", "topics", "ven-events:1f2e3d4c", ""},
		{"validate", "event", "event.json", ""},
	} {
		if got, d := complete(t, args...); len(got) != 0 || d != noFile {
			t.Errorf("__complete %v = %v %s, want nothing and %s", args, got, d, noFile)
		}
	}
}
