// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionAndCompletion(t *testing.T) {
	SetBuildMeta("v9.9.9", "v9.9.9", "abc1234", "2030-01-01T00:00:00Z")
	r := must(t, 0, "version")
	for _, want := range []string{"openadrctl version: v9.9.9", "commit:      abc1234", "openadr:     3.1.0 (go-openadr"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("version lacks %q:\n%s", want, r.stdout)
		}
	}
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		if r := must(t, 0, "completion", sh); !strings.Contains(r.stdout, "openadrctl") {
			t.Errorf("completion %s: %s", sh, r.stdout)
		}
	}
	must(t, 2, "completion", "tcsh")
	must(t, 2, "no-such-command")
	must(t, 2, "bl", "programs", "list", "--no-such-flag")
	if r := must(t, 0, "--help"); !strings.Contains(r.stdout, "Roles:") || !strings.Contains(r.stdout, "Tools:") {
		t.Errorf("help groups:\n%s", r.stdout)
	}
	// A VEN has no command to create programs, Business Logic none to create reports.
	if r := run(t, "ven", "programs", "create", "--help"); strings.Contains(r.stdout, "Create a program") {
		t.Error("ven programs create exists")
	}
	if r := run(t, "bl", "reports", "--help"); strings.Contains(r.stdout, "create") {
		t.Error("bl reports create exists")
	}
}

func TestEnvironment(t *testing.T) {
	t.Setenv("OPENADRCTL_TIMEOUT", "soon")
	r := must(t, 3, "ping", "--url", "http://127.0.0.1:1")
	if !strings.Contains(r.stderr, "OPENADRCTL_TIMEOUT") {
		t.Errorf("stderr: %s", r.stderr)
	}
	t.Setenv("OPENADRCTL_TIMEOUT", "")
	t.Setenv("OPENADRCTL_FORMAT", "json")
	r = must(t, 3, "bl", "programs", "list")
	if e := decode[envelope](t, r); e.Error.Code != "invalid_input" || !strings.Contains(e.Error.Message, "OPENADRCTL_URL") {
		t.Errorf("envelope: %+v", e)
	}
	// A flag wins over the environment.
	if r := runIn(t, `{"programName":"p"}`, "validate", "program", "-", "--format", "text"); r.code != 0 || !strings.HasPrefix(r.stdout, "-: a valid program") {
		t.Errorf("--format text over OPENADRCTL_FORMAT: %d %q", r.code, r.stdout)
	}
}

func TestConnectionFlags(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "s")
	writeFile(t, secret, "x")
	for _, args := range [][]string{
		{"--url", "ftp://vtn"},
		{"--url", "http://u:p@vtn"},
		{"--url", "http://vtn", "--client-id", "a"},
		{"--url", "http://vtn", "--client-secret-file", secret},
		{"--url", "http://vtn", "--token-file", secret, "--client-id", "a", "--client-secret-file", secret},
		{"--url", "http://vtn", "--cert", "c.pem"},
		{"--url", "http://vtn", "--timeout", "0s"},
		{"--url", "http://vtn", "--retries", "-1"},
		{"--url", "http://vtn", "--token-url", "/relative"},
		{"--url", "http://vtn", "--client-id", "a", "--client-secret-file", filepath.Join(dir, "missing")},
		{"--url", "http://vtn", "--token-file", filepath.Join(dir, "missing")},
		{"--url", "http://vtn", "--ca", filepath.Join(dir, "missing")},
		{"--url", "http://vtn", "--ca", secret},
		{"--url", "http://vtn", "--cert", secret, "--key", secret},
	} {
		r := run(t, append([]string{"bl", "programs", "list"}, args...)...)
		if r.code != 3 {
			t.Errorf("%v: exit %d, want 3\n%s", args, r.code, r.stderr)
		}
	}
	empty := filepath.Join(dir, "empty")
	writeFile(t, empty, "\n")
	must(t, 3, "bl", "programs", "list", "--url", "http://vtn", "--token-file", empty)
}

func TestPing(t *testing.T) {
	v := startVTN(t, "--websocket", "--insecure-webhooks")
	r := must(t, 0, v.bl("ping")...)
	for _, want := range []string{"connect    ok", "tls        skipped", "token      ok", "api        ok", "notifiers  ok", "webhook, websocket", "answers"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("ping lacks %q:\n%s", want, r.stdout)
		}
	}
	// Without credentials no token is sent, which this VTN refuses.
	p := decode[Ping](t, must(t, 6, "ping", "--url", v.url, "--format", "json"))
	if p.OK || len(p.Stages) != 5 || p.Stages[2].Detail != "anonymous" || p.Stages[3].Error.Code != "unauthorized" || p.Stages[4].Status != "skipped" {
		t.Errorf("anonymous ping: %+v", p)
	}
	must(t, 0, v.bl("ping", "--format", "table")...)
	// A wrong secret: the token step fails, the others are skipped, exit 6.
	bad := filepath.Join(v.dir, "bad")
	writeFile(t, bad, "wrong")
	r = must(t, 6, "ping", "--url", v.url, "--client-id", "utility", "--client-secret-file", bad, "--format", "json")
	p = decode[Ping](t, r)
	if p.OK || p.Stages[2].Error == nil || p.Stages[2].Error.Code != "token_refused" || p.Stages[3].Status != "skipped" {
		t.Errorf("failed ping: %+v", p)
	}
	// Nothing listening: exit 4.
	r = must(t, 4, "ping", "--url", "http://127.0.0.1:1")
	if !strings.Contains(r.stdout, "connect    failed") {
		t.Errorf("unreachable: %s", r.stdout)
	}
	// A static token is used as it is.
	tok := filepath.Join(v.dir, "token")
	writeFile(t, tok, strings.TrimSpace(must(t, 0, v.bl("token")...).stdout))
	if r := must(t, 0, "ping", "--url", v.url, "--token-file", tok); !strings.Contains(r.stdout, "bearer token given") {
		t.Errorf("token ping: %s", r.stdout)
	}
}

func TestToken(t *testing.T) {
	v := startVTN(t, "--token-ttl", "10m")
	r := must(t, 0, v.bl("token")...)
	if len(strings.TrimSpace(r.stdout)) < 16 || strings.Contains(r.stdout, " ") {
		t.Errorf("token text: %q", r.stdout)
	}
	tok := decode[map[string]any](t, must(t, 0, v.bl("token", "--format", "json")...))
	if tok["token_type"] != "Bearer" || tok["expires_in"].(float64) != 600 || !strings.HasSuffix(tok["token_url"].(string), "/auth/token") {
		t.Errorf("token json: %v", tok)
	}
	must(t, 0, v.ven("token", "--format", "table", "--scope", "read_targets")...)
	must(t, 0, v.bl("token", "--token-url", v.url+"/auth/token")...)
	must(t, 3, "token", "--url", v.url)
	bad := filepath.Join(v.dir, "bad")
	writeFile(t, bad, "wrong")
	e := decode[envelope](t, must(t, 6, "token", "--url", v.url, "--client-id", "utility", "--client-secret-file", bad, "--format", "json"))
	if e.Error.Code != "token_refused" || e.Error.Kind != "refused" {
		t.Errorf("refused token: %+v", e)
	}
}

