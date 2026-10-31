// SPDX-License-Identifier: MIT

package vtn

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/internal/format"
)

func TestRecordDetail(t *testing.T) {
	ms, n := int64(7), 3
	for _, c := range []struct {
		r    Record
		want string
	}{
		{Record{Event: "listening", URL: "http://x", Address: "1:2"}, "http://x on 1:2"},
		{Record{Event: "client", Client: "c", Role: "bl"}, "c (bl)"},
		{Record{Event: "request", Op: "GET /x", Status: 200, Millis: &ms}, "GET /x 200 7ms"},
		{Record{Event: "notification", Mechanism: "webhook", Object: "EVENT", Operation: "POST"}, "webhook EVENT POST"},
		{Record{Event: "seeded", Count: &n}, "3 objects"},
		{Record{Event: "log", Level: "warn", Message: "m"}, "warn m"},
		{Record{Event: "mqtt_login", Client: "c", Status: http.StatusOK}, "c allowed"},
		{Record{Event: "mqtt_login", Client: "c", Status: http.StatusForbidden}, "c denied"},
		{Record{Event: "socket_closed", Message: "bye"}, "bye"},
	} {
		if got := c.r.detail(); got != c.want {
			t.Errorf("%s: %q, want %q", c.r.Event, got, c.want)
		}
	}
	r := Record{Time: time.Date(2026, 10, 31, 1, 2, 3, 0, time.UTC), Event: "socket_opened"}
	if s, _ := r.MarshalTextOutput(); s != "2026-10-31T01:02:03Z  socket_opened" {
		t.Errorf("text %q", s)
	}
	if row := r.TableRows()[0]; len(row) != len(r.TableHeaders()) || row[1] != "socket_opened" {
		t.Errorf("row %q", row)
	}
}

// records runs f on a recorder that writes JSON lines and returns the records.
func records(t *testing.T, logRequests bool, f func(*recorder)) []Record {
	t.Helper()
	var buf bytes.Buffer
	r := &recorder{out: format.NewStreamWriter(&buf, format.FormatJSON), logRequests: logRequests}
	f(r)
	if r.err != nil {
		t.Fatal(r.err)
	}
	var recs []Record
	dec := json.NewDecoder(&buf)
	for dec.More() {
		var rec Record
		if err := dec.Decode(&rec); err != nil {
			t.Fatal(err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestRecorderIsTheMetricsOfTheServer(t *testing.T) {
	recs := records(t, true, func(r *recorder) {
		r.OnRequest("op")
		r.OnResponse("op", 201, 5*time.Millisecond)
		r.OnNotification("mqtt", openadr.ObjectType("EVENT"), openadr.Operation("PUT"))
		r.OnSocketOpened()
		r.OnSocketClosed("gone")
	})
	if len(recs) != 4 {
		t.Fatalf("%+v", recs)
	}
	if recs[0].Event != "request" || recs[0].Status != 201 || *recs[0].Millis != 5 {
		t.Errorf("request %+v", recs[0])
	}
	if recs[1].Mechanism != "mqtt" || recs[1].Object != "EVENT" || recs[1].Operation != "PUT" {
		t.Errorf("notification %+v", recs[1])
	}
	if recs[2].Event != "socket_opened" || recs[3].Message != "gone" || recs[3].Time.IsZero() {
		t.Errorf("sockets %+v", recs[2:])
	}
	if recs := records(t, false, func(r *recorder) { r.OnResponse("op", 200, 0) }); len(recs) != 0 {
		t.Errorf("requests logged without --log-requests: %+v", recs)
	}
}

func TestLogHandlerRecordsWarnings(t *testing.T) {
	recs := records(t, false, func(r *recorder) {
		h := logHandler{r: r}
		if h.Enabled(context.Background(), slog.LevelInfo) || !h.Enabled(context.Background(), slog.LevelWarn) {
			t.Error("the level is not warn")
		}
		l := slog.New(h.WithGroup("ignored").WithAttrs([]slog.Attr{slog.String("a", "1")}))
		l.Info("dropped")
		l.Warn("low disk", "b", 2)
	})
	if len(recs) != 1 || recs[0].Level != "warn" || recs[0].Message != "low disk a=1 b=2" {
		t.Fatalf("%+v", recs)
	}
}

func TestAdvertiseNeedsAPort(t *testing.T) {
	for _, public := range []string{"::not a url", "ftp://example.com"} {
		if _, err := advertise("i", public, false, slog.Default()); err == nil {
			t.Errorf("%s: no error", public)
		} else if strings.Contains(public, "ftp") && !strings.Contains(err.Error(), "no port") {
			t.Errorf("%s: %v", public, err)
		}
	}
}
