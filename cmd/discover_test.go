// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-openadr/discovery"
)

// TestDiscoverResult runs discover with the browse replaced: what it writes
// of the VTNs found, and the exit code when multicast fails, without the
// network.
func TestDiscoverResult(t *testing.T) {
	defer func(b func(context.Context, discovery.Options) ([]discovery.VTN, error)) { browse = b }(browse)
	var asked discovery.Options
	browse = func(_ context.Context, o discovery.Options) ([]discovery.VTN, error) {
		asked = o
		return []discovery.VTN{
			{Instance: "VTN a", Host: "a.local", Port: 8443, Addrs: []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("fe80::1")},
				Version: "3.1.0", BasePath: "openadr3/3.1.0", ProgramNames: []string{"p1", "p2"}, RequiresAuth: true,
				OpenAPIURL: "https://a.local:8443/openapi.yaml", Text: map[string]string{"version": "3.1.0", "other": "x"}},
			{Instance: "VTN b", Host: "b.local", Port: 8080, LocalURL: "http://b.local:8080/"},
		}, nil
	}
	r := must(t, 0, "discover", "--wait", "2s", "--first", "--format", "json")
	if asked.Timeout != 2*time.Second || !asked.First {
		t.Errorf("asked with %+v", asked)
	}
	want := Discovered{VTNs: []DiscoveredVTN{
		{Instance: "VTN a", URL: "https://a.local:8443/openadr3/3.1.0", Host: "a.local", Port: 8443, Addresses: []string{"192.0.2.1", "fe80::1"},
			Version: "3.1.0", RequiresAuth: true, Programs: []string{"p1", "p2"}, OpenAPIURL: "https://a.local:8443/openapi.yaml",
			TXT: map[string]string{"version": "3.1.0", "other": "x"}},
		{Instance: "VTN b", URL: "http://b.local:8080/", Host: "b.local", Port: 8080},
	}}
	if got := decode[Discovered](t, r); !reflect.DeepEqual(got, want) {
		t.Errorf("discover:\n%+v\nwant\n%+v", got, want)
	}
	if r := must(t, 0, "discover", "--format", "table"); !strings.Contains(r.stdout, "VTN a") || !strings.Contains(r.stdout, "token") {
		t.Errorf("table:\n%s", r.stdout)
	}
	browse = func(context.Context, discovery.Options) ([]discovery.VTN, error) { return nil, nil }
	if r := must(t, 0, "discover", "--format", "json"); strings.ReplaceAll(r.stdout, "\r\n", "\n") != "{\n  \"vtns\": []\n}\n" {
		t.Errorf("none: %q", r.stdout)
	}
	if r := must(t, 0, "discover"); !strings.Contains(r.stdout, "no VTN answered") {
		t.Errorf("none, text: %q", r.stdout)
	}
	browse = func(context.Context, discovery.Options) ([]discovery.VTN, error) {
		return nil, errors.New("no interface with multicast")
	}
	r = must(t, 4, "discover", "--format", "json")
	if e := decode[envelope](t, r); e.Error.Kind != "transport" || e.Error.Code != "transport_connect_failed" || !strings.Contains(e.Error.Message, "multicast DNS: no interface with multicast") {
		t.Errorf("no multicast: %+v", e)
	}
	must(t, 3, "discover", "--wait", "0s")
}
