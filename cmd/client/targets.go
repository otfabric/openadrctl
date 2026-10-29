// SPDX-License-Identifier: MIT

package client

import (
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"

	"github.com/otfabric/openadrctl/internal/oadr"
)

// The targets of OpenADR 3.1.0 are strings; those of 3.0.1 have a type,
// and --target is TYPE=value there. The flags set the strings; retarget
// and retargetQuery turn them into what 3.0.1 sends.

// typedTargets reads --target TYPE=value as 3.0.1 targets: one values map
// for each type, in the order the types first come.
func typedTargets(specs []string) ([]openadr.ValuesMap, error) {
	var out []openadr.ValuesMap
	at := map[string]int{}
	for _, s := range specs {
		typ, val, ok := strings.Cut(s, "=")
		if !ok || typ == "" || val == "" {
			return nil, bad("target", "%q: OpenADR 3.0.1 targets have a type: TYPE=value, such as GROUP=group-1", s)
		}
		i, seen := at[typ]
		if !seen {
			i = len(out)
			at[typ] = i
			out = append(out, openadr.ValuesMap{Type: typ})
		}
		out[i].Values = append(out[i].Values, openadr.String(val))
	}
	return out, nil
}

// retarget moves the --target strings of an object to its typed targets,
// with OpenADR 3.0.1. Without --target the object keeps the targets it has.
func retarget[T oadr.Object](spec openadr.Version, v *T) error {
	if spec != openadr.Version301 {
		return nil
	}
	move := func(plain *[]string, typed *[]openadr.ValuesMap) error {
		if len(*plain) == 0 {
			return nil
		}
		t, err := typedTargets(*plain)
		if err != nil {
			return err
		}
		*plain, *typed = nil, t
		return nil
	}
	switch o := any(v).(type) {
	case *openadr.Program:
		return move(&o.Targets, &o.TypedTargets)
	case *openadr.Event:
		return move(&o.Targets, &o.TypedTargets)
	case *openadr.VEN:
		return move(&o.Targets, &o.TypedTargets)
	case *openadr.Resource:
		return move(&o.Targets, &o.TypedTargets)
	case *openadr.Subscription:
		return move(&o.Targets, &o.TypedTargets)
	}
	return nil
}

// retargetQuery turns the --target strings of a search into a target type
// and its values, with OpenADR 3.0.1, which searches one type at a time.
func retargetQuery[Q any](spec openadr.Version, q *Q) error {
	if spec != openadr.Version301 {
		return nil
	}
	split := func(targets *[]string, typ *string) error {
		if len(*targets) == 0 {
			return nil
		}
		t, err := typedTargets(*targets)
		if err != nil {
			return err
		}
		if len(t) > 1 {
			return bad("target", "OpenADR 3.0.1 searches targets of one type at a time, not %s and %s", t[0].Type, t[1].Type)
		}
		*typ, *targets = t[0].Type, nil
		for _, v := range t[0].Values {
			s, _ := v.Text()
			*targets = append(*targets, s)
		}
		return nil
	}
	switch o := any(q).(type) {
	case *client.ProgramQuery:
		return split(&o.Targets, &o.TargetType)
	case *client.EventQuery:
		return split(&o.Targets, &o.TargetType)
	case *client.VENQuery:
		return split(&o.Targets, &o.TargetType)
	case *client.ResourceQuery:
		return split(&o.Targets, &o.TargetType)
	}
	return nil
}
