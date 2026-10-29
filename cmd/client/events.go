// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
	"github.com/otfabric/go-openadr/timeline"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/runner"
)

// TimelineFlags are the options of a timeline.
type TimelineFlags struct {
	CSV          bool   `flag:"csv" desc:"Write the table as CSV (the form --csv of events create reads)"`
	SubIntervals bool   `flag:"sub-intervals" desc:"Split a payload with several values into intervals of equal length"`
	At           string `flag:"at" desc:"Only the rows in force at this time: now, or RFC 3339"`
	Until        string `flag:"until" desc:"Lay out an event that repeats without end up to this time (RFC 3339)"`
}

// Table lays out an event as the flags say.
func (f TimelineFlags) Table(e openadr.Event) (*oadr.Timeline, error) {
	return f.TableAt(e, time.Time{})
}

// TableAt is Table with "now" in the event standing for now; the current
// time when zero.
func (f TimelineFlags) TableAt(e openadr.Event, now time.Time) (*oadr.Timeline, error) {
	o := timeline.Options{Now: now, SubIntervals: f.SubIntervals}
	if f.Until != "" {
		t, err := parseTime("until", f.Until)
		if err != nil {
			return nil, err
		}
		o.Until = t
	}
	table, err := timeline.OfEvent(e, o)
	if err != nil {
		return nil, errs.New(errs.KindProtocol, errs.CodeInvalidResponse, "the intervals of the event: "+err.Error(), err)
	}
	if f.At != "" {
		t, err := parseTime("at", f.At)
		if err != nil {
			return nil, err
		}
		table = table.At(t)
	}
	return &oadr.Timeline{EventID: e.ID, Table: table, CSV: f.CSV}, nil
}

type timelineConfig struct {
	Common
	TimelineFlags
}

func newTimelineCmd(role Role) *cobra.Command {
	c := timelineConfig{Common: newCommon()}
	cmd := &cobra.Command{
		Use:   "timeline <event-id>",
		Short: "Lay out the intervals of an event in time",
		Long: `Read an event and lay out its intervals as a table of start, end, type and values, with
the timing rules of the OpenADR User Guide: defaults of the event, "now", intervals that
follow each other, the duration of the event. --csv writes the table as CSV, which events
create --csv and reports create --csv read back.`,
		Example: "  openadrctl " + string(role) + " events timeline 4e5f6a7b\n" +
			"  openadrctl " + string(role) + " events timeline 4e5f6a7b --at now\n" +
			"  openadrctl " + string(role) + " events timeline 4e5f6a7b --csv > prices.csv",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return once(cmd, role, &c.Common, func() error { return checkID(args[0]) }, false, func(ctx context.Context, a *api) (any, error) {
				e, err := pick(a).Event(ctx, args[0])
				if err != nil {
					return nil, err
				}
				return c.Table(e)
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

type watchConfig struct {
	Common
	Interval    time.Duration `flag:"interval" desc:"Time between two searches"`
	Count       int           `flag:"count" desc:"Stop after this many records (0: no limit)"`
	Duration    time.Duration `flag:"duration" desc:"Stop after this long (0: until interrupted)"`
	ChangesOnly bool          `flag:"changes-only" desc:"Leave out the events that are there at the start"`
}

// Change is a record of events watch.
type Change struct {
	Time   time.Time       `json:"time"`
	Change string          `json:"change"`
	ID     string          `json:"id"`
	Name   string          `json:"name,omitempty"`
	Event  json.RawMessage `json:"event"`
	row    []string
}

// MarshalTextOutput implements format.TextMarshaler.
func (c *Change) MarshalTextOutput() (string, error) {
	s := fmt.Sprintf("%s  %-8s event %s", c.Time.Format(time.RFC3339), c.Change, c.ID)
	if c.Name != "" {
		s += " (" + c.Name + ")"
	}
	if c.row[4] != "-" {
		s += "  start " + c.row[4]
	}
	return s + "  " + c.row[5] + " intervals " + c.row[6], nil
}

// TableHeaders implements format.TableMarshaler.
func (c *Change) TableHeaders() []string {
	return []string{"TIME", "CHANGE", "ID", "NAME", "PROGRAM", "START", "INTERVALS", "TYPES"}
}

// TableRows implements format.TableMarshaler.
func (c *Change) TableRows() [][]string {
	return [][]string{{c.Time.Format(time.RFC3339), c.Change, c.ID, c.row[1], c.row[2], c.row[4], c.row[5], c.row[6]}}
}

func newWatchCmd(role Role) *cobra.Command {
	c := watchConfig{Common: newCommon(), Interval: 30 * time.Second}
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Search for events again and again, and print what changed",
		Long: `Poll the VTN: search for events every --interval, and print a record for each event that
is there at the start (present, unless --changes-only), and for each that is created,
updated (its modification time changed) or deleted after that. It needs no subscription,
webhook or broker, and works with every VTN; listen is the way to learn of changes at once.

A search that fails ends the command with that failure's exit code. It exits 0 when it ends
by --count, --duration or an interrupt.`,
		Example: "  openadrctl ven events watch --interval 1m --target group-a\n" +
			"  openadrctl bl events watch --program 0a1b2c3d --changes-only --format json",
		Args: cobra.NoArgs,
	}
	query := eventQuery(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runner.RunStream(cmd, c.Format, func() error {
			if err := c.Validate(); err != nil {
				return err
			}
			if c.Interval < time.Second || c.Count < 0 || c.Duration < 0 {
				return errs.InvalidInput(errs.CodeInvalidInput, "--interval is 1s at least, and --count and --duration not negative", nil)
			}
			return nil
		}, func(ctx context.Context, out *format.StreamWriter) (int, error) {
			a, err := c.connect(role)
			if err != nil {
				return 0, err
			}
			if c.Duration > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, c.Duration)
				defer cancel()
			}
			s := newSink(out, c.Count)
			go c.poll(ctx, a, query, s)
			return s.wait(ctx)
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

func (c *watchConfig) poll(ctx context.Context, a *api, query func(client.Page) client.EventQuery, s *sink) {
	known := map[string]openadr.Event{}
	first := true
	for {
		sctx, cancel := context.WithTimeout(ctx, c.Timeout)
		now := map[string]openadr.Event{}
		var order []string
		var failed error
		for e, err := range client.Each(sctx, func(ctx context.Context, p client.Page) ([]openadr.Event, error) {
			return pick(a).Events(ctx, query(p))
		}) {
			if err != nil {
				failed = err
				break
			}
			now[e.ID] = e
			order = append(order, e.ID)
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		if failed != nil {
			s.fail(conn.Classify(failed, false))
			return
		}
		emit := func(change string, e openadr.Event) {
			s.write(&Change{Time: time.Now().UTC(), Change: change, ID: e.ID, Name: e.Name, Event: oadr.MustEncode(e), row: oadr.Events.Row(e)})
		}
		for _, id := range order {
			e := now[id]
			old, ok := known[id]
			switch {
			case first && !c.ChangesOnly:
				emit("present", e)
			case first:
			case !ok:
				emit("created", e)
			case !old.Modified.Equal(e.Modified):
				emit("updated", e)
			}
		}
		if !first {
			gone := make([]string, 0)
			for id := range known {
				if _, ok := now[id]; !ok {
					gone = append(gone, id)
				}
			}
			sort.Strings(gone)
			for _, id := range gone {
				emit("deleted", known[id])
			}
		}
		known, first = now, false
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-time.After(c.Interval):
		}
	}
}
