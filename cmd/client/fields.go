// SPDX-License-Identifier: MIT

package client

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/timeline"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
)

// bad is invalid input in a flag.
func bad(flag string, format string, a ...any) error {
	return errs.InvalidInput(errs.CodeInvalidInput, "--"+flag+": "+fmt.Sprintf(format, a...), nil)
}

// parseValue reads a value of a payload: a number, true or false, x:y for a
// point, or else a string.
//
// A number is written in decimal, as JSON writes numbers (a sign, a
// leading or trailing point and an exponent are taken too): what is not,
// such as 0x1p3, Inf, NaN or 1_000, is text, as OpenADR allows a value to
// be. A decimal no float64 holds, such as 1e400, is refused rather than
// sent as text.
func parseValue(flag, s string) (openadr.Value, error) {
	if f, ok, err := parseNumber(flag, s); ok || err != nil {
		return openadr.Number(f), err
	}
	if s == "true" || s == "false" {
		return openadr.Bool(s == "true"), nil
	}
	if x, y, found := strings.Cut(s, ":"); found {
		fx, okx, errx := parseNumber(flag, x)
		fy, oky, erry := parseNumber(flag, y)
		if errx != nil || erry != nil {
			return openadr.Value{}, cmp.Or(errx, erry)
		}
		if okx && oky {
			return openadr.PointValue(fx, fy), nil
		}
	}
	return openadr.String(s), nil
}

// decimal is the notation of a number of a value.
var decimal = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// parseNumber reads s as a number when it is written as a decimal one; ok
// is false when it is not. A decimal beyond the range of a float64 is an
// error.
func parseNumber(flag, s string) (f float64, ok bool, err error) {
	if !decimal.MatchString(s) {
		return 0, false, nil
	}
	f, err = strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, bad(flag, "%q is a number larger than a value holds (%.3g)", s, math.MaxFloat64)
	}
	return f, true, nil
}

// parseValuesMaps reads TYPE=v1,v2 flags.
func parseValuesMaps(flag string, specs []string) ([]openadr.ValuesMap, error) {
	var out []openadr.ValuesMap
	for _, s := range specs {
		typ, vals, ok := strings.Cut(s, "=")
		if !ok || typ == "" {
			return nil, bad(flag, "%q: TYPE=value[,value...]", s)
		}
		m := openadr.ValuesMap{Type: typ}
		for v := range strings.SplitSeq(vals, ",") {
			value, err := parseValue(flag, strings.TrimSpace(v))
			if err != nil {
				return nil, err
			}
			m.Values = append(m.Values, value)
		}
		out = append(out, m)
	}
	return out, nil
}

// parseTime reads "now" or an RFC 3339 time.
func parseTime(flag, s string) (time.Time, error) {
	if strings.EqualFold(s, "now") {
		return time.Now().UTC().Truncate(time.Second), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, bad(flag, "%q: now, or an RFC 3339 time such as 2030-01-01T00:00:00Z", s)
	}
	return t, nil
}

// parseDuration reads an ISO 8601 duration (PT1H) or a Go one (1h30m). A
// length is not negative: -PT1H is refused, as -1h is.
func parseDuration(flag, s string) (openadr.Duration, error) {
	if d, err := openadr.ParseDuration(s); err == nil {
		if d.Negative {
			return openadr.Duration{}, bad(flag, "%q: a duration must not be negative", s)
		}
		return d, nil
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return openadr.DurationOf(d), nil
	}
	return openadr.Duration{}, bad(flag, "%q: an ISO 8601 duration such as PT1H, or 1h30m", s)
}

// intervals are the flags that lay out intervals: one per --value, or a
// timeline CSV.
type intervals struct {
	typ      string
	values   []string
	start    string
	duration string
	csv      string
}

