// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/runner"
	"github.com/otfabric/openadrctl/internal/types"
)

type connConfig struct {
	conn.Config
	Format string `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

func newConnConfig() connConfig { return connConfig{Config: conn.Defaults(), Format: "text"} }

// Stage is one step of ping.
type Stage struct {
	Stage   string           `json:"stage"`
	Status  string           `json:"status"`
	Millis  int64            `json:"ms"`
	Detail  string           `json:"detail,omitempty"`
	Error   *types.ErrorInfo `json:"error,omitempty"`
	failure error
}

// Ping is the result of ping.
type Ping struct {
	URL    string  `json:"url"`
	OK     bool    `json:"ok"`
	Stages []Stage `json:"stages"`
}

// MarshalTextOutput implements format.TextMarshaler.
func (p *Ping) MarshalTextOutput() (string, error) {
	var b strings.Builder
	for _, s := range p.Stages {
		line := fmt.Sprintf("%-10s %-8s %5dms", s.Stage, s.Status, s.Millis)
		if s.Detail != "" {
			line += "  " + s.Detail
		}
		if s.Error != nil {
			line += "  " + s.Error.Message
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	if p.OK {
		_, _ = fmt.Fprintf(&b, "%s answers", p.URL)
	} else {
		_, _ = fmt.Fprintf(&b, "%s does not answer as it should", p.URL)
	}
	return b.String(), nil
}

// TableHeaders implements format.TableMarshaler.
func (p *Ping) TableHeaders() []string { return []string{"STAGE", "STATUS", "MS", "DETAIL"} }

// TableRows implements format.TableMarshaler.
func (p *Ping) TableRows() [][]string {
	rows := make([][]string, 0, len(p.Stages))
	for _, s := range p.Stages {
		d := s.Detail
		if s.Error != nil {
			d = s.Error.Message
		}
		rows = append(rows, []string{s.Stage, s.Status, fmt.Sprint(s.Millis), d})
	}
	return rows
}

func newPingCmd() *cobra.Command {
	c := newConnConfig()
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Check a VTN step by step: connect, TLS, token, API, notifiers",
		Long: `Check that a VTN can be used, one step at a time, and how long each step takes:

  connect    a TCP connection to the host of --url
  tls        the TLS handshake (https only), verified against --ca or the system roots
  token      a token from the token endpoint (with client credentials)
  api        GET /programs, one object at most
  notifiers  GET /notifiers: how the VTN notifies (501 is "none", not a failure)

A step that fails ends the check; the steps after it are "skipped". The result is printed
either way, and the exit code is that of the failure (docs/exitcodes.md): 4 when the VTN
cannot be reached, 6 when it refuses, 5 when it does not answer in time.`,
		Example: `  openadrctl ping --url http://127.0.0.1:8080
  OPENADRCTL_CLIENT_SECRET=site-42-secret openadrctl ping --url https://vtn.example/openadr3 --client-id site-42
  openadrctl ping --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runner.RunFormattedChecked(cmd, c.Format, c.Validate, func(ctx context.Context) (any, error) {
				return ping(ctx, c.Config)
			}, runner.WithSuccessExit(func(v any) int {
				for _, s := range v.(*Ping).Stages {
					if s.failure != nil {
						e, _ := errs.As(s.failure)
						return errs.ExitCodeForKind(e.Kind)
					}
				}
				return errs.ExitOK
			}))
		},
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

func ping(ctx context.Context, c conn.Config) (*Ping, error) {
	cn, err := c.Open()
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(cn.Base)
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), map[string]string{"http": "80", "https": "443"}[u.Scheme])
	}
	ven, err := cn.VEN()
	if err != nil {
		return nil, err
	}
	p := &Ping{URL: cn.Base}
	failed := false
	step := func(name string, run func(ctx context.Context) (string, error)) {
		if failed {
			p.Stages = append(p.Stages, Stage{Stage: name, Status: "skipped"})
			return
		}
		sctx, cancel := context.WithTimeout(ctx, c.Timeout)
		defer cancel()
		start := time.Now()
		detail, err := run(sctx)
		s := Stage{Stage: name, Status: "ok", Millis: time.Since(start).Milliseconds(), Detail: detail}
		switch {
		case errors.Is(err, errSkipped):
			s.Status = "skipped"
		case err != nil:
			failed = true
			s.Status, s.Detail = "failed", ""
			s.failure = conn.Classify(err, false)
			e, _ := errs.As(s.failure)
			s.Error = &types.ErrorInfo{Kind: string(e.Kind), Code: e.Code, Message: e.Message}
		}
		p.Stages = append(p.Stages, s)
	}
	step("connect", func(ctx context.Context) (string, error) {
		var d net.Dialer
		nc, err := d.DialContext(ctx, "tcp", host)
		if err != nil {
			return "", err
		}
		defer func() { _ = nc.Close() }()
		return nc.RemoteAddr().String(), nil
	})
	step("tls", func(ctx context.Context) (string, error) {
		if u.Scheme != "https" {
			return "plain http", errSkipped
		}
		d := tls.Dialer{Config: cn.TLS.Clone()}
		d.Config.ServerName = u.Hostname()
		nc, err := d.DialContext(ctx, "tcp", host)
		if err != nil {
			return "", err
		}
		defer func() { _ = nc.Close() }()
		st := nc.(*tls.Conn).ConnectionState()
		detail := tls.VersionName(st.Version)
		if len(st.PeerCertificates) > 0 {
			detail += ", " + st.PeerCertificates[0].Subject.CommonName
		}
		return detail, nil
	})
	step("token", func(ctx context.Context) (string, error) {
		switch {
		case cn.Tokens != nil:
			t, err := cn.Tokens.Fetch(ctx)
			if err != nil {
				return "", err
			}
			detail := "from " + t.TokenURL
			if t.ExpiresIn > 0 {
				detail += fmt.Sprintf(", valid %s", time.Duration(t.ExpiresIn)*time.Second)
			}
			return detail, nil
		case cn.Token != "":
			return "bearer token given", errSkipped
		}
		return "anonymous", errSkipped
	})
	step("api", func(ctx context.Context) (string, error) {
		ps, err := ven.Programs(ctx, client.ProgramQuery{Page: client.Page{Limit: 1}})
		if err != nil {
			return "", err
		}
		if len(ps) == 0 {
			return "GET /programs: no program visible", nil
		}
		return "GET /programs: " + ps[0].Name + ", ...", nil
	})
	step("notifiers", func(ctx context.Context) (string, error) {
		n, err := ven.Notifiers(ctx)
		if errors.Is(err, openadr.ErrNotImplemented) {
			return "none (501)", nil
		}
		if err != nil {
			return "", err
		}
		var via []string
		if n.Webhook {
			via = append(via, "webhook")
		}
		if n.MQTT != nil {
			via = append(via, "mqtt "+strings.Join(n.MQTT.URIs, ","))
		}
		if n.WebSocket {
			via = append(via, "websocket")
		}
		if len(via) == 0 {
			return "none", nil
		}
		return strings.Join(via, ", "), nil
	})
	p.OK = !failed
	return p, nil
}

var errSkipped = errors.New("skipped")
