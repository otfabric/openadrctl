// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
)

func TestSinkEnds(t *testing.T) {
	// A handler still running when the stream ended (a webhook request the
	// server's Close does not wait for) writes nothing after it.
	var out bytes.Buffer
	s := newSink(format.NewStreamWriter(&out, format.FormatJSON), 0)
	ctx, cancel := context.WithCancel(context.Background())
	s.write(map[string]int{"n": 1})
	cancel()
	if code, err := s.wait(ctx); code != errs.ExitOK || err != nil {
		t.Fatalf("wait: %d %v", code, err)
	}
	before := out.String()
	s.write(map[string]int{"n": 2})
	s.fail(errors.New("late"))
	if out.String() != before || s.failure != nil {
		t.Errorf("written after the end: %q, failure %v", out.String(), s.failure)
	}
}

func TestSinkCountAndFailure(t *testing.T) {
	var out bytes.Buffer
	s := newSink(format.NewStreamWriter(&out, format.FormatJSON), 1)
	s.write(1)
	s.write(2)
	s.fail(errors.New("after the count"))
	if code, err := s.wait(context.Background()); code != errs.ExitOK || err != nil || out.String() != "1\n" {
		t.Errorf("count: %d %v %q", code, err, out.String())
	}
	s = newSink(format.NewStreamWriter(&out, format.FormatJSON), 0)
	want := errs.New(errs.KindProtocol, errs.CodeInvalidResponse, "x", nil)
	s.fail(want)
	if _, err := s.wait(context.Background()); err != want {
		t.Errorf("failure: %v", err)
	}
}

func TestBrokerURL(t *testing.T) {
	for in, want := range map[string]bool{
		"mqtt://b:1883": true, "mqtts://b": true, "tcp://b": true, "ssl://b": true,
		"http://b": false, "mqtt://": false, "::": false, "b:1883": false,
	} {
		if got := brokerURL(in); got != want {
			t.Errorf("brokerURL(%q) = %v", in, got)
		}
	}
	for in, want := range map[string]string{
		"tcp://b:1": "mqtt://b:1", "ssl://b": "mqtts://b", "mqtt://tcp://": "mqtt://tcp://",
	} {
		if got := normalBroker(in); got != want {
			t.Errorf("normalBroker(%q) = %q", in, got)
		}
	}
	if got := redacted("mqtt://u:p@b"); got != "mqtt://u:xxxxx@b" {
		t.Errorf("redacted: %s", got)
	}
	if got := redacted("%zz"); got != "%zz" {
		t.Errorf("redacted unparsable: %s", got)
	}
}
