// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
)

func programsCmd(role Role) *cobra.Command {
	n := noun[openadr.Program, client.ProgramQuery]{
		kind: oadr.Programs, use: "programs", aliases: []string{"program"},
		short: "Programs: the demand-response offers of the VTN",
		list: func(a *api, ctx context.Context, q client.ProgramQuery) ([]openadr.Program, error) {
			return pick(a).Programs(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.Program, error) { return pick(a).Program(ctx, id) },
		query: func(cmd *cobra.Command) func(client.Page) client.ProgramQuery {
			var targets []string
			cmd.Flags().StringSliceVar(&targets, "target", nil, "Programs with one of these targets (repeatable)")
			return func(p client.Page) client.ProgramQuery { return client.ProgramQuery{Targets: targets, Page: p} }
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.Program) error {
			name := stringFlag(cmd, "name", "Name of the program (unique on the VTN)")
			targets := listFlag(cmd, "target", "Targets of the program (repeatable): only VENs granted one see it")
			urls := listFlag(cmd, "description-url", "URL of a description of the program (repeatable)")
			payloads := listFlag(cmd, "payload", "Payload of its events as TYPE[:units[:currency]] (repeatable), such as PRICE::EUR")
			attrs := attributesFlag(cmd, "attribute")
			return func(_ *cobra.Command, p *openadr.Program) error {
				name(func(s string) { p.Name = s })
				targets(func(s []string) { p.Targets = s })
				urls(func(s []string) {
					p.Descriptions = nil
					for _, u := range s {
						p.Descriptions = append(p.Descriptions, openadr.ProgramDescription{URL: u})
					}
				})
				payloads(func(s []string) {
					p.PayloadDescriptors = nil
					for _, d := range s {
						parts := strings.SplitN(d, ":", 3)
						pd := openadr.EventPayloadDescriptor{PayloadType: parts[0]}
						if len(parts) > 1 {
							pd.Units = parts[1]
						}
						if len(parts) > 2 {
							pd.Currency = parts[2]
						}
						p.PayloadDescriptors = append(p.PayloadDescriptors, pd)
					}
				})
				return attrs(func(m []openadr.ValuesMap) { p.Attributes = m })
			}
		},
		examples: map[string]string{
			"list":   "  openadrctl " + string(role) + " programs list\n  openadrctl " + string(role) + " programs list --target group-a --format json",
			"get":    "  openadrctl " + string(role) + " programs get 0a1b2c3d",
			"create": "  openadrctl bl programs create --name dynamic-price --payload PRICE::EUR\n  openadrctl bl programs create --file program.json --dry-run",
			"update": "  openadrctl bl programs update 0a1b2c3d --target group-a --target group-b",
			"delete": "  openadrctl bl programs delete 0a1b2c3d --yes",
		},
	}
	if role == BL {
		n.create = func(a *api, ctx context.Context, p openadr.Program) (openadr.Program, error) {
			return a.bl.CreateProgram(ctx, p)
		}
		n.update = func(a *api, ctx context.Context, p openadr.Program) (openadr.Program, error) {
			return a.bl.UpdateProgram(ctx, p)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.Program, error) {
			return a.bl.DeleteProgram(ctx, id)
		}
	}
	return n.command(role)
}

func eventsCmd(role Role) *cobra.Command {
	n := noun[openadr.Event, client.EventQuery]{
		kind: oadr.Events, use: "events", aliases: []string{"event"},
		short: "Events: what a program asks of VENs, interval by interval",
		list: func(a *api, ctx context.Context, q client.EventQuery) ([]openadr.Event, error) {
			return pick(a).Events(ctx, q)
		},
		get:   func(a *api, ctx context.Context, id string) (openadr.Event, error) { return pick(a).Event(ctx, id) },
		query: eventQuery,
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.Event) error {
			program := stringFlag(cmd, "program", "ID of the program of the event")
			name := stringFlag(cmd, "name", "Name of the event")
			targets := listFlag(cmd, "target", "Targets of the event (repeatable)")
			var priority int
			cmd.Flags().IntVar(&priority, "priority", 0, "Priority: 0 is the highest")
			duration := stringFlag(cmd, "duration", "Duration of the whole event (ISO 8601 or 1h), when not that of its intervals")
			reports := listFlag(cmd, "report", "Ask VENs for a report of this payload type (repeatable), such as USAGE")
			var iv intervals
			iv.register(cmd, "intervals")
			return func(cmd *cobra.Command, e *openadr.Event) error {
				program(func(s string) { e.ProgramID = s })
				name(func(s string) { e.Name = s })
				targets(func(s []string) { e.Targets = s })
				if cmd.Flags().Changed("priority") {
					e.Priority = &priority
				}
				var err error
				duration(func(s string) {
					var d openadr.Duration
					if d, err = parseDuration("duration", s); err == nil {
						e.Duration = &d
					}
				})
				if err != nil {
					return err
				}
				reports(func(s []string) {
					e.ReportDescriptors = nil
					for _, t := range s {
						e.ReportDescriptors = append(e.ReportDescriptors, openadr.NewReportDescriptor(t))
					}
				})
				if iv.given(cmd) {
					period, intervals, err := iv.build()
					if err != nil {
						return err
					}
					e.IntervalPeriod, e.Intervals = period, intervals
				}
				return nil
			}
		},
		sensitive: true,
		examples: map[string]string{
			"list": "  openadrctl " + string(role) + " events list --program 0a1b2c3d --active\n" +
				"  openadrctl " + string(role) + " events list --target group-a --format table",
			"get": "  openadrctl " + string(role) + " events get 4e5f6a7b --format json",
			"create": "  openadrctl bl events create --program 0a1b2c3d --name tomorrow --type PRICE \\\n" +
				"      --start 2030-01-01T00:00:00Z --interval PT1H --value 0.17 --value 0.21 --value 0.25 --yes\n" +
				"  openadrctl bl events create --program 0a1b2c3d --csv prices.csv --yes",
			"update": "  openadrctl bl events update 4e5f6a7b --priority 1 --yes",
			"delete": "  openadrctl bl events delete 4e5f6a7b --yes",
		},
	}
	if role == BL {
		n.create = func(a *api, ctx context.Context, e openadr.Event) (openadr.Event, error) {
			return a.bl.CreateEvent(ctx, e)
		}
		n.update = func(a *api, ctx context.Context, e openadr.Event) (openadr.Event, error) {
			return a.bl.UpdateEvent(ctx, e)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.Event, error) { return a.bl.DeleteEvent(ctx, id) }
	}
	return n.command(role, newTimelineCmd(role), newWatchCmd(role))
}

