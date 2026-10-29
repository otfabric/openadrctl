// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/notification/mqtt"
	"github.com/otfabric/go-openadr/notification/webhook"
	"github.com/otfabric/go-openadr/notification/websocket"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/runner"
)

type listenConfig struct {
	Common
	Via        string        `flag:"via" env:"OPENADRCTL_VIA" desc:"How to be notified: webhook, mqtt or websocket (required)"`
	Count      int           `flag:"count" desc:"Stop after this many notifications (0: no limit)"`
	Duration   time.Duration `flag:"duration" desc:"Stop after this long (0: until interrupted)"`
	ClientName string        `flag:"client-name" desc:"Client name of the subscription (default: openadrctl)"`
	Program    string        `flag:"program" desc:"Only this program, its events and the reports on them"`
	Targets    []string      `flag:"target" desc:"Targets to name in the subscription, as in a search (repeatable)"`
	Keep       bool          `flag:"keep" desc:"Leave the subscription on the VTN when the command ends"`

	// webhook
	Listen      string `flag:"listen" env:"OPENADRCTL_LISTEN" desc:"webhook: address to receive notifications on"`
	CallbackURL string `flag:"callback-url" env:"OPENADRCTL_CALLBACK_URL" desc:"webhook: URL under which the VTN reaches --listen (default: http://<listen>/)"`
	TLSCert     string `flag:"tls-cert" desc:"webhook: serve https with this certificate (PEM)"`
	TLSKey      string `flag:"tls-key" desc:"webhook: private key of --tls-cert (PEM)"`

	// mqtt
	Broker           string   `flag:"broker" env:"OPENADRCTL_MQTT_BROKER" desc:"mqtt: broker URL, mqtt:// or mqtts:// (default: the first the VTN announces)"`
	Topics           []string `flag:"topics" desc:"mqtt: sets of topics to subscribe to (repeatable), such as events or ven-events:<id>; see topics"`
	MQTTClientID     string   `flag:"mqtt-client-id" env:"OPENADRCTL_MQTT_CLIENT_ID" desc:"mqtt: client ID at the broker (default: openadrctl-<random>)"`
	MQTTUsername     string   `flag:"mqtt-username" env:"OPENADRCTL_MQTT_USERNAME" desc:"mqtt: user name (default: what the VTN announces)"`
	MQTTPassword     string   `env:"OPENADRCTL_MQTT_PASSWORD"`
	MQTTPasswordFile string   `flag:"mqtt-password-file" env:"OPENADRCTL_MQTT_PASSWORD_FILE" desc:"mqtt: file holding the password (or OPENADRCTL_MQTT_PASSWORD; with OAUTH2_BEARER_TOKEN the client's token is used)"`
	MQTTCA           string   `flag:"mqtt-ca" env:"OPENADRCTL_MQTT_CA" desc:"mqtt: CA certificates that verify an mqtts broker (PEM)"`
}

