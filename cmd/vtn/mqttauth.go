// SPDX-License-Identifier: MIT

package vtn

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-openadr/auth"
)

// mqttAuth answers a broker that asks whether a client may log in, as an
// HTTP authenticator such as EMQX's asks: a POST of the user name and
// password, as JSON or as a form. A client may when its password is a
// token this VTN issued to the client the user name names; the VTN's own
// publisher may with --mqtt-username and its password.
//
// The answer is 200 {"result": "allow"}, or 403 {"result": "deny"}. With a
// secret (OPENADRCTL_VTN_MQTT_AUTH_SECRET) only the broker, which sends it
// as its bearer token, is answered: any other request is answered 401
// before its body is read.
type mqttAuth struct {
	issuer             *auth.Issuer
	username, password string
	secret             string
	rec                *recorder
	refused            refusals
	// lock answers the publisher's user name 429 for a while after
	// wrong passwords, and held sums those answers up.
	lock lockout
	held refusals
}

func (m *mqttAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.secret != "" && !m.fromBroker(r) {
		if n, ok := m.refused.note(time.Now()); ok {
			m.rec.emit(&Record{Event: "log", Level: "warn",
				Message: fmt.Sprintf("POST /auth/mqtt: requests without the broker's secret answered 401 since the last such record: %d (one record a minute at most)", n)})
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="mqtt"`)
		http.Error(w, "the broker's secret is required", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST the user name and password", http.StatusMethodNotAllowed)
		return
	}
	var login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
			http.Error(w, "not JSON", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "not a form", http.StatusBadRequest)
			return
		}
		login.Username, login.Password = r.PostForm.Get("username"), r.PostForm.Get("password")
	}
	publisher := m.username != "" && login.Username == m.username
	if wait, locked := m.lock.locked(time.Now()); publisher && locked {
		if n, ok := m.held.note(time.Now()); ok {
			m.rec.emit(&Record{Event: "log", Level: "warn",
				Message: fmt.Sprintf("POST /auth/mqtt: logins of %s answered 429 after %d wrong passwords, since the last such record: %d (one record a minute at most)", m.username, maxPublisherFailures, n)})
		}
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second)/time.Second)+1))
		http.Error(w, "too many wrong passwords for this user name: try again later", http.StatusTooManyRequests)
		return
	}
	allowed := m.allowed(r, login.Username, login.Password)
	if publisher && !allowed {
		m.lock.failed(time.Now())
	}
	result, status := "deny", http.StatusForbidden
	if allowed {
		result, status = "allow", http.StatusOK
	}
	m.rec.emit(&Record{Event: "mqtt_login", Client: login.Username, Status: status})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
}

// fromBroker tells whether r carries the broker's secret as its bearer token.
func (m *mqttAuth) fromBroker(r *http.Request) bool {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	return ok && strings.EqualFold(scheme, "Bearer") && subtle.ConstantTimeCompare([]byte(token), []byte(m.secret)) == 1
}

// refusals counts the requests answered 401, and says when to write a
// record of them: for the first, and then at most once a minute, with the
// number since the last record. A host that sends request after request
// does not write a record for each.
type refusals struct {
	mu   sync.Mutex
	n    int
	next time.Time
}

func (q *refusals) note(now time.Time) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.n++
	if now.Before(q.next) {
		return 0, false
	}
	n := q.n
	q.n, q.next = 0, now.Add(time.Minute)
	return n, true
}

func (m *mqttAuth) allowed(r *http.Request, username, password string) bool {
	if username == "" || password == "" {
		return false
	}
	if m.username != "" && username == m.username {
		return subtle.ConstantTimeCompare([]byte(password), []byte(m.password)) == 1
	}
	id, err := m.issuer.Validate(r.Context(), password)
	return err == nil && id.ClientID == username
}

// After maxPublisherFailures wrong passwords for the publisher's user name
// within publisherLockout, that user name is answered 429 for
// publisherLockout, also with the right password. The tokens of the
// clients are not affected. They are variables for the tests.
var (
	maxPublisherFailures = 10
	publisherLockout     = time.Minute
)

// lockout counts the wrong passwords of the publisher's user name.
type lockout struct {
	mu    sync.Mutex
	since time.Time // the first failure that counts
	n     int
	until time.Time // answered 429 until then
}

// locked reports whether the user name is answered 429 at now, and for how
// long still.
func (l *lockout) locked(now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Before(l.until) {
		return l.until.Sub(now), true
	}
	return 0, false
}

// failed counts a wrong password at now.
func (l *lockout) failed(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n == 0 || now.Sub(l.since) >= publisherLockout {
		l.since, l.n = now, 0
	}
	l.n++
	if l.n >= maxPublisherFailures {
		l.until, l.n = now.Add(publisherLockout), 0
	}
}
