// SPDX-License-Identifier: MIT

// Package conn connects openadrctl to a VTN: the flags every client command
// shares, the go-openadr clients they make, and the classification of what
// goes wrong into the error kinds of the tool (docs/exitcodes.md).
package conn

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/otfabric/go-openadr/client"

	"github.com/otfabric/openadrctl/internal/errs"
)

// Config is the connection of a client command: the VTN, the client's
// credentials, and how to reach the VTN.
type Config struct {
	URL              string        `flag:"url" env:"OPENADRCTL_URL" desc:"Base URL of the VTN's API, the one /programs is appended to, such as https://vtn.example/openadr3/3.1.0"`
	ClientID         string        `flag:"client-id" env:"OPENADRCTL_CLIENT_ID" desc:"OAuth2 client ID (client-credentials grant)"`
	ClientSecret     string        `env:"OPENADRCTL_CLIENT_SECRET"`
	ClientSecretFile string        `flag:"client-secret-file" env:"OPENADRCTL_CLIENT_SECRET_FILE" desc:"File holding the OAuth2 client secret (or OPENADRCTL_CLIENT_SECRET; never a flag)"`
	TokenURL         string        `flag:"token-url" env:"OPENADRCTL_TOKEN_URL" desc:"Token endpoint (default: what the VTN names at GET /auth/server)"`
	Scopes           []string      `flag:"scope" env:"OPENADRCTL_SCOPE" desc:"Scopes to ask the token endpoint for (default: all the client has)"`
	Token            string        `env:"OPENADRCTL_TOKEN"`
	TokenFile        string        `flag:"token-file" env:"OPENADRCTL_TOKEN_FILE" desc:"File holding a bearer token, instead of client credentials (or OPENADRCTL_TOKEN)"`
	CA               string        `flag:"ca" env:"OPENADRCTL_CA" desc:"CA certificates that verify the VTN (PEM; default: the system roots)"`
	Cert             string        `flag:"cert" env:"OPENADRCTL_CERT" desc:"Client certificate for mutual TLS (PEM)"`
	Key              string        `flag:"key" env:"OPENADRCTL_KEY" desc:"Private key of --cert (PEM)"`
	Insecure         bool          `flag:"insecure" env:"OPENADRCTL_INSECURE" desc:"Do not verify the VTN's certificate (testing only)"`
	Timeout          time.Duration `flag:"timeout" env:"OPENADRCTL_TIMEOUT" desc:"Time limit of each command's exchange with the VTN"`
	Retries          int           `flag:"retries" env:"OPENADRCTL_RETRIES" desc:"Retry a request that failed in a way another attempt can cure (no answer, 429, 502-504) up to this many times; a create whose fate is not known is never resent"`
}

// Defaults returns the defaults of a Config.
func Defaults() Config { return Config{Timeout: 30 * time.Second} }

// Validate checks c without touching the network or the files.
func (c Config) Validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errs.InvalidInput(errs.CodeInvalidInput, "--url is required (or OPENADRCTL_URL)", nil)
	}
	if _, err := BaseURL(c.URL); err != nil {
		return err
	}
	secret := c.ClientSecret != "" || c.ClientSecretFile != ""
	token := c.Token != "" || c.TokenFile != ""
	switch {
	case c.ClientSecret != "" && c.ClientSecretFile != "":
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, "give the client secret in OPENADRCTL_CLIENT_SECRET or --client-secret-file, not both", nil)
	case c.Token != "" && c.TokenFile != "":
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, "give the token in OPENADRCTL_TOKEN or --token-file, not both", nil)
	case token && (c.ClientID != "" || secret):
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, "give a bearer token or client credentials, not both", nil)
	case c.ClientID != "" && !secret:
		return errs.InvalidInput(errs.CodeInvalidInput, "--client-id needs a secret: OPENADRCTL_CLIENT_SECRET or --client-secret-file", nil)
	case secret && c.ClientID == "":
		return errs.InvalidInput(errs.CodeInvalidInput, "a client secret needs --client-id (or OPENADRCTL_CLIENT_ID)", nil)
	}
	if c.TokenURL != "" {
		if u, err := url.Parse(c.TokenURL); err != nil || !u.IsAbs() {
			return errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--token-url %q: an absolute URL", c.TokenURL), err)
		}
	}
	if (c.Cert == "") != (c.Key == "") {
		return errs.InvalidInput(errs.CodeInvalidFlagCombination, "--cert and --key go together", nil)
	}
	if c.Timeout <= 0 {
		return errs.InvalidInput(errs.CodeInvalidInput, "--timeout must be positive", nil)
	}
	if c.Retries < 0 {
		return errs.InvalidInput(errs.CodeInvalidInput, "--retries must not be negative", nil)
	}
	return nil
}