func newListenCmd(role Role) *cobra.Command {
	c := listenConfig{Common: newCommon(), Listen: "127.0.0.1:8081"}
	var sel selection
	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Subscribe to notifications and print them as they arrive",
		Long: `Receive the VTN's notifications and print one record per notification, until --count,
--duration or an interrupt. With --format json the records are JSON Lines.

--via webhook   serves a webhook on --listen, subscribes with it as the callback URL, and
                deletes the subscription at the end (unless --keep). The VTN has to reach
                --callback-url, and many accept only https on a public address: --tls-cert
                and --tls-key serve https.
--via websocket subscribes with the WebSocket mechanism and opens the VTN's WebSocket
                (an extension of OpenLEADR and go-openadr, not of OpenADR 3.1.0).
--via mqtt      reads the broker from GET /notifiers and the topics of --topics from the
                VTN, and subscribes to them at the broker. No subscription is created.

Only what happens while listening is received: notifications are not kept for a client that
is away. Progress goes to stderr. The command exits 0 when it ends by --count, --duration or
an interrupt, 4 when the WebSocket or broker cannot be reached at the start.`,
		Example: `  openadrctl ven listen --via websocket --object EVENT
  openadrctl ven listen --via webhook --listen 0.0.0.0:8443 --callback-url https://site-42.example:8443/ \
      --tls-cert site.pem --tls-key site.key --object EVENT --object PROGRAM
  openadrctl bl listen --via mqtt --topics events --topics reports --format json
  openadrctl bl listen --via websocket --object REPORT --operation CREATE --count 1 --format json`,
		Args: cobra.NoArgs,
	}
	sel.register(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runner.RunStream(cmd, c.Format, func() error { return c.validate(&sel) }, func(ctx context.Context, out *format.StreamWriter) (int, error) {
			return c.run(ctx, cmd, role, &sel, out)
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	for _, f := range []string{"tls-cert", "tls-key", "mqtt-ca", "mqtt-password-file"} {
		_ = cmd.MarkFlagFilename(f, "pem", "crt", "key", "txt")
	}
	return cmd
}

func (c *listenConfig) validate(sel *selection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Count < 0 || c.Duration < 0 {
		return errs.InvalidInput(errs.CodeInvalidInput, "--count and --duration must not be negative", nil)
	}
	if c.Program != "" && !openadr.ValidID(c.Program) {
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--program %q is not an object ID", c.Program), nil)
	}
	if _, err := sel.operation(); err != nil {
		return err
	}
	switch c.Via {
	case "webhook":
		if _, _, err := net.SplitHostPort(c.Listen); err != nil {
			return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--listen %q: host:port", c.Listen), err)
		}
		if (c.TLSCert == "") != (c.TLSKey == "") {
			return errs.InvalidInput(errs.CodeInvalidFlagCombination, "--tls-cert and --tls-key go together", nil)
		}
		if c.CallbackURL != "" {
			if u, err := url.Parse(c.CallbackURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--callback-url %q: an http or https URL", c.CallbackURL), err)
			}
		}
	case "websocket":
	case "mqtt":
		if len(c.Topics) == 0 {
			return errs.InvalidInput(errs.CodeInvalidInput, "--via mqtt needs --topics: "+topicSetNames(), nil)
		}
		for _, t := range c.Topics {
			if _, err := parseTopicSet(t); err != nil {
				return err
			}
		}
		if c.Broker != "" {
			if u, err := url.Parse(c.Broker); err != nil || (u.Scheme != "mqtt" && u.Scheme != "mqtts" && u.Scheme != "tcp" && u.Scheme != "ssl") {
				return errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--broker %q: an mqtt:// or mqtts:// URL", c.Broker), err)
			}
		}
		if c.MQTTPassword != "" && c.MQTTPasswordFile != "" {
			return errs.InvalidInput(errs.CodeInvalidFlagCombination, "give the MQTT password in OPENADRCTL_MQTT_PASSWORD or --mqtt-password-file, not both", nil)
		}
	default:
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--via %q: give webhook, mqtt or websocket", c.Via), nil)
	}
	return nil
}

// Notification is a record of listen and of vtn start: a notification as
// it arrived.
type Notification struct {
	Time       time.Time       `json:"time"`
	Via        string          `json:"via"`
	Topic      string          `json:"topic,omitempty"`
	ObjectType string          `json:"objectType"`
	Operation  string          `json:"operation"`
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Object     json.RawMessage `json:"object,omitempty"`
}

func notificationOf(via, topic string, n openadr.Notification) *Notification {
	r := &Notification{Time: time.Now().UTC(), Via: via, Topic: topic, ObjectType: string(n.ObjectType), Operation: string(n.Operation)}
	if n.Object != nil {
		r.ID = oadr.MetadataOf(n.Object).ID
		r.Object = oadr.MustEncode(n.Object)
		if k := KindOf(n.Object); k != nil {
			if row := k.Row(n.Object); len(row) > 1 && row[1] != "-" {
				r.Name = row[1]
			}
		}
	}
	return r
}

// KindOf returns how an object is shown, nil for a value that is no object.
func KindOf(v any) *oadr.Kind {
	switch v.(type) {
	case openadr.Program:
		return oadr.Programs
	case openadr.Event:
		return oadr.Events
	case openadr.Report:
		return oadr.Reports
	case openadr.VEN:
		return oadr.VENs
	case openadr.Resource:
		return oadr.Resources
	case openadr.Subscription:
		return oadr.Subscriptions
	case openadr.ResourceGroup:
		return oadr.ResourceGroups
	}
	return nil
}

// MarshalTextOutput implements format.TextMarshaler.
func (n *Notification) MarshalTextOutput() (string, error) {
	s := fmt.Sprintf("%s  %-9s %-12s %-6s %s", n.Time.Format(time.RFC3339), n.Via, n.ObjectType, n.Operation, n.ID)
	if n.Name != "" {
		s += " (" + n.Name + ")"
	}
	return s, nil
}

// TableHeaders implements format.TableMarshaler.
func (n *Notification) TableHeaders() []string {
	return []string{"TIME", "VIA", "OBJECT", "OPERATION", "ID", "NAME"}
}

// TableRows implements format.TableMarshaler.
func (n *Notification) TableRows() [][]string {
	return [][]string{{n.Time.Format(time.RFC3339), n.Via, n.ObjectType, n.Operation, n.ID, n.Name}}
}

// sink writes the records of a stream, and ends it after count of them.
type sink struct {
	mu    sync.Mutex
	out   *format.StreamWriter
	count int
	seen  int
	err   error
	// failure ends the stream with an error of the tool.
	failure error
	done    chan struct{}
}

func newSink(out *format.StreamWriter, count int) *sink {
	return &sink{out: out, count: count, done: make(chan struct{})}
}

func (s *sink) write(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil || (s.count > 0 && s.seen >= s.count) {
		return
	}
	if s.err = s.out.Write(v); s.err != nil {
		close(s.done)
		return
	}
	s.seen++
	if s.count > 0 && s.seen == s.count {
		close(s.done)
	}
}

// fail ends the stream with err.
func (s *sink) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil && s.err == nil && (s.count <= 0 || s.seen < s.count) {
		s.failure = err
		close(s.done)
	}
}

// wait blocks until the stream ends, and returns the exit code.
func (s *sink) wait(ctx context.Context) (int, error) {
	select {
	case <-ctx.Done():
	case <-s.done:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, errs.Output(errs.CodeOutputWriteFailed, s.err)
	}
	if s.failure != nil {
		return 0, s.failure
	}
	return errs.ExitOK, nil
}

func (c *listenConfig) run(ctx context.Context, cmd *cobra.Command, role Role, sel *selection, out *format.StreamWriter) (int, error) {
	a, err := c.connect(role)
	if err != nil {
		return 0, err
	}
	if c.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Duration)
		defer cancel()
	}
	progress := cmd.ErrOrStderr()
	s := newSink(out, c.Count)
	switch c.Via {
	case "mqtt":
		return c.runMQTT(ctx, a, s, progress)
	case "webhook":
		return c.runWebhook(ctx, a, sel, s, progress)
	default:
		return c.runWebSocket(ctx, a, sel, s, progress)
	}
}

