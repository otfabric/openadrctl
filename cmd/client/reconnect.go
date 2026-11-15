// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/otfabric/openadrctl/internal/conn"
)

// lockedWriter is the progress of listen, which the subscribers write from
// their goroutines while the command writes it from its own.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// warnings is the handler of the Logger listen gives a subscriber: an entry
// at warning level or above is one line on progress, "warning: <message>:
// <error>", escaped as text from a peer is. A stack is left out. note, when
// set, is told of every such entry, with its error, if any.
type warnings struct {
	w    io.Writer
	note func(msg string, err error)
}

func (h *warnings) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelWarn }

func (h *warnings) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	var err error
	r.Attrs(func(a slog.Attr) bool {
		v := a.Value.Resolve()
		switch a.Key {
		case "stack":
		case "error":
			if e, ok := v.Any().(error); ok {
				err = e
			}
			b.WriteString(": " + v.String())
		default:
			b.WriteString(" " + a.Key + "=" + v.String())
		}
		return true
	})
	_, _ = fmt.Fprintf(h.w, "warning: %s\n", conn.Clean(b.String()))
	if h.note != nil {
		h.note(r.Message, err)
	}
	return nil
}

func (h *warnings) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *warnings) WithGroup(string) slog.Handler      { return h }

// The texts with which go-openadr's MQTT subscriber reports that the broker
// refused the login (CONNACK return codes 4 and 5), and the entry it makes
// of a connection that failed.
const (
	mqttCannotConnect = "mqtt: cannot connect to the broker"
	mqttBadLogin      = "mqtt: the broker refused the connection: bad user name or password"
	mqttNotAuthorized = "mqtt: the broker refused the connection: not authorized"
)

// refusedLogin tells whether an entry of the MQTT subscriber is a login the
// broker refused.
func refusedLogin(msg string, err error) bool {
	return msg == mqttCannotConnect && err != nil && (err.Error() == mqttBadLogin || err.Error() == mqttNotAuthorized)
}

// brokerWatch is what the MQTT subscribers of listen report, through their
// Logger and Metrics, for the loop of runMQTT. Each subscriber is one
// generation: the reports of one the loop has replaced are ignored. The
// callbacks do not block: they note what happened and wake the loop.
type brokerWatch struct {
	progress io.Writer
	shown    string // the broker, as it is printed
	filters  string // the topic filters, as they are printed

	mu       sync.Mutex
	gen      int  // the subscriber whose reports count
	sessions int  // of every subscriber so far
	up       bool // the current subscriber has been subscribed
	lost     bool // ... and its connection was lost since the loop looked
	refused  error
	last     error // the latest failure of the current subscriber
	wake     chan struct{}
}

func newBrokerWatch(progress io.Writer, shown, filters string) *brokerWatch {
	return &brokerWatch{progress: progress, shown: shown, filters: filters, wake: make(chan struct{}, 1)}
}

// next starts a generation, and returns what its subscriber reports to.
func (w *brokerWatch) next() (*slog.Logger, *brokerReport) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gen++
	w.up, w.lost, w.refused, w.last = false, false, nil, nil
	r := &brokerReport{w: w, gen: w.gen}
	return slog.New(&warnings{w: w.progress, note: r.noted}), r
}

// state is what the loop reads: whether the current subscriber has been
// subscribed, and the lost connection and the refused login it reported
// since the loop looked last, which it takes.
func (w *brokerWatch) state() (up, lost bool, refused, last error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	up, lost, refused, last = w.up, w.lost, w.refused, w.last
	w.lost, w.refused = false, nil
	return up, lost, refused, last
}

func (w *brokerWatch) poke() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// brokerReport is the Metrics of one subscriber, and the note of its
// Logger.
type brokerReport struct {
	w   *brokerWatch
	gen int
}

// current runs f under the lock when r is the current generation, and
// reports whether it was.
func (r *brokerReport) current(f func()) bool {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if r.gen != r.w.gen {
		return false
	}
	f()
	return true
}

func (r *brokerReport) noted(msg string, err error) {
	if r.current(func() {
		if err != nil {
			r.w.last = err
		}
		if refusedLogin(msg, err) && r.w.refused == nil {
			r.w.refused = err
		}
	}) {
		r.w.poke()
	}
}

func (r *brokerReport) OnConnected() {
	again := false
	if !r.current(func() {
		again = r.w.sessions > 0
		r.w.sessions++
		r.w.up, r.w.last = true, nil
	}) {
		return
	}
	if again {
		_, _ = fmt.Fprintf(r.w.progress, "listening: %s again, %s\n", r.w.shown, r.w.filters)
	} else {
		_, _ = fmt.Fprintf(r.w.progress, "listening: %s, %s\n", r.w.shown, r.w.filters)
	}
	r.w.poke()
}

func (r *brokerReport) OnDisconnected(error) {
	if r.current(func() { r.w.lost = true }) {
		r.w.poke()
	}
}

func (r *brokerReport) OnPublished() {}
func (r *brokerReport) OnDropped()   {}
func (r *brokerReport) OnReceived()  {}

// streamReport is the Metrics of the WebSocket subscriber of listen: a
// WebSocket opened again after the first is a line on progress. That it was
// lost is the Logger's warning.
type streamReport struct {
	progress io.Writer
	mu       sync.Mutex
	sessions int
}

func (r *streamReport) OnConnected() {
	r.mu.Lock()
	r.sessions++
	again := r.sessions > 1
	r.mu.Unlock()
	if again {
		_, _ = fmt.Fprintln(r.progress, "listening: WebSocket open again")
	}
}

func (r *streamReport) OnDisconnected(error) {}
func (r *streamReport) OnReceived()          {}