// BaseURL checks --url: an http or https URL, without a trailing slash.
func BaseURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errs.InvalidInput(errs.CodeInvalidURL, fmt.Sprintf("--url %q: an http or https URL, such as https://vtn.example/openadr3/3.1.0", s), err)
	}
	if u.User != nil {
		return "", errs.InvalidInput(errs.CodeInvalidURL, "--url must not hold credentials", nil)
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

// Conn is a connection that is set up: the HTTP client, the source of
// tokens, and the configuration of the go-openadr clients.
type Conn struct {
	Base   string
	HTTP   *http.Client
	TLS    *tls.Config
	Tokens *TokenSource // nil without client credentials
	Token  string       // a static token, when given
	cfg    Config
}

// Open reads the files c names and builds the HTTP client. It does not
// contact the VTN.
func (c Config) Open() (*Conn, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	base, _ := BaseURL(c.URL)
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.Insecure} //nolint:gosec // --insecure is the user's choice
	if c.CA != "" {
		pool, err := LoadPool(c.CA)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	if c.Cert != "" {
		cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
		if err != nil {
			return nil, errs.InvalidInput(errs.CodeInvalidCertificate, fmt.Sprintf("the certificate %s and key %s: %v", c.Cert, c.Key, err), err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	// No client timeout: it would end notification streams. Each command
	// bounds its exchange with a context.
	hc := &http.Client{Transport: tr}
	out := &Conn{Base: base, HTTP: hc, TLS: tlsCfg, cfg: c}
	switch {
	case c.Token != "":
		out.Token = strings.TrimSpace(c.Token)
	case c.TokenFile != "":
		t, err := readSecret(c.TokenFile, "--token-file")
		if err != nil {
			return nil, err
		}
		out.Token = t
	case c.ClientID != "":
		secret := c.ClientSecret
		if c.ClientSecretFile != "" {
			s, err := readSecret(c.ClientSecretFile, "--client-secret-file")
			if err != nil {
				return nil, err
			}
			secret = s
		}
		out.Tokens = &TokenSource{Base: base, HTTP: hc, ClientID: c.ClientID, ClientSecret: secret, Scopes: c.Scopes, TokenURL: c.TokenURL}
	}
	return out, nil
}

func readSecret(file, flag string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("%s %s: %v", flag, file, err), err)
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%s %s is empty", flag, file), nil)
	}
	return s, nil
}

// ClientConfig is the configuration of a go-openadr client on c.
func (c *Conn) ClientConfig() client.Config {
	cfg := client.Config{BaseURL: c.Base, HTTPClient: c.HTTP, UserAgent: "openadrctl"}
	switch {
	case c.Tokens != nil:
		cfg.Tokens = c.Tokens
	case c.Token != "":
		cfg.Tokens = client.StaticToken(c.Token)
	}
	if c.cfg.Retries > 0 {
		cfg.RetryPolicy = client.ExponentialBackoff(500*time.Millisecond, 10*time.Second, c.cfg.Retries+1)
	}
	return cfg
}

// VEN returns a client that speaks as a VEN.
func (c *Conn) VEN() (*client.VEN, error) {
	v, err := client.NewVEN(c.ClientConfig())
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	return v, nil
}

// BL returns a client that speaks as Business Logic.
func (c *Conn) BL() (*client.BL, error) {
	b, err := client.NewBL(c.ClientConfig())
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidConfig, err.Error(), err)
	}
	return b, nil
}

// LoadPool loads the CA certificates of a PEM file.
func LoadPool(file string) (*x509.CertPool, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("the CA certificates %s: %v", file, err), err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errs.InvalidInput(errs.CodeInvalidCertificate, fmt.Sprintf("%s holds no PEM certificate", file), nil)
	}
	return pool, nil
}

// drain reads and closes a response body.
func drain(r io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r, 1<<20))
	_ = r.Close()
}
