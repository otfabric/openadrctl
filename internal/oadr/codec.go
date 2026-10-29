// SPDX-License-Identifier: MIT

// Package oadr turns the objects of go-openadr into what openadrctl reads
// and writes: their JSON as OpenADR 3.1.0 gives it, and their text and
// table forms.
package oadr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/server/storage"

	"github.com/otfabric/openadrctl/internal/errs"
)

// Object is any of the seven kinds of object of the API.
type Object interface {
	openadr.Program | openadr.Event | openadr.Report | openadr.VEN | openadr.Resource |
		openadr.Subscription | openadr.ResourceGroup
}

// zeroTimes is what an object that no VTN has stored says of its times.
var zeroTimes = []byte(`"createdDateTime":"0001-01-01T00:00:00Z","modificationDateTime":"0001-01-01T00:00:00Z",`)

// Encode returns the JSON of an object, as OpenADR 3.1.0 gives it. The
// times of an object no VTN has stored are left out.
func Encode(v any) (json.RawMessage, error) {
	var data []byte
	var err error
	switch v := v.(type) {
	case openadr.Program:
		data, err = storage.Encode(v)
	case openadr.Event:
		data, err = storage.Encode(v)
	case openadr.Report:
		data, err = storage.Encode(v)
	case openadr.VEN:
		data, err = storage.Encode(v)
	case openadr.Resource:
		data, err = storage.Encode(v)
	case openadr.ResourceGroup:
		data, err = storage.Encode(v)
	case openadr.Subscription:
		data, err = storage.Encode(storage.SubscriptionRecord{Subscription: v})
		if err == nil {
			var rec struct {
				Subscription json.RawMessage `json:"subscription"`
			}
			err = json.Unmarshal(data, &rec)
			data = rec.Subscription
		}
	default:
		return nil, fmt.Errorf("oadr: cannot encode a %T", v)
	}
	if err != nil {
		return nil, err
	}
	return bytes.Replace(data, zeroTimes, nil, 1), nil
}

// Decode reads the JSON of an object.
func Decode[T Object](data []byte) (T, error) {
	var out T
	var v any
	var err error
	switch any(out).(type) {
	case openadr.Program:
		v, err = storage.Decode[openadr.Program](data)
	case openadr.Event:
		v, err = storage.Decode[openadr.Event](data)
	case openadr.Report:
		v, err = storage.Decode[openadr.Report](data)
	case openadr.VEN:
		v, err = storage.Decode[openadr.VEN](data)
	case openadr.Resource:
		v, err = storage.Decode[openadr.Resource](data)
	case openadr.ResourceGroup:
		v, err = storage.Decode[openadr.ResourceGroup](data)
	case openadr.Subscription:
		wrapped, _ := json.Marshal(struct {
			Subscription json.RawMessage `json:"subscription"`
		}{json.RawMessage(data)})
		var rec storage.SubscriptionRecord
		rec, err = storage.Decode[storage.SubscriptionRecord](wrapped)
		v = rec.Subscription
	}
	if err != nil {
		return out, err
	}
	if err := unknownProperties(data, v); err != nil {
		return out, err
	}
	return v.(T), nil
}

// unknownProperties reports the properties of data that the object does not
// have: a property that does not come back when the object is encoded again,
// and is not empty. It catches a misspelt name, which would otherwise be
// dropped without a word.
func unknownProperties(data []byte, v any) error {
	var in map[string]any
	var back map[string]json.RawMessage
	enc, err := Encode(v)
	if err == nil && json.Unmarshal(data, &in) == nil && json.Unmarshal(enc, &back) == nil {
		var unknown []string
		for k, val := range in {
			if _, ok := back[k]; !ok && !empty(val) {
				unknown = append(unknown, fmt.Sprintf("%q", k))
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return fmt.Errorf("not a property of OpenADR 3.1.0: %s", strings.Join(unknown, ", "))
		}
	}
	return nil
}

func empty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case bool:
		return !v
	case float64:
		return v == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// ReadFile reads the JSON of an object from a file, or from stdin for "-".
func ReadFile[T Object](path string, stdin io.Reader) (T, error) {
	var zero T
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return zero, errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("%s: %v", path, err), err)
	}
	if !json.Valid(data) {
		return zero, errs.InvalidInput(errs.CodeInputDecodeFailed, fmt.Sprintf("%s is not JSON", path), nil)
	}
	v, err := Decode[T](data)
	if err != nil {
		return zero, errs.InvalidInput(errs.CodeInputDecodeFailed, fmt.Sprintf("%s: %v", path, err), err)
	}
	return v, nil
}

// MustEncode is Encode for objects that came from go-openadr, which always
// encode.
func MustEncode(v any) json.RawMessage {
	data, err := Encode(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return data
}
