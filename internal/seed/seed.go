// SPDX-License-Identifier: MIT

// Package seed reads the seed file of vtn start: the OAuth2 clients of the
// VTN and the objects it starts with.
package seed

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/auth"
	"github.com/otfabric/go-openadr/server/storage"
	"gopkg.in/yaml.v3"

	"github.com/otfabric/openadrctl/internal/oadr"
)

// Example is the commented seed file generate-config writes.
//
//go:embed example.yaml
var Example string

// Client is an OAuth2 client of the VTN.
type Client struct {
	ID        string `yaml:"id"`
	Secret    string `yaml:"secret"`
	SecretEnv string `yaml:"secretEnv"`
	Role      string `yaml:"role"`
}

// File is a seed file as written.
type File struct {
	Clients        []Client         `yaml:"clients"`
	Programs       []map[string]any `yaml:"programs"`
	VENs           []map[string]any `yaml:"vens"`
	Resources      []map[string]any `yaml:"resources"`
	Events         []map[string]any `yaml:"events"`
	ResourceGroups []map[string]any `yaml:"resourceGroups"`
}

// Seed is a seed file that was read and checked.
type Seed struct {
	Clients        []auth.Client
	Programs       []openadr.Program
	VENs           []openadr.VEN
	Resources      []openadr.Resource
	Events         []openadr.Event
	ResourceGroups []openadr.ResourceGroup
}

// Load reads and checks a seed file, YAML or JSON. Every problem is
// reported, not only the first.
func Load(path string) (*Seed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse reads and checks the content of a seed file.
func Parse(data []byte) (*Seed, error) {
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	var problems []error
	s := &Seed{}
	ids := map[string]bool{}
	for i, c := range f.Clients {
		where := fmt.Sprintf("clients[%d]", i)
		secret := c.Secret
		if c.SecretEnv != "" {
			if secret != "" {
				problems = append(problems, fmt.Errorf("%s: secret or secretEnv, not both", where))
			}
			secret = os.Getenv(c.SecretEnv)
			if secret == "" {
				problems = append(problems, fmt.Errorf("%s: the environment variable %s is not set", where, c.SecretEnv))
			}
		}
		client := auth.Client{ID: c.ID, Secret: secret}
		switch c.Role {
		case "bl":
			client.Scopes = auth.BusinessLogicScopes()
		case "ven":
			client.Scopes = auth.VENScopes()
		default:
			problems = append(problems, fmt.Errorf("%s: role %q: bl or ven", where, c.Role))
		}
		switch {
		case c.ID == "":
			problems = append(problems, fmt.Errorf("%s: id is required", where))
		case ids[c.ID]:
			problems = append(problems, fmt.Errorf("%s: client %q twice", where, c.ID))
		case secret == "" && c.SecretEnv == "":
			problems = append(problems, fmt.Errorf("%s: secret or secretEnv is required", where))
		}
		ids[c.ID] = true
		s.Clients = append(s.Clients, client)
	}
	s.Programs = objects[openadr.Program]("programs", f.Programs, &problems)
	s.VENs = objects[openadr.VEN]("vens", f.VENs, &problems)
	s.Resources = objects[openadr.Resource]("resources", f.Resources, &problems)
	s.Events = objects[openadr.Event]("events", f.Events, &problems)
	s.ResourceGroups = objects[openadr.ResourceGroup]("resourceGroups", f.ResourceGroups, &problems)
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return s, nil
}

// objects reads objects of one kind, gives each an ID and times when it has
// none, and validates them.
func objects[T oadr.Object](section string, items []map[string]any, problems *[]error) []T {
	var out []T
	now := time.Now().UTC().Truncate(time.Second)
	for i, item := range items {
		where := fmt.Sprintf("%s[%d]", section, i)
		data, err := json.Marshal(item)
		if err != nil {
			*problems = append(*problems, fmt.Errorf("%s: %v", where, err))
			continue
		}
		v, err := oadr.Decode[T](data)
		if err != nil {
			*problems = append(*problems, fmt.Errorf("%s: %v", where, err))
			continue
		}
		if val, ok := any(v).(interface{ Validate() error }); ok {
			if err := val.Validate(); err != nil {
				*problems = append(*problems, fmt.Errorf("%s: %s", where, strings.TrimPrefix(err.Error(), "openadr: ")))
				continue
			}
		}
		m := oadr.MetadataOf(v)
		if m.ID == "" {
			m.ID = NewID()
		} else if !openadr.ValidID(m.ID) {
			*problems = append(*problems, fmt.Errorf("%s: id %q is not an object ID", where, m.ID))
			continue
		}
		if m.Created.IsZero() {
			m.Created = now
		}
		if m.Modified.IsZero() {
			m.Modified = m.Created
		}
		setMetadata(&v, m)
		out = append(out, v)
	}
	return out
}

func setMetadata[T oadr.Object](v *T, m openadr.Metadata) {
	switch p := any(v).(type) {
	case *openadr.Program:
		p.Metadata = m
	case *openadr.Event:
		p.Metadata = m
	case *openadr.VEN:
		p.Metadata = m
	case *openadr.Resource:
		p.Metadata = m
	case *openadr.ResourceGroup:
		p.Metadata = m
	}
}

// NewID returns an object ID as a VTN of go-openadr makes them.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Apply writes the objects of s to a store. An object that is already there
// (a store kept in a file, started again) is left as it is.
func (s *Seed) Apply(ctx context.Context, st storage.Store) (created int, err error) {
	var problems []error
	add := func(where string, err error) {
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %v", where, err))
			return
		}
		created++
	}
	for i, p := range s.Programs {
		if exists(ctx, st.Programs().Get, p.ID) {
			continue
		}
		add(fmt.Sprintf("programs[%d]", i), st.Programs().Create(ctx, p))
	}
	for i, v := range s.VENs {
		if exists(ctx, st.VENs().Get, v.ID) {
			continue
		}
		add(fmt.Sprintf("vens[%d]", i), st.VENs().Create(ctx, v))
	}
	for i, r := range s.Resources {
		if exists(ctx, st.Resources().Get, r.ID) {
			continue
		}
		add(fmt.Sprintf("resources[%d]", i), st.Resources().Create(ctx, r))
	}
	for i, e := range s.Events {
		if exists(ctx, st.Events().Get, e.ID) {
			continue
		}
		add(fmt.Sprintf("events[%d]", i), st.Events().Create(ctx, e))
	}
	for i, g := range s.ResourceGroups {
		if exists(ctx, st.ResourceGroups().Get, g.ID) {
			continue
		}
		add(fmt.Sprintf("resourceGroups[%d]", i), st.ResourceGroups().Create(ctx, g))
	}
	return created, errors.Join(problems...)
}

func exists[T any](ctx context.Context, get func(context.Context, string) (T, error), id string) bool {
	_, err := get(ctx, id)
	return err == nil
}
