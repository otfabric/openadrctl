// SPDX-License-Identifier: MIT

package vtn

import (
	"crypto/subtle"
	"encoding/json"
	"mime"
	"net/http"

	"github.com/otfabric/go-openadr/auth"
)

// mqttAuth answers a broker that asks whether a client may log in, as an
// HTTP authenticator such as EMQX's asks: a POST of the user name and
// password, as JSON or as a form. A client may when its password is a
// token this VTN issued to the client the user name names; the VTN's own
// publisher may with --mqtt-username and its password.
//
// The answer is 200 {"result": "allow"}, or 403 {"result": "deny"}.
type mqttAuth struct {
	issuer             *auth.Issuer
	username, password string
	rec                *recorder
}

func (m *mqttAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	allowed := m.allowed(r, login.Username, login.Password)
	result, status := "deny", http.StatusForbidden
	if allowed {
		result, status = "allow", http.StatusOK
	}
	m.rec.emit(&Record{Event: "mqtt_login", Client: login.Username, Status: status})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
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
