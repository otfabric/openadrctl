// SPDX-License-Identifier: MIT

package client

import (
	"encoding/json"
	"testing"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/internal/errs"
)

// A value is a number only when written as a decimal one, as JSON writes
// numbers; anything else that is not true, false or x:y is text. A decimal
// no float64 holds is refused, not sent as text.
func TestParseValue(t *testing.T) {
	for in, want := range map[string]string{
		"0.35":     `0.35`,
		"-2":       `-2`,
		"+5":       `5`,
		".5":       `0.5`,
		"5.":       `5`,
		"1E5":      `100000`,
		"007":      `7`,
		"1e-400":   `0`,
		"true":     `true`,
		"false":    `false`,
		"True":     `"True"`,
		"1:2.5":    `{"x":1,"y":2.5}`,
		"0x1p3":    `"0x1p3"`,
		"Inf":      `"Inf"`,
		"-inf":     `"-inf"`,
		"NaN":      `"NaN"`,
		"infinity": `"infinity"`,
		"1_000":    `"1_000"`,
		"0x1:2":    `"0x1:2"`,
		"NaN:1":    `"NaN:1"`,
		"12:30:00": `"12:30:00"`,
		"":         `""`,
		"high":     `"high"`,
	} {
		v, err := parseValue("value", in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := asJSON(v); got != want {
			t.Errorf("%q = %s, want %s", in, got, want)
		}
	}
	for _, in := range []string{"1e400", "-1e400", "1e400:1", "1:-1e309"} {
		_, err := parseValue("value", in)
		if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidInput || e.Kind != errs.KindInvalidInput {
			t.Errorf("%q: %v, want invalid_input", in, err)
		}
	}
	if _, err := parseValuesMaps("attribute", []string{"LIMIT=1,1e400"}); err == nil {
		t.Error("an attribute of 1e400 was taken")
	}
}

// asJSON writes v as a value of a valuesMap is written.
func asJSON(v openadr.Value) string {
	var x any
	switch v.Kind() {
	case openadr.KindNumber:
		x, _ = v.Number()
	case openadr.KindBool:
		x, _ = v.Bool()
	case openadr.KindString:
		x, _ = v.Text()
	case openadr.KindPoint:
		p, _ := v.Point()
		x = map[string]float64{"x": p.X, "y": p.Y}
	}
	b, err := json.Marshal(x)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}