func TestProgramsAndEvents(t *testing.T) {
	v := startVTN(t)
	// Search: all pages, one page, table and JSON.
	r := must(t, 0, v.bl("bl", "programs", "list")...)
	if !strings.Contains(r.stdout, "dynamic-price") || !strings.Contains(r.stdout, "flex-group-a") {
		t.Errorf("programs list:\n%s", r.stdout)
	}
	list := decode[map[string][]map[string]any](t, must(t, 0, v.bl("bl", "programs", "list", "--limit", "1", "--format", "json")...))
	if len(list["programs"]) != 1 {
		t.Errorf("one page: %v", list)
	}
	must(t, 0, v.bl("bl", "programs", "list", "--skip", "1", "--format", "table")...)
	must(t, 3, v.bl("bl", "programs", "list", "--limit", "51")...)
	// A VEN sees a program with targets only when it names one.
	if r := must(t, 0, v.ven("ven", "programs", "list")...); strings.Contains(r.stdout, "flex-group-a") {
		t.Errorf("a VEN saw a targeted program:\n%s", r.stdout)
	}
	if r := must(t, 0, v.ven("ven", "programs", "list", "--target", "group-a")...); !strings.Contains(r.stdout, "flex-group-a") {
		t.Errorf("a VEN does not see its program:\n%s", r.stdout)
	}

	// Create, read, update and delete a program.
	r = must(t, 0, v.bl("bl", "programs", "create", "--name", "cli-test", "--payload", "PRICE:KWH:EUR", "--target", "t1",
		"--description-url", "https://example.com/p", "--attribute", "LOCATION=1:2", "--format", "json")...)
	p := decode[map[string]any](t, r)
	id := p["id"].(string)
	if p["programName"] != "cli-test" {
		t.Errorf("created: %v", p)
	}
	r = must(t, 0, v.bl("bl", "programs", "get", id)...)
	for _, want := range []string{"name:", "cli-test", "targets:", "t1", "descriptions:", "https://example.com/p", "attributes:", "LOCATION=(1,2)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("get lacks %q:\n%s", want, r.stdout)
		}
	}
	must(t, 0, v.bl("bl", "programs", "get", id, "--format", "table")...)
	if r := must(t, 0, v.bl("bl", "programs", "update", id, "--target", "t2")...); r.stdout != "updated program "+id+" (cli-test)\n" {
		t.Errorf("update: %q", r.stdout)
	}
	file := filepath.Join(v.dir, "program.json")
	writeFile(t, file, `{"programName":"cli-test-2"}`)
	p = decode[map[string]any](t, must(t, 0, v.bl("bl", "programs", "update", id, "--file", file, "--format", "json")...))
	if p["programName"] != "cli-test-2" || p["id"] != id || p["targets"] != nil {
		t.Errorf("update from a file replaces the object: %v", p)
	}
	r = runIn(t, `{"programName":"from-stdin"}`, v.bl("bl", "programs", "create", "--file", "-")...)
	if r.code != 0 || !strings.HasPrefix(r.stdout, "created program ") {
		t.Errorf("create from stdin: %d %s %s", r.code, r.stdout, r.stderr)
	}
	must(t, 2, v.bl("bl", "programs", "delete", id)...)
	if r := must(t, 0, v.bl("bl", "programs", "delete", id, "--yes")...); !strings.HasPrefix(r.stdout, "deleted program "+id) {
		t.Errorf("delete: %s", r.stdout)
	}
	e := decode[envelope](t, must(t, 6, v.bl("bl", "programs", "get", id, "--format", "json")...))
	if e.Error.Code != "not_found" || e.OpenADR["status"].(float64) != 404 {
		t.Errorf("not found: %+v", e)
	}
	e = decode[envelope](t, must(t, 6, v.bl("bl", "programs", "delete", "dynamic-price", "--yes", "--format", "json")...))
	if e.Error.Code != "conflict" || !strings.Contains(e.Error.Message, "still has events") {
		t.Errorf("conflict: %+v", e)
	}
	e = decode[envelope](t, must(t, 6, v.ven("bl", "programs", "create", "--name", "x", "--format", "json")...))
	if e.Error.Code != "forbidden" {
		t.Errorf("forbidden: %+v", e)
	}
	e = decode[envelope](t, must(t, 6, "bl", "programs", "list", "--url", v.url, "--format", "json"))
	if e.Error.Code != "unauthorized" {
		t.Errorf("unauthorized: %+v", e)
	}
	// Invalid objects are refused before anything is sent.
	e = decode[envelope](t, must(t, 3, v.bl("bl", "programs", "create", "--format", "json")...))
	if e.Error.Code != "invalid_object" || e.OpenADR["field"] != "programName" {
		t.Errorf("invalid: %+v", e)
	}
	must(t, 3, v.bl("bl", "programs", "get", "not an id")...)
	must(t, 3, v.bl("bl", "programs", "create", "--file", filepath.Join(v.dir, "missing.json"))...)
	writeFile(t, file, "not json")
	must(t, 3, v.bl("bl", "programs", "create", "--file", file)...)
	must(t, 3, v.bl("bl", "programs", "create", "--name", "x", "--attribute", "nonsense")...)

	// Events: create with intervals, confirm, read, lay out, update, delete.
	must(t, 2, v.bl("bl", "events", "create", "--program", "dynamic-price", "--type", "PRICE", "--value", "1")...)
	r = must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "cli-event", "--type", "PRICE",
		"--start", "2030-01-02T00:00:00Z", "--interval", "PT30M", "--value", "0.1", "--value", "0.2", "--priority", "1",
		"--target", "group-a", "--duration", "1h", "--report", "USAGE", "--yes", "--format", "json")...)
	ev := decode[map[string]any](t, r)
	eid := ev["id"].(string)
	if ev["priority"].(float64) != 1 || len(ev["intervals"].([]any)) != 2 || ev["duration"] != "PT1H" {
		t.Errorf("event: %v", ev)
	}
	r = must(t, 0, v.bl("bl", "events", "timeline", eid)...)
	if !strings.Contains(r.stdout, "2030-01-02T00:30:00Z  2030-01-02T01:00:00Z  PRICE  0.2") {
		t.Errorf("timeline:\n%s", r.stdout)
	}
	csvFile := filepath.Join(v.dir, "intervals.csv")
	r = must(t, 0, v.bl("bl", "events", "timeline", eid, "--csv")...)
	writeFile(t, csvFile, r.stdout)
	tl := decode[map[string]any](t, must(t, 0, v.bl("bl", "events", "timeline", eid, "--at", "2030-01-02T00:45:00Z", "--format", "json")...))
	if rows := tl["rows"].([]any); len(rows) != 1 {
		t.Errorf("timeline at: %v", tl)
	}
	must(t, 0, v.bl("bl", "events", "timeline", eid, "--sub-intervals", "--until", "2030-01-03T00:00:00Z", "--format", "table")...)
	must(t, 3, v.bl("bl", "events", "timeline", eid, "--at", "yesterday")...)
	r = must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--csv", csvFile, "--yes")...)
	if !strings.HasPrefix(r.stdout, "created event") {
		t.Errorf("event from CSV: %s", r.stdout)
	}
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--csv", csvFile, "--value", "1", "--yes")...)
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--value", "1", "--yes")...)
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--type", "PRICE", "--yes")...)
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--type", "PRICE", "--value", "1", "--start", "soon", "--yes")...)
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--type", "PRICE", "--value", "1", "--interval", "long", "--yes")...)
	must(t, 3, v.bl("bl", "events", "create", "--program", "dynamic-price", "--duration", "long", "--yes")...)
	if r := must(t, 0, v.ven("ven", "events", "list", "--program", "dynamic-price", "--target", "group-a", "--active")...); !strings.Contains(r.stdout, "cli-event") {
		t.Errorf("VEN events:\n%s", r.stdout)
	}
	if r := must(t, 0, v.ven("ven", "events", "get", eid)...); !strings.Contains(r.stdout, "reports asked:    USAGE") {
		t.Errorf("event text:\n%s", r.stdout)
	}
	must(t, 2, v.bl("bl", "events", "update", eid, "--name", "renamed")...)
	must(t, 0, v.bl("bl", "events", "update", eid, "--name", "renamed", "--yes")...)
	must(t, 0, v.bl("bl", "events", "update", eid, "--name", "dry", "--dry-run")...)
	must(t, 0, v.bl("bl", "events", "delete", eid, "--yes")...)
}

