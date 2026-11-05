// SPDX-License-Identifier: MIT

package conn

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"

	"github.com/otfabric/openadrctl/internal/errs"
)

func valid() Config {
	c := Defaults()
	c.URL = "http://127.0.0.1:1/openadr3/3.1.0"
	return c
}

func TestValidate(t *testing.T) {
	for name, c := range map[string]struct {
		edit func(*Config)
		code string
	}{
		"no url":             {func(c *Config) { c.URL = " " }, errs.CodeInvalidInput},
		"ftp url":            {func(c *Config) { c.URL = "ftp://vtn" }, errs.CodeInvalidURL},
		"url with password":  {func(c *Config) { c.URL = "http://u:p@vtn" }, errs.CodeInvalidURL},
		"spec":               {func(c *Config) { c.Spec = "2.0b" }, errs.CodeInvalidInput},
		"two secrets":        {func(c *Config) { c.ClientID, c.ClientSecret, c.ClientSecretFile = "a", "s", "f" }, errs.CodeInvalidFlagCombination},
		"two tokens":         {func(c *Config) { c.Token, c.TokenFile = "t", "f" }, errs.CodeInvalidFlagCombination},
		"token and secret":   {func(c *Config) { c.Token, c.ClientID, c.ClientSecret = "t", "a", "s" }, errs.CodeInvalidFlagCombination},
		"id without secret":  {func(c *Config) { c.ClientID = "a" }, errs.CodeInvalidInput},
		"secret without id":  {func(c *Config) { c.ClientSecret = "s" }, errs.CodeInvalidInput},
		"relative token url": {func(c *Config) { c.TokenURL = "/token" }, errs.CodeInvalidURL},
		"ftp token url":      {func(c *Config) { c.TokenURL = "ftp://idp/token" }, errs.CodeInvalidURL},
		"cert without key":   {func(c *Config) { c.Cert = "c.pem" }, errs.CodeInvalidFlagCombination},
		"zero timeout":       {func(c *Config) { c.Timeout = 0 }, errs.CodeInvalidInput},
		"negative retries":   {func(c *Config) { c.Retries = -1 }, errs.CodeInvalidInput},
	} {
		cfg := valid()
		c.edit(&cfg)
		e, ok := errs.As(cfg.Validate())
		if !ok || e.Kind != errs.KindInvalidInput || e.Code != c.code {
			t.Errorf("%s: %v, want %s", name, e, c.code)
		}
	}
	if err := valid().Validate(); err != nil {
		t.Errorf("valid: %v", err)
	}
	// The password of a URL is not repeated in the error.
	cfg := valid()
	cfg.URL = "ftp://user:hunter2@vtn"
	if err := cfg.Validate(); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("--url with a password: %v", err)
	}
	cfg = valid()
	cfg.TokenURL = "ftp://user:hunter2@idp/token"
	if err := cfg.Validate(); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("--token-url with a password: %v", err)
	}
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	code := func(edit func(*Config)) string {
		c := valid()
		edit(&c)
		_, err := c.Open()
		e, _ := errs.As(err)
		if e == nil {
			return ""
		}
		return e.Code
	}
	for name, c := range map[string]struct {
		edit func(*Config)
		want string
	}{
		"invalid":           {func(c *Config) { c.URL = "" }, errs.CodeInvalidInput},
		"no CA file":        {func(c *Config) { c.CA = filepath.Join(dir, "none") }, errs.CodeInputFileOpenFailed},
		"CA not PEM":        {func(c *Config) { c.CA = write("ca", "not pem") }, errs.CodeInvalidCertificate},
		"bad key pair":      {func(c *Config) { c.Cert, c.Key = write("c", "x"), write("k", "y") }, errs.CodeInvalidCertificate},
		"no token file":     {func(c *Config) { c.TokenFile = filepath.Join(dir, "none") }, errs.CodeInputFileOpenFailed},
		"empty token file":  {func(c *Config) { c.TokenFile = write("empty", " \n") }, errs.CodeInvalidInput},
		"no secret file":    {func(c *Config) { c.ClientID, c.ClientSecretFile = "a", filepath.Join(dir, "none") }, errs.CodeInputFileOpenFailed},
		"empty secret file": {func(c *Config) { c.ClientID, c.ClientSecretFile = "a", write("es", "") }, errs.CodeInvalidInput},
	} {
		if got := code(c.edit); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}

	c := valid()
	c.TokenFile, c.Retries = write("token", " abc \n"), 2
	cn, err := c.Open()
	if err != nil {
		t.Fatal(err)
	}
	if cn.Token != "abc" || cn.Tokens != nil || cn.Version() != openadr.Version310 {
		t.Errorf("token conn: %+v", cn)
	}
	if cn.HTTP.CheckRedirect == nil || cn.HTTP.Transport.(*http.Transport).ResponseHeaderTimeout != c.Timeout {
		t.Error("the HTTP client has no redirect policy or response header timeout")
	}
	if cfg := cn.ClientConfig(); cfg.Tokens == nil || cfg.RetryPolicy == nil {
		t.Errorf("client config: %+v", cfg)
	}
	if _, err := cn.VEN(); err != nil {
		t.Error(err)
	}
	if _, err := cn.BL(); err != nil {
		t.Error(err)
	}

	c = valid()
	c.ClientID, c.ClientSecretFile = "a", write("secret", "s3cret\n")
	if cn, err = c.Open(); err != nil || cn.Tokens == nil || cn.Tokens.ClientSecret != "s3cret" {
		t.Fatalf("credentials conn: %+v %v", cn, err)
	}
	if cfg := cn.ClientConfig(); cfg.Tokens != cn.Tokens {
		t.Error("the token source is not the client's")
	}
	c = valid()
	c.Token = " t "
	if cn, err = c.Open(); err != nil || cn.Token != "t" {
		t.Errorf("static token: %+v %v", cn, err)
	}
	// A client the library refuses: an invalid configuration, exit 3.
	cn.Base = "::"
	if _, err := cn.VEN(); err == nil {
		t.Error("VEN of a bad base URL")
	}
	if _, err := cn.BL(); err == nil {
		t.Error("BL of a bad base URL")
	}
}

