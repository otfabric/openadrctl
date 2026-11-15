// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/errs"
)

func TestBudgetBounds(t *testing.T) {
	defer func(n int, b int64) { maxSearchObjects, maxSearchBytes = n, b }(maxSearchObjects, maxSearchBytes)
	maxSearchObjects, maxSearchBytes = 10, 1<<20
	obj := openadr.Program{Name: "p"}
	// takes counts how many objects a budget takes before it refuses one.
	takes := func(b *budget, v any) (int, error) {
		for i := 1; i <= 100; i++ {
			if err := b.take(v); err != nil {
				return i - 1, err
			}
		}
		return 100, nil
	}
	for _, c := range []struct {
		max, want int
		flag      bool
	}{
		{0, 10, false}, // the ceiling
		{1, 1, true},
		{9, 9, true},
		{10, 10, true}, // the flag at the ceiling: the flag's message
	} {
		n, err := takes(newBudget(c.max), obj)
		e, ok := errs.As(err)
		if n != c.want || !ok || e.Kind != errs.KindProtocol || e.Code != errs.CodeInvalidResponse ||
			strings.Contains(e.Message, "--max-objects") != c.flag {
			t.Errorf("--max-objects %d: took %d (want %d), %v", c.max, n, c.want, err)
		}
	}
	// Size: each object of 100 KiB as JSON, a ceiling of 1 MiB.
	maxSearchObjects = 1000
	big := openadr.Program{Name: strings.Repeat("a", 100<<10)}
	n, err := takes(newBudget(0), big)
	if e, ok := errs.As(err); n != 10 || !ok || !strings.Contains(e.Message, "more than 1 MiB") {
		t.Errorf("size: took %d, %v", n, err)
	}
}

func TestCheckMaxObjects(t *testing.T) {
	for n, ok := range map[int]bool{-1: false, 0: true, 1: true, maxSearchObjects - 1: true, maxSearchObjects: true, maxSearchObjects + 1: false} {
		if err := checkMaxObjects(n); (err == nil) != ok {
			t.Errorf("--max-objects %d: %v", n, err)
		}
	}
}

// pagedVTN answers GET /programs and GET /events with pages of fresh
// objects, total of them in all (-1: without end), and counts the pages.
type pagedVTN struct {
	*httptest.Server
	pages atomic.Int32
}