func (iv *intervals) register(cmd *cobra.Command, what string) {
	f := cmd.Flags()
	f.StringVar(&iv.typ, "type", "", "Payload type of the "+what+", such as PRICE, SIMPLE or USAGE")
	f.StringSliceVar(&iv.values, "value", nil, "A value per interval, in order (repeatable; a decimal number, true/false, x:y or text)")
	f.StringVar(&iv.start, "start", "", "Start of the first interval: now, or RFC 3339")
	f.StringVar(&iv.duration, "interval", "", "Length of each interval: PT15M, 1h, ...")
	f.StringVar(&iv.csv, "csv", "", "Intervals from a timeline CSV (interval,start,end,type,values; see openadrctl timeline)")
	_ = cmd.MarkFlagFilename("csv", "csv")
}

func (iv *intervals) given(cmd *cobra.Command) bool {
	for _, n := range []string{"type", "value", "start", "interval", "csv"} {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// build returns the default period and the intervals the flags describe.
func (iv *intervals) build() (*openadr.IntervalPeriod, []openadr.Interval, error) {
	if iv.csv != "" {
		if len(iv.values) > 0 {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidFlagCombination, "give --value or --csv, not both", nil)
		}
		f, err := os.Open(iv.csv)
		if err != nil {
			return nil, nil, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("--csv %s: %v", iv.csv, err), err)
		}
		defer func() { _ = f.Close() }()
		table, err := timeline.ReadCSV(oadr.LimitReader(f))
		if errors.Is(err, oadr.ErrTooLarge) {
			return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--csv %s: %v", iv.csv, oadr.ErrTooLarge), err)
		}
		if err != nil {
			return nil, nil, errs.InvalidInput(errs.CodeInputDecodeFailed, fmt.Sprintf("--csv %s: %v", iv.csv, err), err)
		}
		return nil, table.Intervals(), nil
	}
	if len(iv.values) == 0 {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, "give --value (one per interval) with --type, or --csv", nil)
	}
	if iv.typ == "" {
		return nil, nil, errs.InvalidInput(errs.CodeInvalidInput, "--type names the payload of --value", nil)
	}
	period := &openadr.IntervalPeriod{}
	if iv.start != "" {
		t, err := parseTime("start", iv.start)
		if err != nil {
			return nil, nil, err
		}
		period.Start = &t
	}
	if iv.duration != "" {
		d, err := parseDuration("interval", iv.duration)
		if err != nil {
			return nil, nil, err
		}
		period.Duration = &d
	}
	if period.Start == nil && period.Duration == nil {
		period = nil
	}
	out := make([]openadr.Interval, 0, len(iv.values))
	for i, v := range iv.values {
		value, err := parseValue("value", v)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, openadr.Interval{ID: int32(i), Payloads: []openadr.ValuesMap{{Type: iv.typ, Values: []openadr.Value{value}}}})
	}
	return period, out, nil
}

// stringFlag registers a string flag and returns a setter that applies it
// when it was given.
func stringFlag(cmd *cobra.Command, name, usage string) func(set func(string)) {
	var v string
	cmd.Flags().StringVar(&v, name, "", usage)
	return func(set func(string)) {
		if cmd.Flags().Changed(name) {
			set(v)
		}
	}
}

// listFlag is stringFlag for a repeatable flag.
func listFlag(cmd *cobra.Command, name, usage string) func(set func([]string)) {
	var v []string
	cmd.Flags().StringSliceVar(&v, name, nil, usage)
	return func(set func([]string)) {
		if cmd.Flags().Changed(name) {
			set(v)
		}
	}
}

// attributesFlag registers --attribute TYPE=v1,v2.
func attributesFlag(cmd *cobra.Command, name string) func(set func([]openadr.ValuesMap)) error {
	var v []string
	cmd.Flags().StringArrayVar(&v, name, nil, "Attribute as TYPE=value[,value...] (repeatable)")
	return func(set func([]openadr.ValuesMap)) error {
		if !cmd.Flags().Changed(name) {
			return nil
		}
		m, err := parseValuesMaps(name, v)
		if err != nil {
			return err
		}
		set(m)
		return nil
	}
}