// subscribe creates the subscription of listen, and returns how to delete
// it.
func (c *listenConfig) subscribe(ctx context.Context, a *api, sel *selection, oo openadr.ObjectOperation, progress io.Writer) (func(), error) {
	name := c.ClientName
	if name == "" {
		name = "openadrctl"
	}
	sub := openadr.Subscription{ClientName: name, ProgramID: c.Program, Targets: c.Targets, ObjectOperations: []openadr.ObjectOperation{oo}}
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	var created openadr.Subscription
	var err error
	if a.role == VEN {
		created, err = a.ven.CreateSubscription(cctx, sub)
	} else {
		created, err = a.bl.CreateSubscription(cctx, sub)
	}
	if err != nil {
		return nil, conn.Classify(err, true)
	}
	_, _ = fmt.Fprintf(progress, "subscribed: subscription %s, %s of %s\n", created.ID, joinOps(oo.Operations), joinObjects(oo.Objects))
	return func() {
		if c.Keep {
			_, _ = fmt.Fprintf(progress, "kept: subscription %s\n", created.ID)
			return
		}
		dctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
		defer cancel()
		if a.role == VEN {
			_, err = a.ven.DeleteSubscription(dctx, created.ID)
		} else {
			_, err = a.bl.DeleteSubscription(dctx, created.ID)
		}
		if err != nil {
			_, _ = fmt.Fprintf(progress, "warning: subscription %s was not deleted: %v\n", created.ID, err)
			return
		}
		_, _ = fmt.Fprintf(progress, "unsubscribed: subscription %s\n", created.ID)
	}, nil
}

