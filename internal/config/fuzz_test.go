// SPDX-License-Identifier: MIT

package config

import (
	"math"
	"strings"
	"testing"
)

// FuzzLoadFromEnv feeds one value to every kind of field: LoadFromEnv sets
// it or returns an error, never panics, and never sets a float that is not
// finite.
func FuzzLoadFromEnv(f *testing.F) {
	for _, s := range []string{"1", "-1", "300", "70000", "1.5", "NaN", "Inf", "true", "junk", "90s", "-1h", "a,b", " , ", "9223372036854775808"} {
		f.Add(s)
	}
	keys := []string{"T_NAME", "T_B", "T_U8", "T_U16", "T_U32", "T_U64", "T_U", "T_I", "T_I8", "T_I16", "T_I32", "T_I64", "T_F", "T_D", "T_S",
		"N_PORT", "N_LEVEL", "N_RATIO", "N_LABEL", "N_TOGGLE", "N_COUNT", "N_NAMES", "N_TAGS"}
	f.Fuzz(func(t *testing.T, s string) {
		if s == "" || strings.ContainsRune(s, 0) {
			return // unset, or no environment variable can hold it
		}
		for _, k := range keys {
			t.Setenv(k, s)
		}
		var c cfg
		_ = LoadFromEnv(&c)
		var n namedCfg
		_ = LoadFromEnv(&n)
		if math.IsNaN(c.F) || math.IsInf(c.F, 0) || math.IsNaN(float64(n.Ratio)) || math.IsInf(float64(n.Ratio), 0) {
			t.Fatalf("%q set a float that is not finite", s)
		}
	})
}
