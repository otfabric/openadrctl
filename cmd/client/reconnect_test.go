// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
)

func TestWarningsAreOneLineEach(t *testing.T) {
	var out bytes.Buffer
	var noted []string
	log := slog.New(&warnings{w: &out, note: func(msg string, err error) { noted = append(noted, msg) }})
	log.Info("not shown")
	log.Warn("mqtt: a message that is not a notification", slog.String("topic", "t\x1b[2J"), slog.Any("error", errors.New("a\nb")))
	log.With("ignored", 1).Error("mqtt: callback did not return normally", slog.String("callback", "Notify"), slog.String("stack", "goroutine 1\n..."))
	want := "warning: mqtt: a message that is not a notification topic=t\\x1b[2J: a\\nb\n" +
		"warning: mqtt: callback did not return normally callback=Notify\n"
	if out.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", out.String(), want)
	}
	if len(noted) != 2 {
		t.Errorf("noted: %v", noted)
	}
}

func TestRefusedLogin(t *testing.T) {
	for _, c := range []struct {
		msg string
		err error
		ok  bool
	}{
		{mqttCannotConnect, errors.New(mqttBadLogin), true},
		{mqttCannotConnect, errors.New(mqttNotAuthorized), true},
		{mqttCannotConnect, errors.New("mqtt: the broker refused the connection: server unavailable"), false},
		{mqttCannotConnect, errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), false},
		{mqttCannotConnect, nil, false},
		{"mqtt: connection to the broker lost", errors.New(mqttBadLogin), false},
	} {
		if got := refusedLogin(c.msg, c.err); got != c.ok {
			t.Errorf("%q, %v: %v", c.msg, c.err, got)
		}
	}
}

// What a subscriber that was replaced reports is ignored.
func TestBrokerWatchIgnoresReplacedSubscribers(t *testing.T) {
	var out bytes.Buffer
	w := newBrokerWatch(&out, "mqtt://b", "events/+")
	_, old := w.next()
	old.OnConnected()
	if up, _, _, _ := w.state(); !up {
		t.Fatal("not up after OnConnected")
	}
	logger, cur := w.next()
	old.OnDisconnected(nil)
	old.noted(mqttCannotConnect, errors.New(mqttBadLogin))
	old.OnConnected()
	if up, lost, refused, last := w.state(); up || lost || refused != nil || last != nil {
		t.Errorf("after reports of the replaced one: %v %v %v %v", up, lost, refused, last)
	}
	logger.Warn(mqttCannotConnect, slog.Any("error", errors.New(mqttNotAuthorized)))
	cur.OnDisconnected(nil)
	if _, lost, refused, _ := w.state(); !lost || refused == nil {
		t.Errorf("the current one: lost %v, refused %v", lost, refused)
	}
	// Taken: not reported twice.
	if _, lost, refused, last := w.state(); lost || refused != nil || last == nil {
		t.Errorf("a second look: lost %v, refused %v, last %v", lost, refused, last)
	}
	cur.OnConnected()
	cur.OnPublished()
	cur.OnDropped()
	cur.OnReceived()
	want := "listening: mqtt://b, events/+\n" +
		"warning: mqtt: cannot connect to the broker: mqtt: the broker refused the connection: not authorized\n" +
		"listening: mqtt://b again, events/+\n"
	if out.String() != want {
		t.Errorf("progress:\n%s", out.String())
	}
}