func joinOps(ops []openadr.Operation) string {
	s := make([]string, len(ops))
	for i, o := range ops {
		s[i] = string(o)
	}
	return strings.Join(s, ",")
}

func joinObjects(objs []openadr.ObjectType) string {
	s := make([]string, len(objs))
	for i, o := range objs {
		s[i] = string(o)
	}
	return strings.Join(s, ",")
}

func (c *listenConfig) runWebSocket(ctx context.Context, a *api, sel *selection, s *sink, progress io.Writer) (int, error) {
	oo, _ := sel.operation()
	oo.Mechanism = openadr.MechanismWebSocket
	unsubscribe, err := c.subscribe(ctx, a, sel, oo, progress)
	if err != nil {
		return 0, err
	}
	defer unsubscribe()
	sub, err := websocket.NewSubscriber(websocket.SubscriberConfig{
		Dial:   pick(a).NotificationStream,
		Notify: func(_ context.Context, n openadr.Notification) { s.write(notificationOf("websocket", "", n)) },
	})
	if err != nil {
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	defer func() { _ = sub.Close() }()
	rctx, cancel := context.WithTimeout(ctx, c.Timeout)
	err = sub.Ready(rctx)
	cancel()
	if err != nil {
		if interrupted(ctx) {
			return errs.ExitOK, nil
		}
		if ctx.Err() != nil {
			return 0, errs.New(errs.KindTransport, errs.CodeConnectionLost, "the WebSocket of the VTN did not open before --duration ended", err)
		}
		// The first attempt tells why: dial once more for the error.
		dctx, dcancel := context.WithTimeout(ctx, c.Timeout)
		defer dcancel()
		if st, derr := pick(a).NotificationStream(dctx); derr != nil {
			return 0, conn.Classify(derr, false)
		} else {
			_ = st.Close()
		}
		return 0, errs.New(errs.KindTransport, errs.CodeConnectionLost, "the WebSocket of the VTN did not open: "+err.Error(), err)
	}
	_, _ = fmt.Fprintln(progress, "listening: WebSocket open")
	return s.wait(ctx)
}

func (c *listenConfig) runWebhook(ctx context.Context, a *api, sel *selection, s *sink, progress io.Writer) (int, error) {
	l, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return 0, errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("listen on %s: %v", c.Listen, err), err)
	}
	scheme := "http"
	if c.TLSCert != "" {
		cert, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey)
		if err != nil {
			_ = l.Close()
			return 0, errs.InvalidInput(errs.CodeInvalidCertificate, fmt.Sprintf("--tls-cert %s and --tls-key %s: %v", c.TLSCert, c.TLSKey, err), err)
		}
		l = tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		scheme = "https"
	}
	callback := c.CallbackURL
	if callback == "" {
		callback = scheme + "://" + l.Addr().String() + "/"
	}
	token := randomToken()
	h := &webhook.Handler{BearerToken: token, Notify: func(_ context.Context, n openadr.Notification) error {
		s.write(notificationOf("webhook", "", n))
		return nil
	}}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { _ = srv.Serve(l) }()
	defer func() { _ = srv.Close() }()
	_, _ = fmt.Fprintf(progress, "listening: webhook on %s, callback URL %s\n", l.Addr(), callback)
	oo, _ := sel.operation()
	oo.CallbackURL, oo.BearerToken = callback, token
	unsubscribe, err := c.subscribe(ctx, a, sel, oo, progress)
	if err != nil {
		return 0, err
	}
	defer unsubscribe()
	return s.wait(ctx)
}

