// SPDX-License-Identifier: MIT

package oadr

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/internal/format"
)

// Kind describes how a kind of object is shown.
type Kind struct {
	// Name and Plural name the kind in messages and in JSON.
	Name, Plural string
	// Headers and Row are its columns in a table.
	Headers []string
	Row     func(v any) []string
	// Fields are its properties in text, after the ID, name and times.
	Fields func(v any) []Field
}

// Field is one line of the text form of an object.
type Field struct{ Name, Value string }

// One is a single object as a command's result. Action, when set, says what
// the command did to it ("created", "updated", "deleted").
type One struct {
	Kind   *Kind
	Value  any
	Action string
}

// MarshalJSON writes the object as OpenADR gives it.
func (o *One) MarshalJSON() ([]byte, error) { return Encode(o.Value) }

// MarshalTextOutput implements format.TextMarshaler.
func (o *One) MarshalTextOutput() (string, error) {
	row := o.Kind.Row(o.Value)
	if o.Action != "" {
		s := fmt.Sprintf("%s %s %s", o.Action, o.Kind.Name, row[0])
		if len(row) > 1 && row[1] != "" && row[1] != "-" {
			s += fmt.Sprintf(" (%s)", row[1])
		}
		return s, nil
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for i, h := range o.Kind.Headers {
		if row[i] != "" {
			_, _ = fmt.Fprintf(tw, "%s:\t%s\n", strings.ToLower(h), row[i])
		}
	}
	if o.Kind.Fields != nil {
		for _, f := range o.Kind.Fields(o.Value) {
			_, _ = fmt.Fprintf(tw, "%s:\t%s\n", f.Name, f.Value)
		}
	}
	m := MetadataOf(o.Value)
	if !m.Created.IsZero() {
		_, _ = fmt.Fprintf(tw, "created:\t%s\n", Time(m.Created))
		_, _ = fmt.Fprintf(tw, "modified:\t%s\n", Time(m.Modified))
	}
	_ = tw.Flush()
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (o *One) TableHeaders() []string { return o.Kind.Headers }

// TableRows implements format.TableMarshaler.
func (o *One) TableRows() [][]string { return [][]string{o.Kind.Row(o.Value)} }

// Many is the result of a search.
type Many struct {
	Kind   *Kind
	Values []any
}

// MarshalJSON writes {"<plural>": [...]}.
func (m *Many) MarshalJSON() ([]byte, error) {
	items := make([]json.RawMessage, 0, len(m.Values))
	for _, v := range m.Values {
		data, err := Encode(v)
		if err != nil {
			return nil, err
		}
		items = append(items, data)
	}
	return json.Marshal(map[string]any{m.Kind.jsonKey(): items})
}

func (k *Kind) jsonKey() string { return strings.ReplaceAll(k.Plural, " ", "_") }

// MarshalTextOutput implements format.TextMarshaler.
func (m *Many) MarshalTextOutput() (string, error) {
	if len(m.Values) == 0 {
		return "no " + m.Kind.Plural, nil
	}
	return format.RenderTable(m)
}

// TableHeaders implements format.TableMarshaler.
func (m *Many) TableHeaders() []string { return m.Kind.Headers }

// TableRows implements format.TableMarshaler.
func (m *Many) TableRows() [][]string {
	rows := make([][]string, 0, len(m.Values))
	for _, v := range m.Values {
		rows = append(rows, m.Kind.Row(v))
	}
	return rows
}

// MetadataOf returns the metadata of an object.
func MetadataOf(v any) openadr.Metadata {
	switch v := v.(type) {
	case openadr.Program:
		return v.Metadata
	case openadr.Event:
		return v.Metadata
	case openadr.Report:
		return v.Metadata
	case openadr.VEN:
		return v.Metadata
	case openadr.Resource:
		return v.Metadata
	case openadr.Subscription:
		return v.Metadata
	case openadr.ResourceGroup:
		return v.Metadata
	}
	return openadr.Metadata{}
}

// Time formats a time for people: RFC 3339 in UTC, "-" for none.
func Time(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

// List joins strings, "-" for none.
func List(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
}

func or(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Values formats the values of a payload.
func Values(vs []openadr.Value) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, v.String())
	}
	return strings.Join(parts, ",")
}

// Period formats an interval period: start, duration and randomization.
func Period(p *openadr.IntervalPeriod) string {
	if p == nil {
		return "-"
	}
	var parts []string
	if p.Start != nil {
		parts = append(parts, "start "+Time(*p.Start))
	}
	if p.Duration != nil {
		parts = append(parts, "duration "+p.Duration.String())
	}
	if p.RandomizeStart != nil {
		parts = append(parts, "randomize "+p.RandomizeStart.String())
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func valuesMaps(ms []openadr.ValuesMap) string {
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		parts = append(parts, m.Type+"="+Values(m.Values))
	}
	return List(parts)
}

func eventTypes(e openadr.Event) string {
	seen := map[string]bool{}
	var types []string
	for _, iv := range e.Intervals {
		for _, p := range iv.Payloads {
			if !seen[p.Type] {
				seen[p.Type] = true
				types = append(types, p.Type)
			}
		}
	}
	return List(types)
}

// Kinds of object.
var (
	Programs = &Kind{Name: "program", Plural: "programs",
		Headers: []string{"ID", "NAME", "TARGETS", "PAYLOADS"},
		Row: func(v any) []string {
			p := v.(openadr.Program)
			var payloads []string
			for _, d := range p.PayloadDescriptors {
				switch d := d.(type) {
				case openadr.EventPayloadDescriptor:
					payloads = append(payloads, d.PayloadType)
				case openadr.ReportPayloadDescriptor:
					payloads = append(payloads, d.PayloadType)
				}
			}
			return []string{p.ID, p.Name, List(p.Targets), List(payloads)}
		},
		Fields: func(v any) []Field {
			p := v.(openadr.Program)
			var urls []string
			for _, d := range p.Descriptions {
				urls = append(urls, d.URL)
			}
			return []Field{{"interval period", Period(p.IntervalPeriod)}, {"descriptions", List(urls)}, {"attributes", valuesMaps(p.Attributes)}}
		},
	}
	Events = &Kind{Name: "event", Plural: "events",
		Headers: []string{"ID", "NAME", "PROGRAM", "PRIORITY", "START", "INTERVALS", "TYPES"},
		Row: func(v any) []string {
			e := v.(openadr.Event)
			prio, start := "-", "-"
			if e.Priority != nil {
				prio = strconv.Itoa(*e.Priority)
			}
			if e.IntervalPeriod != nil && e.IntervalPeriod.Start != nil {
				start = Time(*e.IntervalPeriod.Start)
			} else if len(e.Intervals) > 0 && e.Intervals[0].IntervalPeriod != nil && e.Intervals[0].IntervalPeriod.Start != nil {
				start = Time(*e.Intervals[0].IntervalPeriod.Start)
			}
			return []string{e.ID, or(e.Name), e.ProgramID, prio, start, strconv.Itoa(len(e.Intervals)), eventTypes(e)}
		},
		Fields: func(v any) []Field {
			e := v.(openadr.Event)
			dur := "-"
			if e.Duration != nil {
				dur = e.Duration.String()
			}
			var reports []string
			for _, r := range e.ReportDescriptors {
				reports = append(reports, r.PayloadType)
			}
			return []Field{{"targets", List(e.Targets)}, {"interval period", Period(e.IntervalPeriod)}, {"duration", dur}, {"reports asked", List(reports)}}
		},
	}
	Reports = &Kind{Name: "report", Plural: "reports",
		Headers: []string{"ID", "NAME", "EVENT", "CLIENT_NAME", "RESOURCES"},
		Row: func(v any) []string {
			r := v.(openadr.Report)
			var res []string
			for _, x := range r.Resources {
				res = append(res, x.ResourceName)
			}
			return []string{r.ID, or(r.Name), r.EventID, r.ClientName, List(res)}
		},
		Fields: func(v any) []Field {
			r := v.(openadr.Report)
			fields := []Field{{"client id", or(r.ClientID)}}
			for _, x := range r.Resources {
				fields = append(fields, Field{"resource " + x.ResourceName, fmt.Sprintf("%d intervals", len(x.Intervals))})
			}
			return fields
		},
	}
	VENs = &Kind{Name: "VEN", Plural: "vens",
		Headers: []string{"ID", "NAME", "CLIENT_ID", "TARGETS"},
		Row: func(v any) []string {
			x := v.(openadr.VEN)
			return []string{x.ID, x.Name, or(x.ClientID), List(x.Targets)}
		},
		Fields: func(v any) []Field { return []Field{{"attributes", valuesMaps(v.(openadr.VEN).Attributes)}} },
	}
	Resources = &Kind{Name: "resource", Plural: "resources",
		Headers: []string{"ID", "NAME", "VEN", "CLIENT_ID", "TARGETS"},
		Row: func(v any) []string {
			x := v.(openadr.Resource)
			return []string{x.ID, x.Name, x.VENID, or(x.ClientID), List(x.Targets)}
		},
		Fields: func(v any) []Field { return []Field{{"attributes", valuesMaps(v.(openadr.Resource).Attributes)}} },
	}
	Subscriptions = &Kind{Name: "subscription", Plural: "subscriptions",
		Headers: []string{"ID", "CLIENT_NAME", "PROGRAM", "OBJECTS", "OPERATIONS", "DELIVERY"},
		Row: func(v any) []string {
			s := v.(openadr.Subscription)
			var objs, ops, via []string
			for _, oo := range s.ObjectOperations {
				for _, o := range oo.Objects {
					objs = append(objs, string(o))
				}
				for _, o := range oo.Operations {
					ops = append(ops, string(o))
				}
				switch oo.Mechanism {
				case openadr.MechanismWebSocket:
					via = append(via, "websocket")
				default:
					via = append(via, oo.CallbackURL)
				}
			}
			return []string{s.ID, s.ClientName, or(s.ProgramID), List(dedup(objs)), List(dedup(ops)), List(dedup(via))}
		},
		Fields: func(v any) []Field {
			s := v.(openadr.Subscription)
			return []Field{{"client id", or(s.ClientID)}, {"targets", List(s.Targets)}}
		},
	}
	ResourceGroups = &Kind{Name: "resource group", Plural: "resource groups",
		Headers: []string{"ID", "NAME", "CHILDREN", "TARGETS"},
		Row: func(v any) []string {
			g := v.(openadr.ResourceGroup)
			var ch []string
			for _, c := range g.Children {
				ch = append(ch, string(c.Type)+":"+c.ID)
			}
			return []string{g.ID, g.Name, List(ch), List(g.Targets)}
		},
		Fields: func(v any) []Field {
			return []Field{{"attributes", valuesMaps(v.(openadr.ResourceGroup).Attributes)}}
		},
	}
)

func dedup(s []string) []string {
	seen := map[string]bool{}
	out := s[:0:0]
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