func TestDryRunOffline(t *testing.T) {
	// No VTN and no URL: a dry run of a create needs neither.
	r := must(t, 0, "bl", "events", "create", "--program", "p1", "--type", "SIMPLE", "--value", "1", "--value", "true",
		"--value", "1.5:2", "--value", "high", "--dry-run", "--format", "json")
	if !strings.Contains(r.stdout, `"values": [`) || strings.Contains(r.stdout, "createdDateTime") {
		t.Errorf("dry run:\n%s", r.stdout)
	}
	r = must(t, 0, "bl", "programs", "create", "--name", "p", "--dry-run")
	if !strings.Contains(r.stdout, "name:") {
		t.Errorf("dry run text:\n%s", r.stdout)
	}
	e := decode[envelope](t, must(t, 3, "bl", "events", "create", "--program", "not valid", "--dry-run", "--format", "json"))
	if e.Error.Code != "invalid_object" || e.OpenADR["field"] != "programID" {
		t.Errorf("dry run invalid: %+v", e)
	}
}

func TestReportsVENsResources(t *testing.T) {
	v := startVTN(t)
	// A VEN reports on an event; Business Logic reads the reports.
	r := must(t, 0, v.ven("ven", "reports", "create", "--event", "price-2030-01-01", "--client-name", "site-42", "--name", "usage",
		"--resource", "meter-1", "--type", "USAGE", "--units", "KWH", "--reading-type", "DIRECT_READ",
		"--start", "2030-01-01T00:00:00Z", "--interval", "PT1H", "--value", "1.2", "--value", "0.9", "--format", "json")...)
	rep := decode[map[string]any](t, r)
	rid := rep["id"].(string)
	if rep["clientID"] != "site-42" || len(rep["resources"].([]any)) != 1 {
		t.Errorf("report: %v", rep)
	}
	must(t, 0, v.ven("ven", "reports", "update", rid, "--resource", "meter-2", "--type", "USAGE", "--value", "3")...)
	r = must(t, 0, v.bl("bl", "reports", "get", rid)...)
	if !strings.Contains(r.stdout, "resource meter-1") || !strings.Contains(r.stdout, "resource meter-2") {
		t.Errorf("report text:\n%s", r.stdout)
	}
	if r := must(t, 0, v.bl("bl", "reports", "list", "--event", "price-2030-01-01", "--client-name", "site-42", "--program", "dynamic-price")...); !strings.Contains(r.stdout, rid) {
		t.Errorf("reports list:\n%s", r.stdout)
	}
	must(t, 0, v.ven("ven", "reports", "list", "--format", "json")...)
	must(t, 0, v.ven("ven", "reports", "delete", rid, "--yes")...)
	if r := must(t, 0, v.bl("bl", "reports", "list")...); r.stdout != "no reports\n" {
		t.Errorf("empty list: %q", r.stdout)
	}

	// VENs and resources as Business Logic and as the VEN.
	r = must(t, 0, v.bl("bl", "vens", "create", "--name", "site-43", "--ven-client-id", "site-43", "--target", "group-b",
		"--attribute", "LOCATION=1:2", "--format", "json")...)
	vid := decode[map[string]any](t, r)["id"].(string)
	must(t, 0, v.bl("bl", "vens", "update", vid, "--target", "group-c")...)
	if r := must(t, 0, v.bl("bl", "vens", "list", "--target", "group-c")...); !strings.Contains(r.stdout, "site-43") {
		t.Errorf("vens list:\n%s", r.stdout)
	}
	must(t, 0, v.bl("bl", "vens", "list", "--name", "site-43", "--format", "table")...)
	r = must(t, 0, v.bl("bl", "resources", "create", "--ven", vid, "--name", "battery", "--ven-client-id", "site-43",
		"--target", "t", "--attribute", "MAX_POWER_CONSUMPTION=5", "--format", "json")...)
	resID := decode[map[string]any](t, r)["id"].(string)
	must(t, 0, v.bl("bl", "resources", "update", resID, "--name", "battery-2")...)
	if r := must(t, 0, v.bl("bl", "resources", "get", resID)...); !strings.Contains(r.stdout, "battery-2") || !strings.Contains(r.stdout, "MAX_POWER_CONSUMPTION=5") {
		t.Errorf("resource:\n%s", r.stdout)
	}
	must(t, 0, v.bl("bl", "resources", "list", "--ven", vid, "--name", "battery-2", "--target", "t")...)
	must(t, 0, v.bl("bl", "resources", "delete", resID, "--yes")...)
	must(t, 0, v.bl("bl", "vens", "delete", vid, "--yes")...)
	// The VEN's own objects.
	r = must(t, 0, v.ven("ven", "resources", "create", "--ven", "site-42", "--name", "heat-pump", "--format", "json")...)
	own := decode[map[string]any](t, r)["id"].(string)
	must(t, 0, v.ven("ven", "resources", "update", own, "--attribute", "MAX_POWER_CONSUMPTION=3")...)
	must(t, 0, v.ven("ven", "resources", "get", own)...)
	must(t, 0, v.ven("ven", "resources", "delete", own, "--yes")...)
	must(t, 0, v.ven("ven", "vens", "get", "site-42")...)
	must(t, 0, v.ven("ven", "vens", "update", "site-42", "--attribute", "LOCATION=1:1")...)
	r = must(t, 0, v.ven("ven", "vens", "create", "--name", "site-42b", "--format", "json")...)
	must(t, 0, v.ven("ven", "vens", "delete", decode[map[string]any](t, r)["id"].(string), "--yes")...)
}