// eventQuery registers the filters of an event search.
func eventQuery(cmd *cobra.Command) func(client.Page) client.EventQuery {
	var program string
	var targets []string
	var active bool
	cmd.Flags().StringVar(&program, "program", "", "Events of this program")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "Events with one of these targets (repeatable)")
	cmd.Flags().BoolVar(&active, "active", false, "Leave out events that are over")
	return func(p client.Page) client.EventQuery {
		return client.EventQuery{ProgramID: program, Targets: targets, ActiveOnly: active, Page: p}
	}
}

func reportsCmd(role Role) *cobra.Command {
	n := noun[openadr.Report, client.ReportQuery]{
		kind: oadr.Reports, use: "reports", aliases: []string{"report"},
		short: "Reports: what VENs say about events",
		list: func(a *api, ctx context.Context, q client.ReportQuery) ([]openadr.Report, error) {
			return pick(a).Reports(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.Report, error) { return pick(a).Report(ctx, id) },
		query: func(cmd *cobra.Command) func(client.Page) client.ReportQuery {
			var program, event, clientName string
			cmd.Flags().StringVar(&program, "program", "", "Reports on events of this program")
			cmd.Flags().StringVar(&event, "event", "", "Reports on this event")
			cmd.Flags().StringVar(&clientName, "client-name", "", "Reports with this client name")
			return func(p client.Page) client.ReportQuery {
				return client.ReportQuery{ProgramID: program, EventID: event, ClientName: clientName, Page: p}
			}
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.Report) error {
			event := stringFlag(cmd, "event", "ID of the event the report is on")
			clientName := stringFlag(cmd, "client-name", "Name of the reporting client")
			name := stringFlag(cmd, "name", "Name of the report")
			var resource, readingType, units string
			cmd.Flags().StringVar(&resource, "resource", openadr.AggregatedReport, "Name of the resource the intervals are of")
			cmd.Flags().StringVar(&readingType, "reading-type", "", "Reading type of the payload, such as DIRECT_READ")
			cmd.Flags().StringVar(&units, "units", "", "Units of the payload, such as KWH")
			var iv intervals
			iv.register(cmd, "readings")
			return func(cmd *cobra.Command, r *openadr.Report) error {
				event(func(s string) { r.EventID = s })
				clientName(func(s string) { r.ClientName = s })
				name(func(s string) { r.Name = s })
				if !iv.given(cmd) {
					return nil
				}
				period, intervals, err := iv.build()
				if err != nil {
					return err
				}
				// The readings replace those of their resource.
				res := openadr.ReportResource{ResourceName: resource, IntervalPeriod: period, Intervals: intervals}
				kept := r.Resources[:0:0]
				for _, x := range r.Resources {
					if x.ResourceName != resource {
						kept = append(kept, x)
					}
				}
				r.Resources = append(kept, res)
				types := map[string]bool{}
				for _, x := range r.PayloadDescriptors {
					types[x.PayloadType] = true
				}
				for _, i := range intervals {
					for _, p := range i.Payloads {
						if !types[p.Type] {
							types[p.Type] = true
							r.PayloadDescriptors = append(r.PayloadDescriptors, openadr.ReportPayloadDescriptor{PayloadType: p.Type, ReadingType: readingType, Units: units})
						}
					}
				}
				return nil
			}
		},
		examples: map[string]string{
			"list":   "  openadrctl " + string(role) + " reports list --event 4e5f6a7b",
			"get":    "  openadrctl " + string(role) + " reports get 9c8d7e6f --format json",
			"create": "  openadrctl ven reports create --event 4e5f6a7b --client-name site-42 --resource meter-1 \\\n      --type USAGE --units KWH --start 2030-01-01T00:00:00Z --interval PT1H --value 1.2 --value 0.9",
			"update": "  openadrctl ven reports update 9c8d7e6f --resource meter-1 --type USAGE --start 2030-01-01T00:00:00Z --interval PT1H --value 1.3",
			"delete": "  openadrctl ven reports delete 9c8d7e6f --yes",
		},
	}
	if role == VEN {
		n.create = func(a *api, ctx context.Context, r openadr.Report) (openadr.Report, error) {
			return a.ven.CreateReport(ctx, r)
		}
		n.update = func(a *api, ctx context.Context, r openadr.Report) (openadr.Report, error) {
			return a.ven.UpdateReport(ctx, r)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.Report, error) {
			return a.ven.DeleteReport(ctx, id)
		}
	}
	return n.command(role)
}

func vensCmd(role Role) *cobra.Command {
	n := noun[openadr.VEN, client.VENQuery]{
		kind: oadr.VENs, use: "vens", aliases: []string{"ven"},
		short: "VENs: the VEN objects the VTN knows",
		list: func(a *api, ctx context.Context, q client.VENQuery) ([]openadr.VEN, error) {
			return pick(a).VENs(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.VEN, error) { return pick(a).VEN(ctx, id) },
		query: func(cmd *cobra.Command) func(client.Page) client.VENQuery {
			var name string
			var targets []string
			cmd.Flags().StringVar(&name, "name", "", "The VEN of this name")
			cmd.Flags().StringSliceVar(&targets, "target", nil, "VENs with one of these targets (repeatable)")
			return func(p client.Page) client.VENQuery { return client.VENQuery{Name: name, Targets: targets, Page: p} }
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.VEN) error {
			name := stringFlag(cmd, "name", "Name of the VEN (unique on the VTN)")
			attrs := attributesFlag(cmd, "attribute")
			var targets func(func([]string))
			var owner func(func(string))
			if role == BL {
				targets = listFlag(cmd, "target", "Targets granted to the VEN (repeatable)")
				owner = stringFlag(cmd, "ven-client-id", "Client ID the VEN authenticates with")
			}
			return func(_ *cobra.Command, v *openadr.VEN) error {
				name(func(s string) { v.Name = s })
				if role == BL {
					targets(func(s []string) { v.Targets = s })
					owner(func(s string) { v.ClientID = s })
				}
				return attrs(func(m []openadr.ValuesMap) { v.Attributes = m })
			}
		},
		examples: map[string]string{
			"list":   "  openadrctl " + string(role) + " vens list",
			"get":    "  openadrctl " + string(role) + " vens get 1f2e3d4c",
			"create": "  openadrctl ven vens create --name site-42\n  openadrctl bl vens create --name site-42 --ven-client-id site-42 --target group-a",
			"update": "  openadrctl bl vens update 1f2e3d4c --target group-a --target group-b",
			"delete": "  openadrctl " + string(role) + " vens delete 1f2e3d4c --yes",
		},
	}
	switch role {
	case VEN:
		n.create = func(a *api, ctx context.Context, v openadr.VEN) (openadr.VEN, error) { return a.ven.CreateVEN(ctx, v) }
		n.update = func(a *api, ctx context.Context, v openadr.VEN) (openadr.VEN, error) { return a.ven.UpdateVEN(ctx, v) }
		n.del = func(a *api, ctx context.Context, id string) (openadr.VEN, error) { return a.ven.DeleteVEN(ctx, id) }
	case BL:
		n.create = func(a *api, ctx context.Context, v openadr.VEN) (openadr.VEN, error) { return a.bl.CreateVEN(ctx, v) }
		n.update = func(a *api, ctx context.Context, v openadr.VEN) (openadr.VEN, error) { return a.bl.UpdateVEN(ctx, v) }
		n.del = func(a *api, ctx context.Context, id string) (openadr.VEN, error) { return a.bl.DeleteVEN(ctx, id) }
	}
	return n.command(role)
}

func resourcesCmd(role Role) *cobra.Command {
	n := noun[openadr.Resource, client.ResourceQuery]{
		kind: oadr.Resources, use: "resources", aliases: []string{"resource"},
		short: "Resources: the devices behind a VEN",
		list: func(a *api, ctx context.Context, q client.ResourceQuery) ([]openadr.Resource, error) {
			return pick(a).Resources(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.Resource, error) {
			return pick(a).Resource(ctx, id)
		},
		query: func(cmd *cobra.Command) func(client.Page) client.ResourceQuery {
			var name, ven string
			var targets []string
			cmd.Flags().StringVar(&name, "name", "", "Resources of this name")
			cmd.Flags().StringVar(&ven, "ven", "", "Resources of this VEN")
			cmd.Flags().StringSliceVar(&targets, "target", nil, "Resources with one of these targets (repeatable)")
			return func(p client.Page) client.ResourceQuery {
				return client.ResourceQuery{Name: name, VENID: ven, Targets: targets, Page: p}
			}
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.Resource) error {
			name := stringFlag(cmd, "name", "Name of the resource (unique within its VEN)")
			ven := stringFlag(cmd, "ven", "ID of the VEN of the resource")
			attrs := attributesFlag(cmd, "attribute")
			var targets func(func([]string))
			var owner func(func(string))
			if role == BL {
				targets = listFlag(cmd, "target", "Targets granted to the resource (repeatable)")
				owner = stringFlag(cmd, "ven-client-id", "Client ID of the VEN that owns the resource")
			}
			return func(_ *cobra.Command, r *openadr.Resource) error {
				name(func(s string) { r.Name = s })
				ven(func(s string) { r.VENID = s })
				if role == BL {
					targets(func(s []string) { r.Targets = s })
					owner(func(s string) { r.ClientID = s })
				}
				return attrs(func(m []openadr.ValuesMap) { r.Attributes = m })
			}
		},
		examples: map[string]string{
			"list":   "  openadrctl " + string(role) + " resources list --ven 1f2e3d4c",
			"get":    "  openadrctl " + string(role) + " resources get 5a6b7c8d",
			"create": "  openadrctl " + string(role) + " resources create --ven 1f2e3d4c --name meter-1 --attribute MAX_POWER_CONSUMPTION=11",
			"update": "  openadrctl " + string(role) + " resources update 5a6b7c8d --attribute MAX_POWER_CONSUMPTION=22",
			"delete": "  openadrctl " + string(role) + " resources delete 5a6b7c8d --yes",
		},
	}
	switch role {
	case VEN:
		n.create = func(a *api, ctx context.Context, r openadr.Resource) (openadr.Resource, error) {
			return a.ven.CreateResource(ctx, r)
		}
		n.update = func(a *api, ctx context.Context, r openadr.Resource) (openadr.Resource, error) {
			return a.ven.UpdateResource(ctx, r)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.Resource, error) {
			return a.ven.DeleteResource(ctx, id)
		}
	case BL:
		n.create = func(a *api, ctx context.Context, r openadr.Resource) (openadr.Resource, error) {
			return a.bl.CreateResource(ctx, r)
		}
		n.update = func(a *api, ctx context.Context, r openadr.Resource) (openadr.Resource, error) {
			return a.bl.UpdateResource(ctx, r)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.Resource, error) {
			return a.bl.DeleteResource(ctx, id)
		}
	}
	return n.command(role)
}

func subscriptionsCmd(role Role) *cobra.Command {
	n := noun[openadr.Subscription, client.SubscriptionQuery]{
		kind: oadr.Subscriptions, use: "subscriptions", aliases: []string{"subscription", "subs"},
		short: "Subscriptions: what the VTN notifies a client of, and where",
		long: `Subscriptions ask the VTN to notify a webhook (callback URL) or, on a VTN that has the
extension, a WebSocket of operations on objects. "listen" subscribes and receives in one.`,
		list: func(a *api, ctx context.Context, q client.SubscriptionQuery) ([]openadr.Subscription, error) {
			if a.role == VEN {
				return a.ven.Subscriptions(ctx, q)
			}
			return a.bl.Subscriptions(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.Subscription, error) {
			if a.role == VEN {
				return a.ven.Subscription(ctx, id)
			}
			return a.bl.Subscription(ctx, id)
		},
		create: func(a *api, ctx context.Context, s openadr.Subscription) (openadr.Subscription, error) {
			if a.role == VEN {
				return a.ven.CreateSubscription(ctx, s)
			}
			return a.bl.CreateSubscription(ctx, s)
		},
		update: func(a *api, ctx context.Context, s openadr.Subscription) (openadr.Subscription, error) {
			if a.role == VEN {
				return a.ven.UpdateSubscription(ctx, s)
			}
			return a.bl.UpdateSubscription(ctx, s)
		},
		del: func(a *api, ctx context.Context, id string) (openadr.Subscription, error) {
			if a.role == VEN {
				return a.ven.DeleteSubscription(ctx, id)
			}
			return a.bl.DeleteSubscription(ctx, id)
		},
		query: func(cmd *cobra.Command) func(client.Page) client.SubscriptionQuery {
			var program, clientName string
			var objects []string
			cmd.Flags().StringVar(&program, "program", "", "Subscriptions to this program")
			cmd.Flags().StringVar(&clientName, "client-name", "", "Subscriptions of this client name")
			cmd.Flags().StringSliceVar(&objects, "object", nil, "Subscriptions to one of these kinds of object (repeatable): PROGRAM, EVENT, ...")
			return func(p client.Page) client.SubscriptionQuery {
				q := client.SubscriptionQuery{ProgramID: program, ClientName: clientName, Page: p}
				for _, o := range objects {
					q.Objects = append(q.Objects, openadr.ObjectType(strings.ToUpper(o)))
				}
				return q
			}
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.Subscription) error {
			clientName := stringFlag(cmd, "client-name", "Name of the subscribing client")
			program := stringFlag(cmd, "program", "Only this program, its events and the reports on them")
			targets := listFlag(cmd, "target", "Targets the subscriber names, as in a search (repeatable)")
			var sel selection
			sel.register(cmd)
			var callback, token string
			var ws bool
			cmd.Flags().StringVar(&callback, "callback-url", "", "Webhook to notify (https; the VTN checks that it answers first)")
			cmd.Flags().StringVar(&token, "webhook-token", "", "Bearer token the VTN sends to the webhook, made up for the subscription")
			cmd.Flags().BoolVar(&ws, "websocket", false, "Notify over the client's WebSocket instead of a webhook (extension)")
			return func(cmd *cobra.Command, s *openadr.Subscription) error {
				clientName(func(v string) { s.ClientName = v })
				program(func(v string) { s.ProgramID = v })
				targets(func(v []string) { s.Targets = v })
				if !sel.given(cmd) && !cmd.Flags().Changed("callback-url") && !ws {
					return nil
				}
				if ws && callback != "" {
					return errs.InvalidInput(errs.CodeInvalidFlagCombination, "give --callback-url or --websocket, not both", nil)
				}
				oo, err := sel.operation()
				if err != nil {
					return err
				}
				oo.CallbackURL, oo.BearerToken = callback, token
				if ws {
					oo.Mechanism = openadr.MechanismWebSocket
				}
				s.ObjectOperations = []openadr.ObjectOperation{oo}
				return nil
			}
		},
		examples: map[string]string{
			"list": "  openadrctl " + string(role) + " subscriptions list",
			"get":  "  openadrctl " + string(role) + " subscriptions get 2b3c4d5e",
			"create": "  openadrctl " + string(role) + " subscriptions create --client-name site-42 --object EVENT \\\n" +
				"      --operation CREATE --operation UPDATE --callback-url https://site-42.example/openadr --webhook-token s3cret",
			"update": "  openadrctl " + string(role) + " subscriptions update 2b3c4d5e --program 0a1b2c3d",
			"delete": "  openadrctl " + string(role) + " subscriptions delete 2b3c4d5e --yes",
		},
	}
	return n.command(role)
}

// selection is what a subscription is to: kinds of object and operations.
type selection struct {
	objects    []string
	operations []string
}

func (s *selection) register(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&s.objects, "object", nil, "Kinds of object (repeatable): PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN, RESOURCE (default: EVENT)")
	cmd.Flags().StringSliceVar(&s.operations, "operation", nil, "Operations (repeatable): CREATE, UPDATE, DELETE, READ (default: CREATE, UPDATE, DELETE)")
}

func (s *selection) given(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("object") || cmd.Flags().Changed("operation")
}

func (s *selection) operation() (openadr.ObjectOperation, error) {
	var oo openadr.ObjectOperation
	objects := s.objects
	if len(objects) == 0 {
		objects = []string{"EVENT"}
	}
	for _, o := range objects {
		t := openadr.ObjectType(strings.ToUpper(o))
		switch t {
		case openadr.ObjectProgram, openadr.ObjectEvent, openadr.ObjectReport, openadr.ObjectSubscription, openadr.ObjectVEN, openadr.ObjectResource:
		default:
			return oo, bad("object", "%q: PROGRAM, EVENT, REPORT, SUBSCRIPTION, VEN or RESOURCE", o)
		}
		oo.Objects = append(oo.Objects, t)
	}
	ops := s.operations
	if len(ops) == 0 {
		ops = []string{"CREATE", "UPDATE", "DELETE"}
	}
	for _, o := range ops {
		op := openadr.Operation(strings.ToUpper(o))
		switch op {
		case openadr.OperationCreate, openadr.OperationUpdate, openadr.OperationDelete, openadr.OperationRead:
		default:
			return oo, bad("operation", "%q: CREATE, UPDATE, DELETE or READ", o)
		}
		oo.Operations = append(oo.Operations, op)
	}
	return oo, nil
}

func resourceGroupsCmd(role Role) *cobra.Command {
	n := noun[openadr.ResourceGroup, client.ResourceGroupQuery]{
		kind: oadr.ResourceGroups, use: "resource-groups", aliases: []string{"groups", "resource-group"},
		short: "Resource groups: resources and groups under one name (extension)",
		long: `Resource groups are not part of OpenADR 3.1.0: they are an extension of OpenLEADR that
go-openadr shares. A VTN without them answers 404 or 501.`,
		list: func(a *api, ctx context.Context, q client.ResourceGroupQuery) ([]openadr.ResourceGroup, error) {
			return pick(a).ResourceGroups(ctx, q)
		},
		get: func(a *api, ctx context.Context, id string) (openadr.ResourceGroup, error) {
			return pick(a).ResourceGroup(ctx, id)
		},
		query: func(cmd *cobra.Command) func(client.Page) client.ResourceGroupQuery {
			var name string
			var targets []string
			cmd.Flags().StringVar(&name, "name", "", "Groups of this name")
			cmd.Flags().StringSliceVar(&targets, "target", nil, "Groups with one of these targets (repeatable)")
			return func(p client.Page) client.ResourceGroupQuery {
				return client.ResourceGroupQuery{Name: name, Targets: targets, Page: p}
			}
		},
		fields: func(cmd *cobra.Command) func(*cobra.Command, *openadr.ResourceGroup) error {
			name := stringFlag(cmd, "name", "Name of the group")
			targets := listFlag(cmd, "target", "Targets of the group (repeatable)")
			children := listFlag(cmd, "child", "Member as resource:<id> or group:<id> (repeatable)")
			attrs := attributesFlag(cmd, "attribute")
			return func(_ *cobra.Command, g *openadr.ResourceGroup) error {
				name(func(s string) { g.Name = s })
				targets(func(s []string) { g.Targets = s })
				var err error
				children(func(s []string) {
					g.Children = nil
					for _, c := range s {
						typ, id, _ := strings.Cut(c, ":")
						switch typ {
						case "resource", string(openadr.ChildResource):
							g.Children = append(g.Children, openadr.ResourceGroupChild{Type: openadr.ChildResource, ID: id})
						case "group", string(openadr.ChildResourceGroup):
							g.Children = append(g.Children, openadr.ResourceGroupChild{Type: openadr.ChildResourceGroup, ID: id})
						default:
							err = bad("child", "%q: resource:<id> or group:<id>", c)
						}
					}
				})
				if err != nil {
					return err
				}
				return attrs(func(m []openadr.ValuesMap) { g.Attributes = m })
			}
		},
		examples: map[string]string{
			"list":   "  openadrctl " + string(role) + " resource-groups list",
			"get":    "  openadrctl " + string(role) + " resource-groups get 3c4d5e6f",
			"create": "  openadrctl bl resource-groups create --name feeder-7 --child resource:5a6b7c8d --child resource:6b7c8d9e",
			"update": "  openadrctl bl resource-groups update 3c4d5e6f --child resource:5a6b7c8d",
			"delete": "  openadrctl bl resource-groups delete 3c4d5e6f --yes",
		},
	}
	if role == BL {
		n.create = func(a *api, ctx context.Context, g openadr.ResourceGroup) (openadr.ResourceGroup, error) {
			return a.bl.CreateResourceGroup(ctx, g)
		}
		n.update = func(a *api, ctx context.Context, g openadr.ResourceGroup) (openadr.ResourceGroup, error) {
			return a.bl.UpdateResourceGroup(ctx, g)
		}
		n.del = func(a *api, ctx context.Context, id string) (openadr.ResourceGroup, error) {
			return a.bl.DeleteResourceGroup(ctx, id)
		}
	}
	return n.command(role)
}

// reader is what both roles can read.
type reader interface {
	Programs(context.Context, client.ProgramQuery) ([]openadr.Program, error)
	Program(context.Context, string) (openadr.Program, error)
	Events(context.Context, client.EventQuery) ([]openadr.Event, error)
	Event(context.Context, string) (openadr.Event, error)
	Reports(context.Context, client.ReportQuery) ([]openadr.Report, error)
	Report(context.Context, string) (openadr.Report, error)
	VENs(context.Context, client.VENQuery) ([]openadr.VEN, error)
	VEN(context.Context, string) (openadr.VEN, error)
	Resources(context.Context, client.ResourceQuery) ([]openadr.Resource, error)
	Resource(context.Context, string) (openadr.Resource, error)
	ResourceGroups(context.Context, client.ResourceGroupQuery) ([]openadr.ResourceGroup, error)
	ResourceGroup(context.Context, string) (openadr.ResourceGroup, error)
	Notifiers(context.Context) (openadr.Notifiers, error)
	MQTTTopics(context.Context, client.TopicSet) (openadr.Topics, error)
	NotificationStream(context.Context) (*client.NotificationStream, error)
}

func pick(a *api) reader {
	if a.role == VEN {
		return a.ven
	}
	return a.bl
}