func newPagedVTN(t *testing.T, total int) *pagedVTN {
	t.Helper()
	v := &pagedVTN{}
	v.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.pages.Add(1)
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		n := limit
		if total >= 0 {
			n = max(0, min(limit, total-skip))
		}
		page := make([]map[string]any, 0, n)
		for i := range n {
			id := fmt.Sprintf("o-%d", skip+i)
			switch r.URL.Path {
			case "/programs":
				page = append(page, map[string]any{"id": id, "programName": "p" + id})
			case "/events":
				page = append(page, map[string]any{"id": id, "programID": "p", "eventName": "e" + id})
			default:
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	t.Cleanup(v.Close)
	return v
}

// execute runs a client command with args and returns its error and what it wrote.
func execute(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := cmd.ExecuteContext(ctx)
	return out.String(), err
}

// A VTN that keeps every page full: the search ends at the ceiling, after the
// pages that hold it, long before --timeout.
func TestListEndsAtTheCeiling(t *testing.T) {
	defer func(n int) { maxSearchObjects = n }(maxSearchObjects)
	maxSearchObjects = 120
	v := newPagedVTN(t, -1)
	_, err := execute(t, NewBLCmd(), "programs", "list", "--url", v.URL, "--max-objects", "0", "--timeout", "10s", "--format", "json")
	e, ok := errs.As(err)
	if !ok || e.Code != errs.CodeInvalidResponse || !strings.Contains(e.Message, "more than 120 objects") {
		t.Fatalf("list: %v", err)
	}
	if n := v.pages.Load(); n != 3 {
		t.Errorf("%d pages asked for, want 3", n)
	}
}

func TestWatchEndsAtTheCeiling(t *testing.T) {
	defer func(n int) { maxSearchObjects = n }(maxSearchObjects)
	maxSearchObjects = 120
	v := newPagedVTN(t, -1)
	out, err := execute(t, NewVENCmd(), "events", "watch", "--url", v.URL, "--max-objects", "0", "--timeout", "10s", "--format", "json")
	e, ok := errs.As(err)
	if !ok || e.Code != errs.CodeInvalidResponse || !strings.Contains(e.Message, "more than 120 objects") || out != "" {
		t.Fatalf("watch: %v\n%s", err, out)
	}
	if n := v.pages.Load(); n != 3 {
		t.Errorf("%d pages asked for, want 3", n)
	}
}

func TestMaxObjects(t *testing.T) {
	for _, c := range []struct {
		total, max int
		ok         bool
	}{
		{100, 100, true}, // exactly the limit
		{100, 0, true},
		{101, 100, false},
		{51, 50, false},
		{50, 50, true},
	} {
		v := newPagedVTN(t, c.total)
		out, err := execute(t, NewBLCmd(), "programs", "list", "--url", v.URL, "--max-objects", strconv.Itoa(c.max), "--format", "json")
		if c.ok {
			var got map[string][]json.RawMessage
			if err != nil || json.Unmarshal([]byte(out), &got) != nil || len(got["programs"]) != c.total {
				t.Errorf("%d objects, --max-objects %d: %v\n%.200s", c.total, c.max, err, out)
			}
			continue
		}
		if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidResponse || !strings.Contains(e.Message, "--max-objects") {
			t.Errorf("%d objects, --max-objects %d: %v", c.total, c.max, err)
		}
	}
	// One page (--limit) counts too.
	v := newPagedVTN(t, -1)
	if _, err := execute(t, NewBLCmd(), "programs", "list", "--url", v.URL, "--limit", "50", "--max-objects", "10"); err == nil {
		t.Error("--limit 50 --max-objects 10: no error")
	}
	// events watch: one search at the limit, then one past it.
	v = newPagedVTN(t, 50)
	if out, err := execute(t, NewVENCmd(), "events", "watch", "--url", v.URL, "--max-objects", "50", "--count", "50", "--format", "json"); err != nil || strings.Count(out, "\n") != 50 {
		t.Errorf("watch at the limit: %v, %d records", err, strings.Count(out, "\n"))
	}
	if _, err := execute(t, NewVENCmd(), "events", "watch", "--url", v.URL, "--max-objects", "49"); err == nil {
		t.Error("watch past the limit: no error")
	}
	for _, bad := range []string{"-1", strconv.Itoa(maxSearchObjects + 1)} {
		if _, err := execute(t, NewBLCmd(), "programs", "list", "--url", v.URL, "--max-objects", bad); err == nil {
			t.Errorf("--max-objects %s accepted", bad)
		}
	}
}

// --max-objects has a default: a search past it fails unless --max-objects
// 0 asks for up to the bound of every search.
func TestMaxObjectsDefault(t *testing.T) {
	defer func(n int) { defaultMaxObjects = n }(defaultMaxObjects)
	defaultMaxObjects = 60
	at, past := newPagedVTN(t, 60), newPagedVTN(t, 61)
	if _, err := execute(t, NewBLCmd(), "programs", "list", "--url", at.URL, "--format", "json"); err != nil {
		t.Errorf("at the default: %v", err)
	}
	if _, err := execute(t, NewBLCmd(), "programs", "list", "--url", past.URL); err == nil {
		t.Error("past the default: no error")
	} else if e, ok := errs.As(err); !ok || e.Code != errs.CodeInvalidResponse || !strings.Contains(e.Message, "more than 60 objects for one search (--max-objects)") {
		t.Errorf("past the default: %v", err)
	}
	if _, err := execute(t, NewVENCmd(), "events", "watch", "--url", past.URL, "--count", "1"); err == nil {
		t.Error("watch past the default: no error")
	}
	out, err := execute(t, NewBLCmd(), "programs", "list", "--url", past.URL, "--max-objects", "0", "--format", "json")
	var got map[string][]json.RawMessage
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || len(got["programs"]) != 61 {
		t.Errorf("--max-objects 0: %v, %d objects", err, len(got["programs"]))
	}
}