func TestSubscriptionsAndGroups(t *testing.T) {
	v := startVTN(t, "--insecure-webhooks", "--websocket", "--resource-groups")
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Query().Get("echo")))
	}))
	defer hook.Close()
	r := must(t, 0, v.bl("bl", "subscriptions", "create", "--client-name", "cli", "--object", "event", "--object", "PROGRAM",
		"--operation", "create", "--callback-url", hook.URL, "--webhook-token", "t0k", "--program", "dynamic-price", "--format", "json")...)
	sid := decode[map[string]any](t, r)["id"].(string)
	r = must(t, 0, v.bl("bl", "subscriptions", "list", "--object", "EVENT", "--client-name", "cli", "--program", "dynamic-price")...)
	if !strings.Contains(r.stdout, sid) || !strings.Contains(r.stdout, "EVENT,PROGRAM") || !strings.Contains(r.stdout, hook.URL) {
		t.Errorf("subscriptions:\n%s", r.stdout)
	}
	must(t, 0, v.bl("bl", "subscriptions", "update", sid, "--websocket")...)
	if r := must(t, 0, v.bl("bl", "subscriptions", "get", sid)...); !strings.Contains(r.stdout, "websocket") {
		t.Errorf("subscription:\n%s", r.stdout)
	}
	must(t, 3, v.bl("bl", "subscriptions", "update", sid, "--websocket", "--callback-url", hook.URL)...)
	must(t, 3, v.bl("bl", "subscriptions", "update", sid, "--object", "THING")...)
	must(t, 3, v.bl("bl", "subscriptions", "update", sid, "--operation", "EXPLODE")...)
	must(t, 0, v.bl("bl", "subscriptions", "delete", sid, "--yes")...)
	r = must(t, 0, v.ven("ven", "subscriptions", "create", "--client-name", "ven", "--websocket", "--format", "json")...)
	vsid := decode[map[string]any](t, r)["id"].(string)
	must(t, 0, v.ven("ven", "subscriptions", "list")...)
	must(t, 0, v.ven("ven", "subscriptions", "get", vsid)...)
	must(t, 0, v.ven("ven", "subscriptions", "update", vsid, "--client-name", "ven-2")...)
	must(t, 0, v.ven("ven", "subscriptions", "delete", vsid, "--yes")...)

	// Resource groups.
	r = must(t, 0, v.bl("bl", "resource-groups", "create", "--name", "g", "--child", "resource:site-42-meter", "--child", "group:feeder-7",
		"--target", "t", "--attribute", "X=1", "--format", "json")...)
	gid := decode[map[string]any](t, r)["id"].(string)
	must(t, 0, v.bl("bl", "resource-groups", "update", gid, "--child", "ven_resource:site-42-meter")...)
	must(t, 3, v.bl("bl", "resource-groups", "update", gid, "--child", "thing:x")...)
	if r := must(t, 0, v.bl("bl", "resource-groups", "list", "--name", "g", "--target", "t")...); !strings.Contains(r.stdout, gid) {
		t.Errorf("groups:\n%s", r.stdout)
	}
	if r := must(t, 0, v.ven("ven", "resource-groups", "get", "feeder-7")...); !strings.Contains(r.stdout, "ven_resource:site-42-meter") {
		t.Errorf("VEN group:\n%s", r.stdout)
	}
	must(t, 0, v.ven("ven", "resource-groups", "list")...)
	must(t, 0, v.bl("bl", "resource-groups", "delete", gid, "--yes")...)

	// Notifiers.
	if r := must(t, 0, v.ven("ven", "notifiers")...); !strings.Contains(r.stdout, "webhook:    yes") || !strings.Contains(r.stdout, "mqtt:       no") {
		t.Errorf("notifiers:\n%s", r.stdout)
	}
	must(t, 0, v.ven("ven", "notifiers", "--format", "table")...)
	if n := decode[Notifiers](t, must(t, 0, v.bl("bl", "notifiers", "--format", "json")...)); !n.WebSocket || n.MQTT != nil {
		t.Errorf("notifiers json: %+v", n)
	}
	// Without MQTT the topics are not implemented.
	e := decode[envelope](t, must(t, 6, v.bl("bl", "topics", "events", "--format", "json")...))
	if e.Error.Code != "not_implemented" {
		t.Errorf("topics without MQTT: %+v", e)
	}
	for _, set := range []string{"nothing", "ven", "events:x", "ven:not an id"} {
		must(t, 3, v.bl("bl", "topics", set)...)
	}
}

// Notifiers is the JSON of notifiers.
type Notifiers struct {
	Webhook, WebSocket bool
	MQTT               *struct{ URIs []string }
}