func (c *listenConfig) runMQTT(ctx context.Context, a *api, s *sink, progress io.Writer) (int, error) {
	cctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	n, err := pick(a).Notifiers(cctx)
	if err != nil {
		return 0, conn.Classify(err, false)
	}
	broker := mqtt.Broker{URL: c.Broker, ClientID: c.MQTTClientID, Username: c.MQTTUsername}
	if broker.URL == "" {
		if n.MQTT == nil || len(n.MQTT.URIs) == 0 {
			return 0, errs.New(errs.KindNegative, errs.CodeNotImplemented, "the VTN announces no MQTT broker in GET /notifiers; give --broker", nil)
		}
		broker.URL = n.MQTT.URIs[0]
	}
	broker.URL = strings.Replace(strings.Replace(broker.URL, "tcp://", "mqtt://", 1), "ssl://", "mqtts://", 1)
	if broker.ClientID == "" {
		broker.ClientID = "openadrctl-" + randomToken()[:8]
	}
	method := openadr.MQTTAnonymous
	if n.MQTT != nil && n.MQTT.Authentication.Method != "" {
		method = mqttMethod(n.MQTT.Authentication.Method)
	}
	switch {
	case c.MQTTPasswordFile != "":
		data, err := os.ReadFile(c.MQTTPasswordFile)
		if err != nil {
			return 0, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("--mqtt-password-file %s: %v", c.MQTTPasswordFile, err), err)
		}
		broker.Password = strings.TrimSpace(string(data))
	case c.MQTTPassword != "":
		broker.Password = c.MQTTPassword
	case method == openadr.MQTTBearerToken:
		switch {
		case a.conn.Tokens != nil:
			t, err := a.conn.Tokens.Token(cctx)
			if err != nil {
				return 0, conn.Classify(err, false)
			}
			broker.Password = t
		case a.conn.Token != "":
			broker.Password = a.conn.Token
		}
	}
	if broker.Username == "" && n.MQTT != nil && method == openadr.MQTTBearerToken {
		broker.Username = strings.ReplaceAll(n.MQTT.Authentication.Username, "{clientID}", c.ClientID)
	}
	if strings.HasPrefix(broker.URL, "mqtts://") {
		tc := &tls.Config{MinVersion: tls.VersionTLS12}
		if c.MQTTCA != "" {
			pool, err := conn.LoadPool(c.MQTTCA)
			if err != nil {
				return 0, err
			}
			tc.RootCAs = pool
		}
		broker.TLS = tc
	}
	var filters []string
	for _, name := range c.Topics {
		set, _ := parseTopicSet(name)
		t, err := pick(a).MQTTTopics(cctx, set)
		if err != nil {
			return 0, conn.Classify(err, false)
		}
		if t.All != "" {
			filters = append(filters, t.All)
			continue
		}
		for _, f := range []string{t.Create, t.Update, t.Delete} {
			if f != "" {
				filters = append(filters, f)
			}
		}
	}
	sub, err := mqtt.NewSubscriber(mqtt.SubscriberConfig{Broker: broker, Topics: filters,
		Notify: func(_ context.Context, topic string, n openadr.Notification) {
			s.write(notificationOf("mqtt", topic, n))
		}})
	if err != nil {
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	defer func() { _ = sub.Close() }()
	rctx, rcancel := context.WithTimeout(ctx, c.Timeout)
	defer rcancel()
	if err := sub.Ready(rctx); err != nil {
		if interrupted(ctx) {
			return errs.ExitOK, nil
		}
		return 0, errs.New(errs.KindTransport, errs.CodeTransportConnectFailed, fmt.Sprintf("the broker %s: not subscribed (a login the broker refuses? see --mqtt-username and --mqtt-password-file): %v", broker.URL, err), err)
	}
	_, _ = fmt.Fprintf(progress, "listening: %s, %s\n", broker.URL, strings.Join(filters, " "))
	return s.wait(ctx)
}

// mqttMethod reads the authentication method a VTN announces for its
// broker, also as OpenLEADR writes it (OAUTH2BEARERTOKEN).
func mqttMethod(m string) string {
	switch strings.ReplaceAll(strings.ToUpper(m), "_", "") {
	case "OAUTH2BEARERTOKEN":
		return openadr.MQTTBearerToken
	case "ANONYMOUS":
		return openadr.MQTTAnonymous
	case "CERTIFICATE":
		return openadr.MQTTCertificate
	}
	return m
}

// interrupted tells whether the command was interrupted, as opposed to
// reaching --duration.
func interrupted(ctx context.Context) bool { return errors.Is(ctx.Err(), context.Canceled) }

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(errors.New("openadrctl: no randomness"))
	}
	return hex.EncodeToString(b)
}