func TestSameOrigin(t *testing.T) {
	req := func(u string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, u, nil)
		return r
	}
	first := []*http.Request{req("https://vtn.example/a")}
	if err := SameOrigin(req("https://VTN.example/b"), first); err != nil {
		t.Errorf("same origin: %v", err)
	}
	for _, u := range []string{"http://vtn.example/b", "https://vtn.example:8443/b", "https://evil.example/b"} {
		if err := SameOrigin(req(u), first); !errors.Is(err, http.ErrUseLastResponse) {
			t.Errorf("%s: %v", u, err)
		}
	}
	many := make([]*http.Request, maxRedirects)
	for i := range many {
		many[i] = first[0]
	}
	if err := SameOrigin(req("https://vtn.example/b"), many); !errors.Is(err, openadr.ErrProtocol) {
		t.Errorf("too many redirects: %v", err)
	}
}

// tokenServer serves a token endpoint with h, and returns a source for it.
func tokenServer(t *testing.T, h http.HandlerFunc) (*TokenSource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &TokenSource{Base: srv.URL, HTTP: srv.Client(), ClientID: "c", ClientSecret: "the-secret", TokenURL: srv.URL + "/token"}, srv
}

func TestTokenRedirect(t *testing.T) {
	// The secret does not follow a 307 to another origin.
	got := make(chan string, 1)
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got <- r.PostForm.Get("client_secret")
		_, _ = w.Write([]byte(`{"access_token":"stolen","token_type":"Bearer"}`))
	}))
	defer elsewhere.Close()
	src, _ := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/token", http.StatusTemporaryRedirect)
	})
	_, err := src.Fetch(context.Background())
	if e, ok := errs.As(Classify(err, false)); !ok || e.Code != errs.CodeTokenRefused {
		t.Errorf("redirected token request: %v", err)
	}
	select {
	case s := <-got:
		t.Errorf("the secret went to another origin: %q", s)
	default:
	}
	// Within the origin it is followed.
	src, _ = tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			http.Redirect(w, r, "/token2", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"ok","token_type":"bearer"}`))
	})
	src.HTTP = &http.Client{} // no policy of its own: the source brings one
	if tok, err := src.Fetch(context.Background()); err != nil || tok.AccessToken != "ok" {
		t.Errorf("same-origin redirect: %+v %v", tok, err)
	}
}

func TestTokenHostile(t *testing.T) {
	answer := func(body string) (*Token, error) {
		src, _ := tokenServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		return src.Fetch(context.Background())
	}
	// An expires_in no Duration holds: 100 years, not an expiry in the past.
	tok, err := answer(`{"access_token":"a","token_type":"Bearer","expires_in":1e300}`)
	if err != nil || time.Until(tok.Expires) < 50*365*24*time.Hour {
		t.Fatalf("huge expires_in: %+v %v", tok, err)
	}
	if rows := tok.TableRows(); strings.HasPrefix(rows[0][2], "-") {
		t.Errorf("huge expires_in shown as %q", rows[0][2])
	}
	if tok, err = answer(`{"access_token":"a","token_type":"Bearer","expires_in":-5}`); err != nil || tok.ExpiresIn != 0 || !tok.Expires.IsZero() {
		t.Errorf("negative expires_in: %+v %v", tok, err)
	}
	for name, body := range map[string]string{
		"control characters": `{"access_token":"a\r\nX-Injected: 1","token_type":"Bearer"}`,
		"escape":             `{"access_token":"\u001b]0;pwned\u0007","token_type":"Bearer"}`,
		"space":              `{"access_token":"a b","token_type":"Bearer"}`,
		"too large":          `{"access_token":"` + strings.Repeat("a", maxTokenResponse) + `","token_type":"Bearer"}`,
		"not bearer":         `{"access_token":"a","token_type":"MAC"}`,
		"no token":           `{}`,
	} {
		if _, err := answer(body); !errors.Is(err, openadr.ErrProtocol) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A refusal: its description reaches the message escaped.
	src, _ := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"no\u001b[2J"}`))
	})
	_, err = src.Token(context.Background())
	e, ok := errs.As(Classify(err, true))
	if !errors.Is(err, ErrNoToken) || !ok || e.Code != errs.CodeTokenRefused || strings.ContainsRune(e.Message, 0x1b) || !strings.Contains(e.Message, `\x1b[2J`) {
		t.Errorf("refused: %v", e)
	}
	if d := e.Detail.(HTTPDetail); d.OAuthError != "invalid_client" || d.Status != 401 {
		t.Errorf("detail: %+v", d)
	}
	// The password of a token URL is not printed.
	src, srv := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer","expires_in":60}`))
	})
	src.TokenURL = strings.Replace(srv.URL, "http://", "http://user:hunter2@", 1) + "/token"
	if tok, err = src.Fetch(context.Background()); err != nil || strings.Contains(tok.TokenURL, "hunter2") {
		t.Errorf("token URL: %+v %v", tok, err)
	}
	if s, _ := tok.MarshalTextOutput(); s != "a" || len(tok.TableHeaders()) != 4 {
		t.Errorf("text %q", s)
	}
	// A body that never ends: the context ends the read.
	stop := make(chan struct{})
	defer close(stop)
	src, _ = tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"`))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = src.Token(ctx)
	if e, ok := errs.As(Classify(err, true)); !ok || e.Code != errs.CodeRequestTimeout {
		t.Errorf("endless body: %v (%v)", e, err)
	}
}