func TestListenWebSocketAndWebhook(t *testing.T) {
	v := startVTN(t, "--insecure-webhooks", "--websocket", "--log-requests")
	ws := start(t, v.ven("ven", "listen", "--via", "websocket", "--object", "EVENT", "--operation", "CREATE", "--count", "1", "--format", "json")...)
	waitFor(t, ws.err, "listening: WebSocket open")
	wh := start(t, v.bl("bl", "listen", "--via", "webhook", "--listen", "127.0.0.1:0", "--object", "EVENT", "--count", "2", "--keep")...)
	waitFor(t, wh.err, "subscribed:")
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "n1", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := ws.wait(t); code != 0 {
		t.Fatalf("listen websocket: exit %d\n%s", code, ws.err)
	}
	recs := lines(t, ws.out.String())
	if len(recs) != 1 || recs[0]["via"] != "websocket" || recs[0]["operation"] != "CREATE" || recs[0]["name"] != "n1" {
		t.Errorf("websocket records: %v", recs)
	}
	if !strings.Contains(ws.err.String(), "unsubscribed:") {
		t.Errorf("the subscription was not deleted:\n%s", ws.err)
	}
	must(t, 0, v.bl("bl", "events", "update", "price-2030-01-01", "--priority", "3", "--yes")...)
	if code := wh.wait(t); code != 0 {
		t.Fatalf("listen webhook: exit %d\n%s", code, wh.err)
	}
	if out := wh.out.String(); !strings.Contains(out, "webhook   EVENT        CREATE") || !strings.Contains(out, "UPDATE") {
		t.Errorf("webhook records:\n%s", out)
	}
	if !strings.Contains(wh.err.String(), "kept: subscription") {
		t.Errorf("--keep:\n%s", wh.err)
	}
	waitFor(t, v.out, `"event":"notification"`)
	waitFor(t, v.out, `"event":"request"`)

	// --duration ends a stream that saw nothing, with exit 0.
	quiet := start(t, v.ven("ven", "listen", "--via", "websocket", "--duration", "300ms", "--format", "table")...)
	if code := quiet.wait(t); code != 0 {
		t.Errorf("quiet listen: exit %d\n%s", code, quiet.err)
	}
	// A webhook the VTN cannot verify: the subscription is refused.
	r := must(t, 6, v.bl("bl", "listen", "--via", "webhook", "--listen", "127.0.0.1:0", "--callback-url", "http://127.0.0.1:1/", "--format", "json")...)
	if e := decode[envelope](t, r); e.Error.Kind != "refused" {
		t.Errorf("unverifiable webhook: %+v", e)
	}
	for _, args := range [][]string{
		{"--via", "carrier-pigeon"},
		{},
		{"--via", "webhook", "--listen", "nowhere"},
		{"--via", "webhook", "--tls-cert", "c.pem"},
		{"--via", "webhook", "--callback-url", "ftp://x"},
		{"--via", "mqtt"},
		{"--via", "mqtt", "--topics", "nothing"},
		{"--via", "mqtt", "--topics", "events", "--broker", "http://x"},
		{"--via", "websocket", "--count", "-1"},
		{"--via", "websocket", "--program", "not an id"},
		{"--via", "websocket", "--object", "THING"},
		{"--via", "webhook", "--tls-cert", "c.pem", "--tls-key", "k.pem"},
	} {
		r := run(t, v.bl(append([]string{"bl", "listen"}, args...)...)...)
		if r.code != 3 {
			t.Errorf("listen %v: exit %d, want 3\n%s", args, r.code, r.stderr)
		}
	}
	// A VTN without WebSockets refuses the stream.
	plain := startVTN(t)
	r = must(t, 6, plain.ven("ven", "listen", "--via", "websocket", "--format", "json")...)
	if e := decode[envelope](t, r); e.Error.Code != "not_implemented" {
		t.Errorf("no websockets: %+v", e)
	}
}

func TestListenMQTT(t *testing.T) {
	broker := startBroker(t)
	v := startVTN(t, "--mqtt", broker)
	if r := must(t, 0, v.bl("bl", "topics", "events")...); !strings.Contains(r.stdout, "create:") || !strings.Contains(r.stdout, "all:") {
		t.Errorf("topics:\n%s", r.stdout)
	}
	must(t, 0, v.bl("bl", "topics", "program-events:dynamic-price", "--format", "table")...)
	must(t, 0, v.ven("ven", "topics", "ven-events:site-42", "--format", "json")...)
	if r := must(t, 0, v.bl("bl", "notifiers")...); !strings.Contains(r.stdout, broker) || !strings.Contains(r.stdout, "ANONYMOUS") {
		t.Errorf("notifiers:\n%s", r.stdout)
	}
	l := start(t, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--topics", "programs", "--count", "1", "--format", "json")...)
	waitFor(t, l.err, "listening: "+broker)
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "by-mqtt", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := l.wait(t); code != 0 {
		t.Fatalf("listen mqtt: exit %d\n%s", code, l.err)
	}
	recs := lines(t, l.out.String())
	if len(recs) != 1 || recs[0]["via"] != "mqtt" || recs[0]["topic"] == "" || recs[0]["name"] != "by-mqtt" {
		t.Errorf("mqtt records: %v", recs)
	}
	// A password from a file, and a broker given on the command line.
	pw := filepath.Join(v.dir, "pw")
	writeFile(t, pw, "x")
	q := start(t, v.bl("bl", "listen", "--via", "mqtt", "--topics", "vens", "--broker", strings.Replace(broker, "mqtt://", "tcp://", 1),
		"--mqtt-password-file", pw, "--mqtt-client-id", "cli-test")...)
	waitFor(t, q.err, "listening: ")
	if code := q.stop(); code != 0 {
		t.Errorf("listen mqtt with a password: exit %d\n%s", code, q.err)
	}
	must(t, 3, v.bl("bl", "listen", "--via", "mqtt", "--topics", "vens", "--mqtt-password-file", filepath.Join(v.dir, "missing"))...)
	// A broker that is not there: exit 4.
	r := must(t, 4, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--broker", "mqtt://127.0.0.1:1", "--timeout", "1s", "--format", "json")...)
	if e := decode[envelope](t, r); e.Error.Kind != "transport" {
		t.Errorf("no broker: %+v", e)
	}
	// --duration that ends before the broker answers is a failure, not an end.
	r = must(t, 4, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--broker", "mqtt://127.0.0.1:1", "--duration", "200ms")...)
	if !strings.Contains(r.stderr, "not subscribed") {
		t.Errorf("duration before the broker: %s", r.stderr)
	}
	// A VTN without MQTT announces no broker.
	plain := startVTN(t)
	must(t, 6, plain.bl("bl", "listen", "--via", "mqtt", "--topics", "events")...)
}

func TestWatch(t *testing.T) {
	v := startVTN(t)
	w := start(t, v.ven("ven", "events", "watch", "--interval", "1s", "--count", "3", "--format", "json")...)
	waitFor(t, w.out, `"change":"present"`)
	r := must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "w", "--type", "PRICE", "--value", "1", "--yes", "--format", "json")...)
	id := decode[map[string]any](t, r)["id"].(string)
	waitFor(t, w.out, `"change":"created"`)
	must(t, 0, v.bl("bl", "events", "delete", id, "--yes")...)
	if code := w.wait(t); code != 0 {
		t.Fatalf("watch: exit %d\n%s", code, w.err)
	}
	recs := lines(t, w.out.String())
	if len(recs) != 3 || recs[2]["change"] != "deleted" || recs[2]["id"] != id {
		t.Errorf("watch records: %v", recs)
	}
	tw := start(t, v.bl("bl", "events", "watch", "--interval", "1s", "--changes-only", "--duration", "1500ms", "--format", "table")...)
	if code := tw.wait(t); code != 0 || tw.out.String() != "" {
		t.Errorf("changes only: exit %d, %q", code, tw.out)
	}
	tx := start(t, v.bl("bl", "events", "watch", "--interval", "1s", "--count", "1")...)
	if code := tx.wait(t); code != 0 || !strings.Contains(tx.out.String(), "present  event price-2030-01-01") {
		t.Errorf("watch text: %d %s", code, tx.out)
	}
	must(t, 3, v.bl("bl", "events", "watch", "--interval", "10ms")...)
	// A search that fails ends the watch with its exit code.
	bad := filepath.Join(v.dir, "bad")
	writeFile(t, bad, "wrong")
	must(t, 6, "ven", "events", "watch", "--url", v.url, "--client-id", "site-42", "--client-secret-file", bad)
}

func TestTimeouts(t *testing.T) {
	// A VTN that never answers: a read times out, a write has an unknown outcome.
	stop := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	defer hang.Close()
	defer close(stop)
	e := decode[envelope](t, must(t, 5, "bl", "programs", "list", "--url", hang.URL, "--timeout", "300ms", "--format", "json"))
	if e.Error.Code != "request_timeout" {
		t.Errorf("read: %+v", e)
	}
	e = decode[envelope](t, must(t, 5, "bl", "programs", "create", "--name", "x", "--url", hang.URL, "--timeout", "300ms", "--format", "json"))
	if e.Error.Code != "outcome_unknown" {
		t.Errorf("write: %+v", e)
	}
	// No token in time: the create was not sent, so its outcome is known.
	secret := filepath.Join(t.TempDir(), "secret")
	writeFile(t, secret, "s")
	e = decode[envelope](t, must(t, 5, "bl", "programs", "create", "--name", "x", "--url", hang.URL, "--client-id", "c",
		"--client-secret-file", secret, "--timeout", "300ms", "--format", "json"))
	if e.Error.Code != "request_timeout" || !strings.Contains(e.Error.Message, "not sent") {
		t.Errorf("write without a token: %+v", e)
	}
	// An answer that is not OpenADR.
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"not":"a list"}`))
	}))
	defer junk.Close()
	e = decode[envelope](t, must(t, 6, "bl", "programs", "list", "--url", junk.URL, "--format", "json"))
	if e.Error.Kind != "protocol" || e.Error.Code != "invalid_response" {
		t.Errorf("junk: %+v", e)
	}
	// 429 with Retry-After, retried once with --retries.
	calls := 0
	busy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer busy.Close()
	e = decode[envelope](t, must(t, 6, "bl", "programs", "list", "--url", busy.URL, "--retries", "1", "--format", "json"))
	if e.Error.Code != "too_many_requests" || e.OpenADR["retry_after"] != "1s" || calls != 2 {
		t.Errorf("429: %+v after %d calls", e, calls)
	}
}

func TestTokenDiscovery(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusInternalServerError) },
		"no tokenURL": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		},
		"token endpoint gives junk": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/server" {
				_, _ = w.Write([]byte(`{"tokenURL":"/token"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"x","token_type":"MAC"}`))
		},
		"empty token": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/server" {
				_, _ = w.Write([]byte(`{"tokenURL":"token"}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		},
	} {
		srv := httptest.NewServer(h)
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "s"), "x")
		r := run(t, "token", "--url", srv.URL, "--client-id", "a", "--client-secret-file", filepath.Join(dir, "s"))
		if r.code != 6 {
			t.Errorf("%s: exit %d\n%s", name, r.code, r.stderr)
		}
		srv.Close()
	}
}

