// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/otfabric/openadrctl/internal/format"
)

// peerText is what a VTN may write in a name, an ID, a URI or a topic.
const peerText = "a\nb\tc\x1b[2J\u202ed\xff"

// plainLines reports whether s holds only printable UTF-8 without
// bidirectional controls, but for the newlines that end its lines.
func plainLines(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range strings.ReplaceAll(s, "\n", "") {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return false
		}
	}
	return true
}

// Text records hold what came from the VTN on their own lines: escaped as in
// a table, so that they cannot add lines or drive the terminal.
func TestTextRecordsKeepTheirLines(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row := []string{peerText, peerText, peerText, peerText, peerText, peerText, peerText}
	for _, c := range []struct {
		name  string
		v     any
		lines int
	}{
		{"notification", &Notification{Time: now, Via: "webhook", ObjectType: peerText, Operation: peerText, ID: peerText, Name: peerText}, 1},
		{"change", &Change{Time: now, Change: "created", ID: peerText, Name: peerText, row: row}, 1},
		{"notifiers", &Notifiers{MQTT: &MQTT{URIs: []string{peerText, peerText}, Authentication: peerText, Username: peerText}}, 5},
		{"topics", &Topics{Create: peerText, Update: peerText, Delete: peerText, All: peerText}, 4},
	} {
		var out bytes.Buffer
		if err := format.NewStreamWriter(&out, format.FormatText).Write(c.v); err != nil {
			t.Fatal(err)
		}
		s := out.String()
		if strings.Count(s, "\n") != c.lines || !plainLines(s) || !strings.Contains(s, `a\nb\tc\x1b[2J\u202ed\xff`) {
			t.Errorf("%s: %q", c.name, s)
		}
	}
}

// events watch in text: one line per event, whatever the VTN calls it.
func TestWatchTextIsOneLinePerEvent(t *testing.T) {
	vtn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := []map[string]any{}
		if r.URL.Query().Get("skip") == "" {
			for _, id := range []string{"e1", "e2"} {
				page = append(page, map[string]any{"id": id, "programID": "p", "eventName": peerText + "\nline two"})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer vtn.Close()
	out, err := execute(t, NewVENCmd(), "events", "watch", "--url", vtn.URL, "--count", "2")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\n") != 2 || !plainLines(out) || strings.Count(out, `\nline two`) != 2 {
		t.Errorf("watch:\n%s", out)
	}
}
