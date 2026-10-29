// SPDX-License-Identifier: MIT

package oadr

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/timeline"

	"github.com/otfabric/openadrctl/internal/format"
)

// Timeline is the intervals of an event as a table.
type Timeline struct {
	EventID string
	Table   timeline.Table
	// CSV makes the text form the CSV of the table.
	CSV bool
}

// TimelineRow is a row of a Timeline in JSON.
type TimelineRow struct {
	Interval       int32           `json:"interval"`
	Sub            int             `json:"sub"`
	Start          time.Time       `json:"start"`
	End            *time.Time      `json:"end,omitempty"`
	Type           string          `json:"type"`
	Values         json.RawMessage `json:"values"`
	RandomizeStart string          `json:"randomize_start,omitempty"`
}

func valuesJSON(vs []openadr.Value) json.RawMessage {
	parts := make([]any, 0, len(vs))
	for _, v := range vs {
		switch v.Kind() {
		case openadr.KindNumber:
			n, _ := v.Number()
			parts = append(parts, n)
		case openadr.KindBool:
			b, _ := v.Bool()
			parts = append(parts, b)
		case openadr.KindPoint:
			p, _ := v.Point()
			parts = append(parts, map[string]float64{"x": p.X, "y": p.Y})
		default:
			s, _ := v.Text()
			parts = append(parts, s)
		}
	}
	data, _ := json.Marshal(parts)
	return data
}

// MarshalJSON writes {"event": id, "rows": [...]}.
func (t *Timeline) MarshalJSON() ([]byte, error) {
	rows := make([]TimelineRow, 0, len(t.Table))
	for _, r := range t.Table {
		row := TimelineRow{Interval: r.IntervalID, Sub: r.Sub, Start: r.Start.UTC(), Type: r.Type, Values: valuesJSON(r.Values)}
		if !r.End.IsZero() {
			end := r.End.UTC()
			row.End = &end
		}
		if r.RandomizeStart != (openadr.Duration{}) {
			row.RandomizeStart = r.RandomizeStart.String()
		}
		rows = append(rows, row)
	}
	return json.Marshal(struct {
		Event string        `json:"event,omitempty"`
		Rows  []TimelineRow `json:"rows"`
	}{t.EventID, rows})
}

// MarshalTextOutput implements format.TextMarshaler.
func (t *Timeline) MarshalTextOutput() (string, error) {
	if t.CSV {
		var b bytes.Buffer
		if err := t.Table.WriteCSV(&b); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	if len(t.Table) == 0 {
		return "no intervals", nil
	}
	return format.RenderTable(t)
}

// TableHeaders implements format.TableMarshaler.
func (t *Timeline) TableHeaders() []string {
	return []string{"INTERVAL", "START", "END", "TYPE", "VALUES"}
}

// TableRows implements format.TableMarshaler.
func (t *Timeline) TableRows() [][]string {
	rows := make([][]string, 0, len(t.Table))
	for _, r := range t.Table {
		id := strconv.Itoa(int(r.IntervalID))
		if r.Sub > 0 {
			id += "." + strconv.Itoa(r.Sub)
		}
		end := "-"
		if !r.End.IsZero() {
			end = Time(r.End)
		}
		rows = append(rows, []string{id, Time(r.Start), end, r.Type, strings.TrimSpace(Values(r.Values))})
	}
	return rows
}