func TestValidateAndTimeline(t *testing.T) {
	dir := t.TempDir()
	ev := filepath.Join(dir, "event.json")
	writeFile(t, ev, `{"programID":"p1","eventName":"e","intervalPeriod":{"start":"2030-01-01T00:00:00Z","duration":"PT1H"},
"intervals":[{"id":0,"payloads":[{"type":"PRICE","values":[1,2]}]},{"id":1,"payloads":[{"type":"PRICE","values":[3]}]}]}`)
	if r := must(t, 0, "validate", "event", ev); !strings.Contains(r.stdout, "a valid event (e)") {
		t.Errorf("validate: %s", r.stdout)
	}
	must(t, 0, "validate", "event", ev, "--format", "table")
	v := decode[map[string]any](t, must(t, 0, "validate", "event", ev, "--format", "json"))
	if v["kind"] != "event" || v["name"] != "e" {
		t.Errorf("validate json: %v", v)
	}
	for kind, doc := range map[string]string{
		"program": `{"programName":"p"}`, "report": `{"eventID":"e1","clientName":"c","resources":[]}`,
		"ven": `{"venName":"v"}`, "resource": `{"resourceName":"r","venID":"v1"}`,
		"subscription":   `{"clientName":"c","objectOperations":[{"objects":["EVENT"],"operations":["CREATE"],"callbackUrl":"https://x.example/cb"}]}`,
		"resource-group": `{"resourceGroupName":"g","children":[]}`,
	} {
		if r := runIn(t, doc, "validate", kind, "-"); r.code != 0 {
			t.Errorf("validate %s: exit %d %s", kind, r.code, r.stderr)
		}
	}
	e := decode[envelope](t, func() result {
		r := runIn(t, `{"programName":""}`, "validate", "program", "-", "--format", "json")
		if r.code != 3 {
			t.Fatalf("invalid program: exit %d", r.code)
		}
		return r
	}())
	if e.Error.Code != "invalid_object" || e.OpenADR["field"] != "programName" || strings.Contains(e.Error.Message, "sent") {
		t.Errorf("invalid program: %+v", e)
	}
	// A misspelt property is not dropped without a word.
	if r := runIn(t, `{"programName":"p","programname":"q"}`, "validate", "program", "-"); r.code != 3 || !strings.Contains(r.stderr, `"programname"`) {
		t.Errorf("misspelt property: exit %d %s", r.code, r.stderr)
	}
	must(t, 3, "validate", "thing", ev)
	must(t, 3, "validate", "event", filepath.Join(dir, "missing"))
	must(t, 2, "validate", "event")

	r := must(t, 0, "timeline", ev)
	if !strings.Contains(r.stdout, "0         2030-01-01T00:00:00Z  2030-01-01T01:00:00Z  PRICE  1,2") {
		t.Errorf("timeline:\n%s", r.stdout)
	}
	if r := must(t, 0, "timeline", ev, "--sub-intervals"); !strings.Contains(r.stdout, "0.1") {
		t.Errorf("sub-intervals:\n%s", r.stdout)
	}
	if r := must(t, 0, "timeline", ev, "--csv"); !strings.HasPrefix(r.stdout, "interval,sub,start,end,type,values,randomize_start") {
		t.Errorf("csv:\n%s", r.stdout)
	}
	tl := decode[map[string]any](t, must(t, 0, "timeline", ev, "--at", "2030-01-01T01:30:00Z", "--format", "json"))
	if rows := tl["rows"].([]any); len(rows) != 1 {
		t.Errorf("at: %v", tl)
	}
	now := filepath.Join(dir, "now.json")
	writeFile(t, now, `{"programID":"p1","intervalPeriod":{"start":"0001-01-01T00:00:00Z","duration":"PT1H"},
"intervals":[{"id":0,"payloads":[{"type":"SIMPLE","values":[1]}]}]}`)
	if r := must(t, 0, "timeline", now, "--now", "2030-06-01T12:00:00Z"); !strings.Contains(r.stdout, "2030-06-01T12:00:00Z") {
		t.Errorf("now:\n%s", r.stdout)
	}
	must(t, 3, "timeline", now, "--now", "noon")
	empty := filepath.Join(dir, "empty.json")
	writeFile(t, empty, `{"programID":"p1"}`)
	if r := must(t, 0, "timeline", empty); r.stdout != "no intervals\n" {
		t.Errorf("empty: %q", r.stdout)
	}
	bad := filepath.Join(dir, "bad.json")
	writeFile(t, bad, `{"programID":"p1","intervals":[{"id":0,"payloads":[{"type":"X","values":[1]}]}]}`)
	must(t, 6, "timeline", bad)
}

