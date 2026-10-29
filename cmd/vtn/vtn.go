// SPDX-License-Identifier: MIT

// Package vtn is the server side of openadrctl: a VTN seeded from a file,
// with its own token endpoint.
package vtn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/auth"
	"github.com/otfabric/go-openadr/discovery"
	"github.com/otfabric/go-openadr/notification/mqtt"
	"github.com/otfabric/go-openadr/notification/webhook"
	"github.com/otfabric/go-openadr/server"
	"github.com/otfabric/go-openadr/server/storage"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
	"github.com/otfabric/openadrctl/internal/runner"
	"github.com/otfabric/openadrctl/internal/seed"
)

// NewCmd returns the vtn command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vtn",
		Short: "Serve OpenADR 3.1.0 as a VTN",
		Long: `"vtn start" serves the OpenADR 3.1.0 API of a VTN, with an OAuth2 token endpoint of its
own, seeded from a file: the clients that may authenticate, and the programs, VENs,
resources and events it starts with (vtn generate-config writes an example). Business
Logic and VENs then talk to it with "openadrctl bl" and "openadrctl ven", or any other
OpenADR 3 client.

It is a VTN for development, tests and demonstrations: tokens are kept in memory, and the
objects too unless --store names a file.`,
	}
	cmd.AddCommand(newStartCmd(), newGenerateConfigCmd())
	return cmd
}

type startConfig struct {
	Listen           string        `flag:"listen" env:"OPENADRCTL_VTN_LISTEN" desc:"Address to listen on (port 0: any free port)"`
	PublicURL        string        `flag:"public-url" env:"OPENADRCTL_VTN_PUBLIC_URL" desc:"URL under which clients reach the API (default: http(s)://<listen>)"`
	Config           string        `flag:"config" env:"OPENADRCTL_VTN_CONFIG" desc:"Seed file (YAML or JSON): clients and objects; see vtn generate-config (default: the example seed)"`
	Store            string        `flag:"store" env:"OPENADRCTL_VTN_STORE" desc:"File to keep the objects in across restarts (default: in memory)"`
	TLSCert          string        `flag:"tls-cert" env:"OPENADRCTL_VTN_TLS_CERT" desc:"Serve https with this certificate (PEM)"`
	TLSKey           string        `flag:"tls-key" env:"OPENADRCTL_VTN_TLS_KEY" desc:"Private key of --tls-cert (PEM)"`
	Webhooks         bool          `flag:"webhooks" desc:"Serve subscriptions with callback URLs, and notify them (https, public addresses)"`
	InsecureWebhooks bool          `flag:"insecure-webhooks" desc:"As --webhooks, and allow http and private addresses as callback URLs (local tests)"`
	WebSocket        bool          `flag:"websocket" desc:"Notify over WebSockets (extension, not OpenADR 3.1.0)"`
	MQTT             string        `flag:"mqtt" env:"OPENADRCTL_VTN_MQTT" desc:"MQTT broker to publish notifications to, such as mqtt://127.0.0.1:1883"`
	MQTTUsername     string        `flag:"mqtt-username" env:"OPENADRCTL_VTN_MQTT_USERNAME" desc:"User name at the broker"`
	MQTTPassword     string        `env:"OPENADRCTL_VTN_MQTT_PASSWORD"`
	MQTTPublicURL    string        `flag:"mqtt-public-url" env:"OPENADRCTL_VTN_MQTT_PUBLIC_URL" desc:"URL of the broker as clients reach it (default: --mqtt)"`
	ResourceGroups   bool          `flag:"resource-groups" desc:"Serve resource groups (extension, not OpenADR 3.1.0)"`
	Anonymous        bool          `flag:"anonymous" desc:"Answer requests without a token, for programs and events without targets"`
	NotifyReads      bool          `flag:"notify-reads" desc:"Notify subscribers of READ when an object is read"`
	Rate             float64       `flag:"rate" desc:"Requests a second taken from each client, twice as many at once; the rest are answered 429 (0: no limit)"`
	TokenTTL         time.Duration `flag:"token-ttl" desc:"Lifetime of the tokens the VTN issues"`
	Advertise        string        `flag:"advertise" desc:"Announce the VTN on the local network with DNS-SD under this instance name"`
	Duration         time.Duration `flag:"duration" desc:"Stop after this long (0: until interrupted)"`
	LogRequests      bool          `flag:"log-requests" desc:"Write a record for every request"`
	Format           string        `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

func newStartCmd() *cobra.Command {
	c := startConfig{Listen: "127.0.0.1:8080", TokenTTL: time.Hour, Format: "text"}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Serve the OpenADR 3.1.0 API, seeded from a file",
		Long: `Serve a VTN on --listen until interrupted or --duration: the OpenADR 3.1.0 API at the root
of --public-url, its token endpoint at /auth/token (announced at GET /auth/server), and the
objects and clients of --config. Without --config it serves the example seed of
generate-config: clients utility (bl) and site-42 (ven), with the secrets written there.

Notifications: --webhooks (or --insecure-webhooks for callbacks on this machine), --mqtt
(a broker of your own: the VTN publishes, the broker is not part of openadrctl), and
--websocket (an extension). With --store and webhooks, notifications that are not yet
delivered survive a restart.

Records go to stdout, one per thing that happens: listening, client, seeded, then request
(with --log-requests), notification, socket_opened, socket_closed and log, and stopped at
the end. With --format json they are JSON Lines. The seed is checked before anything
listens; every problem is reported, and the command exits 3.

It serves plain http unless --tls-cert and --tls-key are given.`,
		Example: `  openadrctl vtn start
  openadrctl vtn start --config seed.yaml --store vtn.db --insecure-webhooks --websocket
  openadrctl vtn start --listen 0.0.0.0:8443 --public-url https://vtn.example:8443 \
      --tls-cert vtn.pem --tls-key vtn.key --config seed.yaml --format json
  openadrctl vtn start --mqtt mqtt://127.0.0.1:1883 --anonymous --advertise "Test VTN"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runner.RunStream(cmd, c.Format, c.validate, c.run)
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	for _, f := range []string{"tls-cert", "tls-key"} {
		_ = cmd.MarkFlagFilename(f, "pem", "crt", "key")
	}
	_ = cmd.MarkFlagFilename("config", "yaml", "yml", "json")
	return cmd
}

