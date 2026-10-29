// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/discovery"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/cmd/client"
	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/runner"
)

// goOpenADRVersion is the version of go-openadr this binary was built with.
func goOpenADRVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/otfabric/go-openadr" {
				if d.Replace != nil {
					return d.Replace.Version + " (replaced)"
				}
				return d.Version
			}
		}
	}
	return "unknown"
}

func newTokenCmd() *cobra.Command {
	c := newConnConfig()
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Fetch an OAuth2 token with the client credentials",
		Long: `Ask the token endpoint for a token with the client-credentials grant, as the client
commands do: the endpoint is --token-url, or what the VTN names at GET /auth/server.

With --format text only the token is printed, for scripts and for tools such as curl:

  curl -H "Authorization: Bearer $(openadrctl token)" "$OPENADRCTL_URL/programs"

--format json and table show its type, lifetime and scope as well. A token is a secret:
treat what this prints as one.`,
		Example: `  OPENADRCTL_CLIENT_SECRET=utility-secret openadrctl token --url http://127.0.0.1:8080 --client-id utility
  openadrctl token --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runner.RunFormattedChecked(cmd, c.Format, func() error {
				if err := c.Validate(); err != nil {
					return err
				}
				if c.ClientID == "" {
					return errs.InvalidInput(errs.CodeInvalidInput, "token needs client credentials: --client-id and OPENADRCTL_CLIENT_SECRET or --client-secret-file", nil)
				}
				return nil
			}, func(ctx context.Context) (any, error) {
				cn, err := c.Open()
				if err != nil {
					return nil, err
				}
				ctx, cancel := context.WithTimeout(ctx, c.Timeout)
				defer cancel()
				t, err := cn.Tokens.Fetch(ctx)
				if err != nil {
					return nil, conn.Classify(err, false)
				}
				return t, nil
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

type discoverConfig struct {
	Wait   time.Duration `flag:"wait" env:"OPENADRCTL_DISCOVER_WAIT" desc:"How long to wait for answers"`
	First  bool          `flag:"first" desc:"Stop at the first VTN that answers"`
	Format string        `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

// Discovered is the result of discover.
type Discovered struct {
	VTNs []DiscoveredVTN `json:"vtns"`
}

// DiscoveredVTN is a VTN that answered.
type DiscoveredVTN struct {
	Instance     string            `json:"instance"`
	URL          string            `json:"url"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Addresses    []string          `json:"addresses,omitempty"`
	Version      string            `json:"version,omitempty"`
	RequiresAuth bool              `json:"requires_auth"`
	Programs     []string          `json:"programs,omitempty"`
	OpenAPIURL   string            `json:"openapi_url,omitempty"`
	TXT          map[string]string `json:"txt,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (d *Discovered) MarshalTextOutput() (string, error) {
	if len(d.VTNs) == 0 {
		return "no VTN answered", nil
	}
	return format.RenderTable(d)
}

// TableHeaders implements format.TableMarshaler.
func (d *Discovered) TableHeaders() []string {
	return []string{"INSTANCE", "URL", "VERSION", "AUTH", "ADDRESSES", "PROGRAMS"}
}

// TableRows implements format.TableMarshaler.
func (d *Discovered) TableRows() [][]string {
	rows := make([][]string, 0, len(d.VTNs))
	for _, v := range d.VTNs {
		auth := "none"
		if v.RequiresAuth {
			auth = "token"
		}
		rows = append(rows, []string{v.Instance, v.URL, oadr.List([]string{v.Version}), auth, oadr.List(v.Addresses), oadr.List(v.Programs)})
	}
	return rows
}

func newDiscoverCmd() *cobra.Command {
	c := discoverConfig{Wait: 3 * time.Second, Format: "text"}
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find VTNs on the local network (DNS-SD over multicast DNS)",
		Long: `Ask the local network for VTNs that announce themselves with DNS-SD (_openadr3._tcp),
as OpenADR 3 describes for VTNs on a local network, and list those that answer within
--wait. Finding none is not an error: the command exits 0 with an empty list.

It asks on every interface that has multicast and an IPv4 address. Multicast DNS does not
cross routers, and many cloud and container networks do not carry it at all (in Docker,
use the host network). Multicast DNS proves nothing about who answers: check a VTN you
found before giving it credentials.`,
		Example: `  openadrctl discover
  openadrctl discover --wait 10s --format json
  openadrctl discover --first`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runner.RunFormattedChecked(cmd, c.Format, func() error {
				if c.Wait <= 0 {
					return errs.InvalidInput(errs.CodeInvalidInput, "--wait must be positive", nil)
				}
				return nil
			}, func(ctx context.Context) (any, error) {
				vtns, err := discovery.Browse(ctx, discovery.Options{Timeout: c.Wait, First: c.First})
				if err != nil {
					return nil, errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, "multicast DNS: "+err.Error(), err)
				}
				out := &Discovered{VTNs: []DiscoveredVTN{}}
				for _, v := range vtns {
					d := DiscoveredVTN{Instance: v.Instance, URL: v.URL(), Host: v.Host, Port: v.Port, Version: v.Version,
						RequiresAuth: v.RequiresAuth, Programs: v.ProgramNames, OpenAPIURL: v.OpenAPIURL, TXT: v.Text}
					for _, a := range v.Addrs {
						d.Addresses = append(d.Addresses, a.String())
					}
					out.VTNs = append(out.VTNs, d)
				}
				return out, nil
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

// kinds are the kinds of object validate and timeline read.
var kinds = []string{"program", "event", "report", "ven", "resource", "subscription", "resource-group"}

type validateConfig struct {
	Spec   string `flag:"spec" env:"OPENADRCTL_SPEC" desc:"Revision of OpenADR the JSON is of: 3.1.0 or 3.0.1"`
	Format string `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

// Valid is the result of validate.
type Valid struct {
	Kind string `json:"kind"`
	File string `json:"file"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (v *Valid) MarshalTextOutput() (string, error) {
	s := fmt.Sprintf("%s: a valid %s", v.File, v.Kind)
	if v.Name != "" {
		s += " (" + v.Name + ")"
	}
	return s, nil
}

// TableHeaders implements format.TableMarshaler.
func (v *Valid) TableHeaders() []string { return []string{"FILE", "KIND", "NAME", "VALID"} }

// TableRows implements format.TableMarshaler.
func (v *Valid) TableRows() [][]string { return [][]string{{v.File, v.Kind, v.Name, "true"}} }

func newValidateCmd() *cobra.Command {
	c := validateConfig{Spec: string(openadr.Version310), Format: "text"}
	cmd := &cobra.Command{
		Use:   "validate <kind> <file>",
		Short: "Check an object against the OpenADR schema, offline",
		Long: `Read the JSON of an object from a file (- for stdin) and check it as a client does before
it sends one: the properties a client provides, against the OpenADR 3.1.0 schema, or that
of 3.0.1 with --spec 3.0.1. Kinds:
` + strings.Join(kinds, ", ") + `.

A valid object exits 0. One that is not exits 3 with the property that is wrong (with
--format json in openadr.field). Nothing is sent anywhere.`,
		Example: `  openadrctl validate event event.json
  openadrctl bl events get 4e5f6a7b --format json | openadrctl validate event -`,
		Args:      cobra.ExactArgs(2),
		ValidArgs: kinds,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunFormattedChecked(cmd, c.Format, func() error {
				v := openadr.Version(strings.TrimSpace(c.Spec))
				if !v.Supported() {
					return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--spec %q: 3.1.0 or 3.0.1", c.Spec), nil)
				}
				oadr.SetRevision(v)
				return checkKind(args[0])
			}, func(context.Context) (any, error) {
				v, err := readObject(args[0], args[1], cmd)
				if err != nil {
					return nil, err
				}
				if val, ok := v.(interface{ Validate() error }); ok {
					if err := val.Validate(); err != nil {
						e := errs.InvalidInput(errs.CodeInvalidObject, strings.TrimPrefix(err.Error(), "openadr: "), err)
						var ve *openadr.ValidationError
						if errors.As(err, &ve) {
							e = e.WithDetail(conn.HTTPDetail{Field: ve.Field})
						}
						return nil, e
					}
				}
				out := &Valid{Kind: args[0], File: args[1], ID: oadr.MetadataOf(v).ID}
				if k := client.KindOf(v); k != nil {
					if row := k.Row(v); len(row) > 1 && row[1] != "-" {
						out.Name = row[1]
					}
				}
				return out, nil
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

func checkKind(k string) error {
	for _, x := range kinds {
		if x == k {
			return nil
		}
	}
	return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q is not a kind of object: %s", k, strings.Join(kinds, ", ")), nil)
}

func readObject(kind, file string, cmd *cobra.Command) (any, error) {
	in := cmd.InOrStdin()
	switch kind {
	case "program":
		return oadr.ReadFile[openadr.Program](file, in)
	case "event":
		return oadr.ReadFile[openadr.Event](file, in)
	case "report":
		return oadr.ReadFile[openadr.Report](file, in)
	case "ven":
		return oadr.ReadFile[openadr.VEN](file, in)
	case "resource":
		return oadr.ReadFile[openadr.Resource](file, in)
	case "subscription":
		return oadr.ReadFile[openadr.Subscription](file, in)
	default:
		return oadr.ReadFile[openadr.ResourceGroup](file, in)
	}
}

type timelineConfig struct {
	client.TimelineFlags
	Now    string `flag:"now" desc:"The time \"now\" stands for in the event: RFC 3339 (default: the current time)"`
	Format string `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

func newTimelineCmd() *cobra.Command {
	c := timelineConfig{Format: "text"}
	cmd := &cobra.Command{
		Use:   "timeline <event-file>",
		Short: "Lay out the intervals of an event read from a file, offline",
		Long: `Read the JSON of an event from a file (- for stdin) and lay out its intervals in time, as
"ven events timeline" does for an event of a VTN. --csv writes the table as CSV.`,
		Example: `  openadrctl timeline event.json
  openadrctl timeline event.json --csv > intervals.csv
  openadrctl bl events get 4e5f6a7b --format json | openadrctl timeline - --at 2030-01-01T12:00:00Z`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.RunFormattedChecked(cmd, c.Format, func() error { return nil }, func(context.Context) (any, error) {
				e, err := oadr.ReadFile[openadr.Event](args[0], cmd.InOrStdin())
				if err != nil {
					return nil, err
				}
				if c.Now != "" {
					t, err := time.Parse(time.RFC3339, c.Now)
					if err != nil {
						return nil, errs.InvalidInput(errs.CodeInvalidInput, "--now "+strconv.Quote(c.Now)+": an RFC 3339 time", err)
					}
					return c.TableAt(e, t)
				}
				return c.Table(e)
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}