func TestVTNStart(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "seed.yaml")
	must(t, 0, "vtn", "generate-config", "--output", seedFile)
	must(t, 3, "vtn", "generate-config", "--output", seedFile)
	must(t, 0, "vtn", "generate-config", "--output", seedFile, "--force")
	if r := must(t, 0, "vtn", "generate-config"); !strings.Contains(r.stdout, "clients:") {
		t.Errorf("generate-config:\n%s", r.stdout)
	}
	// Kept in a file: an object created survives a restart, and the seed is not applied twice.
	store := filepath.Join(dir, "vtn.db")
	v := startVTN(t, "--config", seedFile, "--store", store)
	r := must(t, 0, v.bl("bl", "programs", "create", "--name", "kept", "--format", "json")...)
	id := decode[map[string]any](t, r)["id"].(string)
	if code := v.stop(); code != 0 {
		t.Fatalf("vtn start: exit %d\n%s", code, v.err)
	}
	if !strings.Contains(v.out.String(), `"event":"stopped"`) {
		t.Errorf("no stopped record:\n%s", v.out)
	}
	v = startVTN(t, "--config", seedFile, "--store", store, "--format", "text")
	waitFor(t, v.out, "seeded         0 objects")
	must(t, 0, v.bl("bl", "programs", "get", id)...)

	// The seed is checked before anything listens, every problem at once.
	bad := filepath.Join(dir, "bad.yaml")
	writeFile(t, bad, "clients:\n  - id: a\n    role: x\n  - id: a\n    secret: s\n    secretEnv: NOPE_NOT_SET\n    role: bl\n  - role: ven\n    secret: s\n"+
		"programs:\n  - programName: \"\"\n  - programName: p\n    id: \"bad id\"\nevents:\n  - programID: nowhere\n")
	r = must(t, 3, "vtn", "start", "--config", bad, "--format", "json")
	e := decode[envelope](t, r)
	for _, want := range []string{`role "x"`, `client "a" twice`, "secret or secretEnv, not both", "NOPE_NOT_SET is not set", "id is required", "programName", "not an object ID"} {
		if !strings.Contains(e.Error.Message, want) {
			t.Errorf("seed problems lack %q: %s", want, e.Error.Message)
		}
	}
	writeFile(t, bad, "events:\n  - programID: nowhere\n")
	if r := must(t, 3, "vtn", "start", "--config", bad, "--anonymous"); !strings.Contains(r.stderr, "events[0]") {
		t.Errorf("an event of no program: %s", r.stderr)
	}
	writeFile(t, bad, "unknown: 1\n")
	must(t, 3, "vtn", "start", "--config", bad)
	writeFile(t, bad, "")
	if r := must(t, 3, "vtn", "start", "--config", bad); !strings.Contains(r.stderr, "nobody could use the VTN") {
		t.Errorf("no clients: %s", r.stderr)
	}
	must(t, 3, "vtn", "start", "--config", filepath.Join(dir, "missing.yaml"))
	t.Setenv("SEED_SECRET", "from-env")
	writeFile(t, bad, "clients:\n  - id: a\n    secretEnv: SEED_SECRET\n    role: ven\n")
	e2 := startVTN(t, "--config", bad, "--anonymous", "--rate", "100", "--notify-reads", "--advertise", "")
	must(t, 0, "ven", "programs", "list", "--url", e2.url)
	for _, args := range [][]string{
		{"--listen", "nowhere"},
		{"--tls-cert", "c.pem"},
		{"--public-url", "ftp://x"},
		{"--mqtt", "http://x"},
		{"--rate", "-1"},
		{"--token-ttl", "0s"},
		{"--tls-cert", filepath.Join(dir, "missing"), "--tls-key", filepath.Join(dir, "missing")},
		{"--store", dir},
	} {
		if r := run(t, append([]string{"vtn", "start"}, args...)...); r.code != 3 {
			t.Errorf("vtn start %v: exit %d\n%s", args, r.code, r.stderr)
		}
	}
	// The address is taken.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	must(t, 4, "vtn", "start", "--listen", l.Addr().String())
	// --duration ends it with exit 0.
	d := start(t, "vtn", "start", "--listen", "127.0.0.1:0", "--duration", "300ms", "--format", "table")
	if code := d.wait(t); code != 0 {
		t.Errorf("--duration: exit %d\n%s", code, d.err)
	}
}

func TestTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := selfSigned(t, dir)
	v := startVTN(t, "--tls-cert", certFile, "--tls-key", keyFile, "--insecure-webhooks")
	if !strings.HasPrefix(v.url, "https://") {
		t.Fatalf("url %s", v.url)
	}
	// Without the CA the handshake fails; with it everything works.
	e := decode[envelope](t, must(t, 4, v.bl("bl", "programs", "list", "--format", "json")...))
	if e.Error.Code != "transport_tls_failed" {
		t.Errorf("unknown CA: %+v", e)
	}
	r := must(t, 0, v.bl("ping", "--ca", certFile)...)
	if !strings.Contains(r.stdout, "tls        ok") {
		t.Errorf("ping over TLS:\n%s", r.stdout)
	}
	must(t, 0, v.bl("bl", "programs", "list", "--ca", certFile)...)
	must(t, 0, v.bl("bl", "programs", "list", "--insecure")...)
	if r := must(t, 4, v.bl("ping")...); !strings.Contains(r.stdout, "tls        failed") {
		t.Errorf("ping without the CA:\n%s", r.stdout)
	}
	// A webhook served over TLS.
	l := start(t, v.bl("bl", "listen", "--via", "webhook", "--listen", "127.0.0.1:0", "--tls-cert", certFile, "--tls-key", keyFile,
		"--ca", certFile, "--duration", "500ms")...)
	// The VTN does not trust the webhook's certificate: the subscription is refused.
	if code := l.wait(t); code != 6 {
		t.Errorf("listen over TLS with a certificate the VTN does not trust: exit %d\n%s", code, l.err)
	}
	_ = os.Remove(certFile)
}

