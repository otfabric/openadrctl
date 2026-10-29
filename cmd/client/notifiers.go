// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"sort"
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/runner"
)

// Notifiers is the result of notifiers: how the VTN notifies.
type Notifiers struct {
	Webhook   bool  `json:"webhook"`
	WebSocket bool  `json:"websocket"`
	MQTT      *MQTT `json:"mqtt,omitempty"`
}

// MQTT is the broker a VTN announces.
type MQTT struct {
	URIs           []string `json:"uris"`
	Serialization  string   `json:"serialization,omitempty"`
	Authentication string   `json:"authentication"`
	Username       string   `json:"username,omitempty"`
}

func notifiersOf(n openadr.Notifiers) *Notifiers {
	out := &Notifiers{Webhook: n.Webhook, WebSocket: n.WebSocket}
	if n.MQTT != nil {
		method := n.MQTT.Authentication.Method
		if method == "" {
			method = openadr.MQTTAnonymous
		}
		out.MQTT = &MQTT{URIs: n.MQTT.URIs, Serialization: n.MQTT.Serialization, Authentication: method, Username: n.MQTT.Authentication.Username}
	}
	return out
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// MarshalTextOutput implements format.TextMarshaler.
func (n *Notifiers) MarshalTextOutput() (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "webhook:    %s\n", yesNo(n.Webhook))
	_, _ = fmt.Fprintf(&b, "websocket:  %s\n", yesNo(n.WebSocket))
	if n.MQTT == nil {
		b.WriteString("mqtt:       no\n")
		return b.String(), nil
	}
	_, _ = fmt.Fprintf(&b, "mqtt:       %s\n", strings.Join(n.MQTT.URIs, ", "))
	_, _ = fmt.Fprintf(&b, "  auth:     %s\n", n.MQTT.Authentication)
	if n.MQTT.Username != "" {
		_, _ = fmt.Fprintf(&b, "  username: %s\n", n.MQTT.Username)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (n *Notifiers) TableHeaders() []string { return []string{"MECHANISM", "OFFERED", "DETAIL"} }

// TableRows implements format.TableMarshaler.
func (n *Notifiers) TableRows() [][]string {
	rows := [][]string{{"WEBHOOK", yesNo(n.Webhook), ""}, {"WEBSOCKET", yesNo(n.WebSocket), ""}}
	if n.MQTT == nil {
		return append(rows, []string{"MQTT", "no", ""})
	}
	return append(rows, []string{"MQTT", "yes", strings.Join(n.MQTT.URIs, ",") + " auth " + n.MQTT.Authentication})
}

func newNotifiersCmd(role Role) *cobra.Command {
	c := newCommon()
	cmd := &cobra.Command{
		Use:     "notifiers",
		Short:   "Show how the VTN notifies: webhooks, MQTT, WebSockets",
		Example: "  openadrctl " + string(role) + " notifiers\n  openadrctl " + string(role) + " notifiers --format json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return once(cmd, role, &c, nil, false, func(ctx context.Context, a *api) (any, error) {
				n, err := pick(a).Notifiers(ctx)
				if err != nil {
					return nil, err
				}
				return notifiersOf(n), nil
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

// topicSets are the twelve sets of objects OpenADR 3.1.0 has MQTT topics for.
var topicSets = map[string]struct {
	withID bool
	set    func(id string) client.TopicSet
	help   string
}{
	"programs":       {false, func(string) client.TopicSet { return client.AllPrograms() }, "every program"},
	"events":         {false, func(string) client.TopicSet { return client.AllEvents() }, "every event"},
	"reports":        {false, func(string) client.TopicSet { return client.AllReports() }, "every report"},
	"subscriptions":  {false, func(string) client.TopicSet { return client.AllSubscriptions() }, "every subscription"},
	"vens":           {false, func(string) client.TopicSet { return client.AllVENs() }, "every VEN"},
	"resources":      {false, func(string) client.TopicSet { return client.AllResources() }, "every resource"},
	"program":        {true, client.OneProgram, "one program"},
	"program-events": {true, client.EventsOfProgram, "the events of a program"},
	"ven":            {true, client.OneVEN, "one VEN"},
	"ven-events":     {true, client.EventsForVEN, "the events a VEN may see"},
	"ven-programs":   {true, client.ProgramsForVEN, "the programs a VEN may see"},
	"ven-resources":  {true, client.ResourcesOfVEN, "the resources of a VEN"},
}

func topicSetNames() string {
	var names []string
	for k, v := range topicSets {
		if v.withID {
			k += ":<id>"
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// parseTopicSet reads programs, ven-events:<id>, ... .
func parseTopicSet(s string) (client.TopicSet, error) {
	name, id, _ := strings.Cut(s, ":")
	t, ok := topicSets[name]
	switch {
	case !ok:
		return client.TopicSet{}, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q is not a set of topics: %s", s, topicSetNames()), nil)
	case t.withID && id == "":
		return client.TopicSet{}, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q needs an ID: %s:<id>", s, name), nil)
	case !t.withID && id != "":
		return client.TopicSet{}, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q takes no ID", name), nil)
	case id != "" && !openadr.ValidID(id):
		return client.TopicSet{}, errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q is not an object ID", id), nil)
	}
	return t.set(id), nil
}

// Topics is the result of topics.
type Topics struct {
	Set    string `json:"set"`
	Create string `json:"create,omitempty"`
	Update string `json:"update,omitempty"`
	Delete string `json:"delete,omitempty"`
	All    string `json:"all,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (t *Topics) MarshalTextOutput() (string, error) {
	var b strings.Builder
	for _, x := range [][2]string{{"create", t.Create}, {"update", t.Update}, {"delete", t.Delete}, {"all", t.All}} {
		if x[1] != "" {
			_, _ = fmt.Fprintf(&b, "%-7s %s\n", x[0]+":", x[1])
		}
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (t *Topics) TableHeaders() []string { return []string{"OPERATION", "TOPIC"} }

// TableRows implements format.TableMarshaler.
func (t *Topics) TableRows() [][]string {
	var rows [][]string
	for _, x := range [][2]string{{"CREATE", t.Create}, {"UPDATE", t.Update}, {"DELETE", t.Delete}, {"ALL", t.All}} {
		if x[1] != "" {
			rows = append(rows, []string{x[0], x[1]})
		}
	}
	return rows
}

func newTopicsCmd(role Role) *cobra.Command {
	c := newCommon()
	cmd := &cobra.Command{
		Use:   "topics <set>",
		Short: "Show the MQTT topics of a set of objects",
		Long: `Ask the VTN on which MQTT topics it publishes the operations on a set of objects. Sets:
` + topicSetNames() + `.

Business Logic gets the topics of every set; a VEN those of its own VEN object
(ven:<id>, ven-events:<id>, ven-programs:<id>, ven-resources:<id>).`,
		Example: "  openadrctl bl topics events\n  openadrctl ven topics ven-events:1f2e3d4c --format json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var set client.TopicSet
			return once(cmd, role, &c, func() error {
				var err error
				set, err = parseTopicSet(args[0])
				return err
			}, false, func(ctx context.Context, a *api) (any, error) {
				t, err := pick(a).MQTTTopics(ctx, set)
				if err != nil {
					return nil, err
				}
				return &Topics{Set: args[0], Create: t.Create, Update: t.Update, Delete: t.Delete, All: t.All}, nil
			})
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}