func TestTokenCache(t *testing.T) {
	calls := 0
	src, _ := tokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"access_token":"t` + string(rune('0'+calls)) + `","token_type":"Bearer","expires_in":3600}`))
	})
	a, _ := src.Token(context.Background())
	b, _ := src.Token(context.Background())
	src.Invalidate("other")
	c, _ := src.Token(context.Background())
	src.Invalidate(c)
	d, _ := src.Token(context.Background())
	if a != "t1" || b != "t1" || c != "t1" || d != "t2" || calls != 2 {
		t.Errorf("tokens %s %s %s %s after %d calls", a, b, c, d, calls)
	}
}

func TestDiscover(t *testing.T) {
	discover := func(h http.HandlerFunc, tls bool) (string, error) {
		srv := httptest.NewUnstartedServer(h)
		if tls {
			srv.StartTLS()
		} else {
			srv.Start()
		}
		defer srv.Close()
		src := &TokenSource{Base: srv.URL, HTTP: srv.Client(), ClientID: "c", ClientSecret: "s"}
		_, err := src.Fetch(context.Background())
		return src.TokenURL, err
	}
	named := func(u string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/server" {
				_, _ = w.Write([]byte(`{"tokenURL":"` + u + `"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"a","token_type":"Bearer"}`))
		}
	}
	if u, err := discover(named("token"), false); err != nil || !strings.HasSuffix(u, "/token") {
		t.Errorf("relative: %s %v", u, err)
	}
	for name, c := range map[string]struct {
		h   http.HandlerFunc
		tls bool
	}{
		"https to http": {named("http://idp.example/token"), true},
		"not http":      {named("file:///etc/passwd"), false},
		"no host":       {named("http:///token"), false},
		"unparsable":    {named("http://[::1"), false},
		"no tokenURL":   {named(""), false},
	} {
		if _, err := discover(c.h, c.tls); !errors.Is(err, openadr.ErrProtocol) {
			t.Errorf("%s: %v", name, err)
		}
	}
	_, err := discover(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"title":"down","detail":"maintenance"}`))
	}, false)
	if e, ok := errs.As(Classify(err, false)); !ok || e.Code != errs.CodeUnavailable || !strings.Contains(e.Message, "maintenance") {
		t.Errorf("503: %v", e)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	if Classify(nil, false) != nil {
		t.Error("nil")
	}
	own := errs.New(errs.KindUsage, errs.CodeConfirmationRequired, "x", nil)
	if Classify(own, false) != own {
		t.Error("an error of the tool is kept")
	}
	api := func(status int) error {
		return &openadr.Error{Method: "GET", Path: "/programs", StatusCode: status, RetryAfter: time.Second}
	}
	for name, c := range map[string]struct {
		err   error
		write bool
		kind  errs.Kind
		code  string
	}{
		"canceled":        {context.Canceled, false, errs.KindInterrupted, errs.CodeContextCanceled},
		"token 429":       {&client.TokenError{StatusCode: 429}, false, errs.KindNegative, errs.CodeTooManyRequests},
		"token 500":       {&client.TokenError{StatusCode: 500}, false, errs.KindNegative, errs.CodeServerError},
		"400":             {api(400), false, errs.KindNegative, errs.CodeBadRequest},
		"401":             {api(401), false, errs.KindNegative, errs.CodeUnauthorized},
		"403":             {api(403), false, errs.KindNegative, errs.CodeForbidden},
		"404":             {api(404), false, errs.KindNegative, errs.CodeNotFound},
		"409":             {api(409), false, errs.KindNegative, errs.CodeConflict},
		"429":             {api(429), false, errs.KindNegative, errs.CodeTooManyRequests},
		"501":             {api(501), false, errs.KindNegative, errs.CodeNotImplemented},
		"503":             {api(503), false, errs.KindNegative, errs.CodeUnavailable},
		"500":             {api(500), false, errs.KindNegative, errs.CodeServerError},
		"302":             {api(302), false, errs.KindNegative, errs.CodeHTTPError},
		"read timeout":    {context.DeadlineExceeded, false, errs.KindTimeout, errs.CodeRequestTimeout},
		"write timeout":   {openadr.ErrTimeout, true, errs.KindTimeout, errs.CodeOutcomeUnknown},
		"no token":        {noToken{context.DeadlineExceeded}, true, errs.KindTimeout, errs.CodeRequestTimeout},
		"net timeout":     {timeoutErr{}, true, errs.KindTimeout, errs.CodeOutcomeUnknown},
		"protocol":        {openadr.ErrProtocol, false, errs.KindProtocol, errs.CodeInvalidResponse},
		"validation":      {&openadr.ValidationError{Field: "name", Reason: "x"}, true, errs.KindInvalidInput, errs.CodeInvalidObject},
		"configuration":   {openadr.ErrConfiguration, false, errs.KindInvalidInput, errs.CodeInvalidConfig},
		"unsupported":     {errors.ErrUnsupported, false, errs.KindInvalidInput, errs.CodeNotInRevision},
		"tls":             {errors.New("remote error: tls: bad certificate"), false, errs.KindTransport, errs.CodeTransportTLSFailed},
		"other":           {errors.New("connection refused"), false, errs.KindTransport, errs.CodeTransportConnectFailed},
		"redirect cutoff": {&openadrURLErr{SameOrigin(nil, make([]*http.Request, maxRedirects))}, false, errs.KindProtocol, errs.CodeInvalidResponse},
	} {
		e, ok := errs.As(Classify(c.err, c.write))
		if !ok || e.Kind != c.kind || e.Code != c.code {
			t.Errorf("%s: %+v, want %s/%s", name, e, c.kind, c.code)
		}
	}
	e, _ := errs.As(Classify(api(429), false))
	if d := e.Detail.(HTTPDetail); d.RetryAfter != "1s" || d.Status != 429 {
		t.Errorf("429 detail: %+v", d)
	}
	e, _ = errs.As(Classify(api(401), false))
	if !strings.Contains(e.Message, "--client-id") {
		t.Errorf("401: %s", e.Message)
	}
	// What the VTN wrote is escaped and cut in the message, kept in the detail.
	hostile := &openadr.Error{Method: "GET", Path: "/events", StatusCode: 400,
		Problem: openadr.Problem{Title: "Bad", Detail: "\x1b]8;;http://evil\x07click\n" + strings.Repeat("x", 4000)}}
	e, _ = errs.As(Classify(hostile, false))
	if strings.ContainsAny(e.Message, "\x1b\x07\n") || len(e.Message) > maxMessage+3 || !strings.HasSuffix(e.Message, "...") {
		t.Errorf("hostile message: %q", e.Message)
	}
	if d := e.Detail.(HTTPDetail); d.Problem == nil || d.Problem.Detail != hostile.Problem.Detail {
		t.Errorf("hostile detail: %+v", d)
	}
	e, _ = errs.As(Classify(&openadr.Error{Method: "GET", Path: "/x", StatusCode: 418, Problem: openadr.Problem{Title: "Teapot"}}, false))
	if !strings.HasSuffix(e.Message, ": Teapot") {
		t.Errorf("title: %s", e.Message)
	}
}