func TestDiscover(t *testing.T) {
	// Whether anything answers depends on the network; the command itself
	// must end within --wait with a list, or say why multicast failed.
	r := run(t, "discover", "--wait", "300ms", "--format", "json")
	switch r.code {
	case 0:
		if d := decode[Discovered](t, r); d.VTNs == nil {
			t.Errorf("discover: %s", r.stdout)
		}
	case 4:
		t.Logf("no multicast here: %s", r.stdout)
	default:
		t.Errorf("discover: exit %d\n%s%s", r.code, r.stdout, r.stderr)
	}
	must(t, 3, "discover", "--wait", "0s")
	if r := run(t, "discover", "--wait", "200ms", "--first"); r.code != 0 && r.code != 4 {
		t.Errorf("discover --first: exit %d", r.code)
	}
}

// TestRoundTrip checks that what openadrctl writes of every kind of object
// reads back: get --format json | validate, and update --file.
func TestRoundTrip(t *testing.T) {
	v := startVTN(t, "--resource-groups", "--websocket")
	must(t, 0, v.ven("ven", "reports", "create", "--event", "price-2030-01-01", "--client-name", "site-42", "--type", "USAGE", "--value", "1")...)
	must(t, 0, v.bl("bl", "subscriptions", "create", "--client-name", "c", "--object", "EVENT", "--websocket")...)
	for kind, noun := range map[string]string{"program": "programs", "event": "events", "report": "reports", "ven": "vens",
		"resource": "resources", "subscription": "subscriptions", "resource-group": "resource-groups"} {
		list := decode[map[string][]json.RawMessage](t, must(t, 0, v.bl("bl", noun, "list", "--format", "json")...))
		objs := list[strings.ReplaceAll(noun, "-", "_")]
		if len(objs) == 0 {
			t.Fatalf("no %s", noun)
		}
		if r := runIn(t, string(objs[0]), "validate", kind, "-"); r.code != 0 {
			t.Errorf("validate %s from the VTN: exit %d %s\n%s", kind, r.code, r.stderr, objs[0])
		}
	}
}

// TestMQTTToken: a VTN whose clients log in to the broker with their token,
// and a broker that asks the VTN at POST /auth/mqtt whether to let them in.
func TestMQTTToken(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	broker, hook := startTokenBroker(t)
	hook.url.Store("http://" + addr + "/auth/mqtt")
	t.Setenv("OPENADRCTL_VTN_MQTT_PASSWORD", "vtn-secret")
	v := startVTN(t, "--listen", addr, "--mqtt", broker, "--mqtt-client-auth", "token", "--mqtt-username", "vtn")

	if r := must(t, 0, v.bl("bl", "notifiers")...); !strings.Contains(r.stdout, "OAUTH2_BEARER_TOKEN") || !strings.Contains(r.stdout, "{clientID}") {
		t.Errorf("notifiers:\n%s", r.stdout)
	}
	// The client's token as the password, the client ID as the user name.
	l1 := start(t, v.bl("bl", "listen", "--via", "mqtt", "--topics", "events", "--count", "1", "--format", "json")...)
	waitFor(t, l1.err, "listening: "+broker)
	must(t, 0, v.bl("bl", "events", "create", "--program", "dynamic-price", "--name", "by-token", "--type", "PRICE", "--value", "1", "--yes")...)
	if code := l1.wait(t); code != 0 {
		t.Fatalf("listen with a token: exit %d\n%s", code, l1.err)
	}
	if recs := lines(t, l1.out.String()); len(recs) != 1 || recs[0]["name"] != "by-token" {
		t.Errorf("records: %v", recs)
	}
	// The VTN's publisher logs in at its first notification, with its own password.
	waitFor(t, v.out, `"event":"mqtt_login","client":"vtn","status":200`)
	waitFor(t, v.out, `"event":"mqtt_login","client":"utility","status":200`)
	must(t, 0, v.ven("ven", "listen", "--via", "mqtt", "--topics", "ven-events:site-42", "--duration", "1s")...)

	// A password that is not a token: the broker refuses, exit 4.
	t.Setenv("OPENADRCTL_MQTT_PASSWORD", "not-a-token")
	r := must(t, 4, v.ven("ven", "listen", "--via", "mqtt", "--topics", "ven-events:site-42", "--timeout", "2s")...)
	if !strings.Contains(r.stderr, "not subscribed") {
		t.Errorf("refused login: %s", r.stderr)
	}
	waitFor(t, v.out, `"event":"mqtt_login","client":"site-42","status":403`)

	// The endpoint, as a broker calls it.
	tok := strings.TrimSpace(must(t, 0, v.ven("token")...).stdout)
	post := func(ct, body string) int {
		t.Helper()
		resp, err := http.Post(v.url+"/auth/mqtt", ct, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for _, c := range []struct {
		ct, body string
		want     int
	}{
		{"application/json", `{"username":"site-42","password":"` + tok + `"}`, 200},
		{"application/x-www-form-urlencoded", "username=site-42&password=" + tok, 200},
		{"application/json", `{"username":"utility","password":"` + tok + `"}`, 403}, // another client's token
		{"application/json", `{"username":"vtn","password":"vtn-secret"}`, 200},
		{"application/json", `{"username":"vtn","password":"wrong"}`, 403},
		{"application/json", `{"username":"site-42"}`, 403},
		{"application/json", `not json`, 400},
	} {
		if got := post(c.ct, c.body); got != c.want {
			t.Errorf("POST /auth/mqtt %s: %d, want %d", c.body, got, c.want)
		}
	}
	if resp, err := http.Get(v.url + "/auth/mqtt"); err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /auth/mqtt: %v %v", resp, err)
	} else {
		_ = resp.Body.Close()
	}

	// Without --mqtt-client-auth token there is no such endpoint.
	plain := startVTN(t)
	if resp, err := http.Post(plain.url+"/auth/mqtt", "application/json", strings.NewReader("{}")); err != nil || resp.StatusCode == http.StatusOK {
		t.Errorf("plain VTN: %v %v", resp, err)
	} else {
		_ = resp.Body.Close()
	}
	must(t, 3, "vtn", "start", "--mqtt-client-auth", "token")
	must(t, 3, "vtn", "start", "--mqtt", broker, "--mqtt-client-auth", "password")
	empty := filepath.Join(t.TempDir(), "empty.yaml")
	writeFile(t, empty, "programs: []\n")
	must(t, 3, "vtn", "start", "--config", empty, "--anonymous", "--mqtt", broker, "--mqtt-client-auth", "token")
}
