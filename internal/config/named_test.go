// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type (
	port   uint16
	level  uint8
	ratio  float64
	label  string
	toggle bool
	count  int
	names  []string
	tag    string
)

type namedCfg struct {
	Port   port   `flag:"port" env:"N_PORT" desc:"port"`
	Level  level  `flag:"level" env:"N_LEVEL" desc:"level"`
	Ratio  ratio  `flag:"ratio" env:"N_RATIO" desc:"ratio"`
	Label  label  `flag:"label" env:"N_LABEL" desc:"label"`
	Toggle toggle `flag:"toggle" env:"N_TOGGLE" desc:"toggle"`
	Count  count  `flag:"count" env:"N_COUNT" desc:"count"`
	Names  names  `flag:"names" env:"N_NAMES" desc:"names"`
	Tags   []tag  `env:"N_TAGS"`
}

// A field of a named type binds as its kind does: it used to panic on the
// type assertion to *uint8 and the like.
func TestNamedTypes(t *testing.T) {
	for k, v := range map[string]string{"N_PORT": "8080", "N_LEVEL": "7", "N_RATIO": "0.5", "N_LABEL": "l",
		"N_TOGGLE": "true", "N_COUNT": "-3", "N_NAMES": "a, b", "N_TAGS": "x,y"} {
		t.Setenv(k, v)
	}
	c := namedCfg{}
	if err := LoadFromEnv(&c); err != nil {
		t.Fatal(err)
	}
	if c.Port != 8080 || c.Level != 7 || c.Ratio != 0.5 || c.Label != "l" || !bool(c.Toggle) || c.Count != -3 ||
		strings.Join(c.Names, "|") != "a|b" || len(c.Tags) != 2 || c.Tags[1] != "y" {
		t.Fatalf("env: %+v", c)
	}
	cmd := &cobra.Command{Use: "x"}
	RegisterFlags(cmd, &c)
	if got := cmd.Flags().Lookup("port").DefValue; got != "8080" {
		t.Errorf("default of --port: %q", got)
	}
	if err := cmd.ParseFlags([]string{"--port", "1", "--level", "2", "--ratio", "3", "--label", "m", "--toggle=false", "--count", "4", "--names", "c"}); err != nil {
		t.Fatal(err)
	}
	if c.Port != 1 || c.Level != 2 || c.Ratio != 3 || c.Label != "m" || bool(c.Toggle) || c.Count != 4 || len(c.Names) != 1 {
		t.Fatalf("flags: %+v", c)
	}
	if err := cmd.ParseFlags([]string{"--level", "300"}); err == nil {
		t.Error("--level 300 overflows uint8")
	}
	t.Setenv("N_PORT", "70000")
	if err := LoadFromEnv(&namedCfg{}); err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Errorf("N_PORT=70000: %v", err)
	}
}

func TestEnvRefusesNonFiniteFloats(t *testing.T) {
	for _, v := range []string{"NaN", "Inf", "-Inf", "1e400"} {
		t.Setenv("N_RATIO", v)
		if err := LoadFromEnv(&namedCfg{}); err == nil {
			t.Errorf("N_RATIO=%s accepted", v)
		}
	}
}

func TestRegisterFlagsUnsupported(t *testing.T) {
	for name, cfg := range map[string]any{
		"slice of named strings": &struct {
			T []tag `flag:"t"`
		}{},
		"slice of ints": &struct {
			T []int `flag:"t"`
		}{},
		"map": &struct {
			T map[string]string `flag:"t"`
		}{},
	} {
		func() {
			defer func() {
				r := recover()
				if s, _ := r.(string); !strings.HasPrefix(s, "config.RegisterFlags: field T") {
					t.Errorf("%s: %v", name, r)
				}
			}()
			RegisterFlags(&cobra.Command{Use: "x"}, cfg)
		}()
	}
	if err := LoadFromEnv(namedCfg{}); err != nil {
		t.Errorf("LoadFromEnv of a value, not a pointer: %v", err)
	}
}