// openadrURLErr is an error that wraps another, as url.Error does.
type openadrURLErr struct{ err error }

func (e *openadrURLErr) Error() string { return "Get \"x\": " + e.err.Error() }
func (e *openadrURLErr) Unwrap() error { return e.err }

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"plain":           "plain",
		"a\nb\tc\rd":      `a\nb\tc\rd`,
		"\x1b[31mred":     `\x1b[31mred`,
		"\u202eevil":      `\u202eevil`,
		"bad \xff byte":   `bad \xff byte`,
		"ünïcödé":         "ünïcödé",
		"c1 \u0085 next":  `c1 \x85 next`,
		"line\u2028break": `line\u2028break`,
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	// Cut on a rune, never inside one.
	odd := "a" + strings.Repeat("é", maxMessage)
	if got := Clean(odd); !strings.HasSuffix(got, "...") || !utf8.ValidString(got) || len(got) > maxMessage+3 {
		t.Errorf("odd: %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
	long := strings.Repeat("é", maxMessage)
	if got := Clean(long); len(got) > maxMessage+3 || !strings.HasSuffix(got, "...") || strings.ContainsRune(got, 0xfffd) {
		t.Errorf("long: %d bytes", len(got))
	}
}

// An answer to GET /auth/server cut off before its end fails Fetch, and a
// TokenSource without an HTTP client uses one of its own.
func TestDiscoverCutOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte(`{"tokenURL":`))
	}))
	defer srv.Close()
	src := &TokenSource{Base: srv.URL, ClientID: "c", ClientSecret: "s"}
	if _, err := src.Fetch(context.Background()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Fetch: %v, want an unexpected EOF", err)
	}
}