func (c *startConfig) validate() error {
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--listen %q: host:port", c.Listen), err)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, "--tls-cert and --tls-key go together", nil)
	}
	if c.PublicURL != "" {
		if u, err := url.Parse(c.PublicURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--public-url %q: an http or https URL", c.PublicURL), err)
		}
	}
	if c.MQTT != "" {
		if u, err := url.Parse(c.MQTT); err != nil || (u.Scheme != "mqtt" && u.Scheme != "mqtts") || u.Host == "" {
			return errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--mqtt %q: an mqtt:// or mqtts:// URL", c.MQTT), err)
		}
	}
	if c.Rate < 0 || c.TokenTTL <= 0 || c.Duration < 0 {
		return errs.InvalidInput(errs.CodeInvalidInput, "--rate and --duration must not be negative, and --token-ttl must be positive", nil)
	}
	return nil
}

// Record is a record of vtn start.
type Record struct {
	Time      time.Time `json:"time"`
	Event     string    `json:"event"`
	Address   string    `json:"address,omitempty"`
	URL       string    `json:"url,omitempty"`
	Client    string    `json:"client,omitempty"`
	Role      string    `json:"role,omitempty"`
	Op        string    `json:"op,omitempty"`
	Status    int       `json:"status,omitempty"`
	Millis    *int64    `json:"ms,omitempty"`
	Mechanism string    `json:"mechanism,omitempty"`
	Object    string    `json:"objectType,omitempty"`
	Operation string    `json:"operation,omitempty"`
	Count     *int      `json:"count,omitempty"`
	Level     string    `json:"level,omitempty"`
	Message   string    `json:"message,omitempty"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (r *Record) MarshalTextOutput() (string, error) {
	return strings.TrimRight(fmt.Sprintf("%s  %-14s %s", r.Time.Format(time.RFC3339), r.Event, r.detail()), " "), nil
}

func (r *Record) detail() string {
	switch r.Event {
	case "listening":
		return r.URL + " on " + r.Address
	case "client":
		return r.Client + " (" + r.Role + ")"
	case "request":
		return fmt.Sprintf("%s %d %dms", r.Op, r.Status, *r.Millis)
	case "notification":
		return r.Mechanism + " " + r.Object + " " + r.Operation
	case "seeded":
		return fmt.Sprintf("%d objects", *r.Count)
	case "log":
		return r.Level + " " + r.Message
	}
	return r.Message
}

// TableHeaders implements format.TableMarshaler.
func (r *Record) TableHeaders() []string { return []string{"TIME", "EVENT", "DETAIL"} }

// TableRows implements format.TableMarshaler.
func (r *Record) TableRows() [][]string {
	return [][]string{{r.Time.Format(time.RFC3339), r.Event, r.detail()}}
}

// recorder writes records, one at a time, and keeps the first failure.
type recorder struct {
	mu          sync.Mutex
	out         *format.StreamWriter
	err         error
	logRequests bool
}

func (r *recorder) emit(rec *Record) {
	rec.Time = time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = r.out.Write(rec)
	}
}

// The recorder is the metrics of the server.
func (r *recorder) OnRequest(string) {}

func (r *recorder) OnResponse(op string, status int, d time.Duration) {
	if r.logRequests {
		ms := d.Milliseconds()
		r.emit(&Record{Event: "request", Op: op, Status: status, Millis: &ms})
	}
}

func (r *recorder) OnNotification(mechanism string, object openadr.ObjectType, operation openadr.Operation) {
	r.emit(&Record{Event: "notification", Mechanism: mechanism, Object: string(object), Operation: string(operation)})
}

func (r *recorder) OnSocketOpened() { r.emit(&Record{Event: "socket_opened"}) }

func (r *recorder) OnSocketClosed(reason string) {
	r.emit(&Record{Event: "socket_closed", Message: reason})
}

// logHandler turns the warnings and errors of the server into records.
type logHandler struct {
	r     *recorder
	attrs []slog.Attr
}

func (h logHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelWarn }

func (h logHandler) Handle(_ context.Context, rec slog.Record) error {
	var b strings.Builder
	b.WriteString(rec.Message)
	add := func(a slog.Attr) bool {
		_, _ = fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	}
	for _, a := range h.attrs {
		add(a)
	}
	rec.Attrs(add)
	h.r.emit(&Record{Event: "log", Level: strings.ToLower(rec.Level.String()), Message: b.String()})
	return nil
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{r: h.r, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...)}
}

func (h logHandler) WithGroup(string) slog.Handler { return h }

func (c *startConfig) run(ctx context.Context, out *format.StreamWriter) (int, error) {
	var sd *seed.Seed
	var err error
	if c.Config != "" {
		sd, err = seed.Load(c.Config)
	} else {
		sd, err = seed.Parse([]byte(seed.Example))
	}
	if err != nil {
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, "the seed: "+oneLine(err), err)
	}
	if len(sd.Clients) == 0 && !c.Anonymous {
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, "the seed has no clients and --anonymous is not given: nobody could use the VTN", nil)
	}
	var cert tls.Certificate
	if c.TLSCert != "" {
		if cert, err = tls.LoadX509KeyPair(c.TLSCert, c.TLSKey); err != nil {
			return 0, errs.InvalidInput(errs.CodeInvalidCertificate, fmt.Sprintf("--tls-cert %s and --tls-key %s: %v", c.TLSCert, c.TLSKey, err), err)
		}
	}
	if c.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Duration)
		defer cancel()
	}
	rec := &recorder{out: out, logRequests: c.LogRequests}
	logger := slog.New(logHandler{r: rec})

	var store storage.Store = storage.NewMemory()
	var outbox storage.Outbox
	if c.Store != "" {
		file, err := storage.OpenFile(c.Store, storage.FileOptions{})
		if err != nil {
			return 0, errs.InvalidInput(errs.CodeInvalidConfig, fmt.Sprintf("--store %s: %v", c.Store, err), err)
		}
		defer func() { _ = file.Close() }()
		store, outbox = file, file
	}
	n, err := sd.Apply(ctx, store)
	if err != nil {
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, "the seed: "+oneLine(err), err)
	}

	l, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return 0, errs.New(errs.KindTransport, errs.CodeListenFailed, fmt.Sprintf("listen on %s: %v", c.Listen, err), err)
	}
	scheme := "http"
	if c.TLSCert != "" {
		scheme = "https"
	}
	public := strings.TrimSuffix(c.PublicURL, "/")
	if public == "" {
		public = scheme + "://" + l.Addr().String()
	}

	issuer, err := auth.NewIssuer(auth.IssuerConfig{Clients: sd.Clients, TokenTTL: c.TokenTTL})
	if err != nil {
		_ = l.Close()
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	cfg := server.Config{
		Store: store, Logger: logger, Metrics: rec,
		ResourceGroups: c.ResourceGroups, AllowAnonymous: c.Anonymous, NotifyReads: c.NotifyReads,
	}
	if len(sd.Clients) > 0 {
		cfg.Validator, cfg.TokenHandler, cfg.TokenURL = issuer, issuer, public+"/auth/token"
	}
	if c.Rate > 0 {
		cfg.Limiter = server.NewRateLimiter(c.Rate, int(2*c.Rate)+1)
	}
	if c.WebSocket {
		cfg.WebSocket = &server.WebSocketConfig{}
	}
	var sender *webhook.Sender
	if c.Webhooks || c.InsecureWebhooks {
		sender = webhook.NewSender(webhook.SenderConfig{AllowInsecure: c.InsecureWebhooks, Logger: logger})
		cfg.Webhooks = sender
		if outbox != nil {
			cfg.Outbox = outbox
		}
	}
	var publisher *mqtt.Publisher
	if c.MQTT != "" {
		publisher, err = mqtt.NewPublisher(mqtt.PublisherConfig{Logger: logger, Broker: mqtt.Broker{
			URL: c.MQTT, ClientID: "openadrctl-vtn-" + seed.NewID()[:8], Username: c.MQTTUsername, Password: c.MQTTPassword,
		}})
		if err != nil {
			_ = l.Close()
			return 0, errs.InvalidInput(errs.CodeInvalidConfig, "--mqtt: "+err.Error(), err)
		}
		uri := c.MQTTPublicURL
		if uri == "" {
			uri = c.MQTT
		}
		cfg.MQTT = &server.MQTTConfig{Publisher: publisher, URIs: []string{uri}}
	}
	vtn, err := server.New(cfg)
	if err != nil {
		_ = l.Close()
		return 0, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	hs := &http.Server{Handler: vtn, ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	if c.TLSCert != "" {
		l = tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(l) }()

	rec.emit(&Record{Event: "listening", Address: l.Addr().String(), URL: public})
	for _, cl := range sd.Clients {
		role := "ven"
		if (auth.Identity{Scopes: cl.Scopes}).IsBusinessLogic() {
			role = "bl"
		}
		rec.emit(&Record{Event: "client", Client: cl.ID, Role: role})
	}
	rec.emit(&Record{Event: "seeded", Count: &n})

	if c.Advertise != "" {
		ad, err := advertise(c.Advertise, public, c.Anonymous && len(sd.Clients) == 0, logger)
		if err != nil {
			rec.emit(&Record{Event: "log", Level: "warn", Message: "DNS-SD advertisement: " + err.Error()})
		} else {
			defer func() { _ = ad.Close() }()
			rec.emit(&Record{Event: "advertised", Message: discovery.Service + " " + c.Advertise})
		}
	}

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-served:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// The WebSockets and the outbox first: they are not the HTTP server's.
	_ = vtn.Close()
	_ = hs.Shutdown(shutdown)
	if sender != nil {
		_ = sender.Close(shutdown)
	}
	if publisher != nil {
		_ = publisher.Close(shutdown)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return 0, errs.New(errs.KindTransport, errs.CodeListenFailed, "serving: "+serveErr.Error(), serveErr)
	}
	rec.emit(&Record{Event: "stopped"})
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.err != nil {
		return 0, errs.Output(errs.CodeOutputWriteFailed, rec.err)
	}
	return errs.ExitOK, nil
}

// advertise announces the VTN with DNS-SD at the address clients reach it
// under.
func advertise(instance, public string, noAuth bool, logger *slog.Logger) (*discovery.Advertiser, error) {
	u, err := url.Parse(public)
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("the public URL %s has no port to announce", public)
	}
	ad := discovery.Advertisement{Instance: instance, Port: n, BasePath: u.Path, NoAuth: noAuth, Logger: logger}
	if net.ParseIP(u.Hostname()) == nil {
		ad.LocalURL = public
	} else if u.Scheme != "https" {
		host, _ := os.Hostname()
		host, _, _ = strings.Cut(host, ".")
		ad.LocalURL = "http://" + net.JoinHostPort(host+".local", port) + u.Path
	}
	return discovery.Advertise(ad)
}

type generateConfig struct {
	Output string `flag:"output" desc:"Write to this file instead of stdout"`
	Force  bool   `flag:"force" desc:"Overwrite --output"`
}

func newGenerateConfigCmd() *cobra.Command {
	c := generateConfig{}
	cmd := &cobra.Command{
		Use:     "generate-config",
		Short:   "Write an example seed file, commented",
		Long:    "Write the example seed of vtn start, with comments: two clients, two programs, a VEN, a resource, an event and a resource group. docs/server-config.md describes every field.",
		Example: `  openadrctl vtn generate-config --output seed.yaml`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if c.Output == "" {
				_, err := io.WriteString(cmd.OutOrStdout(), seed.Example)
				return err
			}
			if _, err := os.Stat(c.Output); err == nil && !c.Force {
				return errs.InvalidInput(errs.CodeOutputFileCreateFailed, c.Output+" exists; use --force to overwrite", nil)
			}
			if err := os.WriteFile(c.Output, []byte(seed.Example), 0o600); err != nil {
				return errs.Output(errs.CodeOutputFileWriteFailed, err)
			}
			_, err := fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s\n", c.Output)
			return err
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

// oneLine puts the problems of a seed on one line.
func oneLine(err error) string {
	var parts []string
	for _, l := range strings.Split(err.Error(), "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" && !strings.HasSuffix(l, "unmarshal errors:") {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, "; ")
}
