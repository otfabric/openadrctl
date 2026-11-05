// SPDX-License-Identifier: MIT

// Package oadr turns the objects of go-openadr into what openadrctl reads
// and writes: their JSON as the revision of OpenADR of --spec gives it
// (Revision), and their text and table forms.
package oadr

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/objectjson"

	"github.com/otfabric/openadrctl/internal/errs"
)

// Object is any of the seven kinds of object of the API.
type Object interface {
	openadr.Program | openadr.Event | openadr.Report | openadr.VEN | openadr.Resource |
		openadr.Subscription | openadr.ResourceGroup
}

// revision is the revision of OpenADR whose JSON Encode writes and Decode
// reads: that of --spec, set by a command before it reads or writes any.
var revision atomic.Value // of openadr.Version

// SetRevision makes v the revision of Encode and Decode.
func SetRevision(v openadr.Version) { revision.Store(v) }

// Revision is the revision of Encode and Decode: 3.1.0 unless set.
func Revision() openadr.Version {
	if v, ok := revision.Load().(openadr.Version); ok {
		return v
	}
	return openadr.Version310
}

// zeroTimes is what an object that no VTN has stored says of its times.
var zeroTimes = []byte(`"createdDateTime":"0001-01-01T00:00:00Z","modificationDateTime":"0001-01-01T00:00:00Z",`)

// Encode returns the JSON of an object, as a VTN of Revision gives it. The
// times of an object no VTN has stored are left out.
func Encode(v any) (json.RawMessage, error) {
	var data []byte
	var err error
	switch v := v.(type) {
	case openadr.Program:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.Event:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.Report:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.VEN:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.Resource:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.ResourceGroup:
		data, err = objectjson.Marshal(Revision(), v)
	case openadr.Subscription:
		data, err = objectjson.Marshal(Revision(), v)
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
	v, err := objectjson.Unmarshal[T](Revision(), data)
	if err != nil {
		return out, err
	}
	if err := unknownProperties(data, v); err != nil {
		return out, err
	}
	return v, nil
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
			return fmt.Errorf("not a property of OpenADR %s: %s", Revision(), strings.Join(unknown, ", "))
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

// MaxInputSize is the most openadrctl reads of a file or stdin it is given:
// an object, a seed, a timeline CSV, a certificate. Larger input is refused,
// not read into memory without end.
const MaxInputSize = 16 << 20

// ErrTooLarge is the error of input of more than MaxInputSize bytes.
var ErrTooLarge = fmt.Errorf("larger than %d MiB", MaxInputSize>>20)

// LimitReader returns a reader of r that fails with ErrTooLarge once more
// than MaxInputSize bytes were read, rather than stopping without a word as
// io.LimitReader does.
func LimitReader(r io.Reader) io.Reader { return &limited{r: r, left: MaxInputSize} }

type limited struct {
	r    io.Reader
	left int64
}

func (l *limited) Read(p []byte) (int, error) {
	if l.left < 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > l.left+1 {
		p = p[:l.left+1]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		return n + int(l.left), ErrTooLarge
	}
	return n, err
}

// ReadAll reads r to its end, MaxInputSize bytes at most.
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(LimitReader(r)) }

// ReadInput reads a file, or stdin for "-", MaxInputSize bytes at most.
func ReadInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return ReadAll(stdin)
	}
	return ReadFileLimited(path)
}

// ReadFileLimited is os.ReadFile of MaxInputSize bytes at most: a larger
// file, or a FIFO or device without end, fails with ErrTooLarge.
func ReadFileLimited(path string) ([]byte, error) {
	f, err := os.Open(filepath.Clean(path)) // the file the user named
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return ReadAll(f)
}

// LoadKeyPair is tls.LoadX509KeyPair with files of MaxInputSize bytes at
// most.
func LoadKeyPair(certFile, keyFile string) (tls.Certificate, error) {
	certPEM, err := ReadFileLimited(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM, err := ReadFileLimited(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// InputError is the error of a file or stdin that could not be read: too
// large, or not there.
func InputError(path string, err error) error {
	if errors.Is(err, ErrTooLarge) {
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%s: %v", path, err), err)
	}
	return errs.InvalidInput(errs.CodeInputFileOpenFailed, fmt.Sprintf("%s: %v", path, err), err)
}

// ReadFile reads the JSON of an object from a file, or from stdin for "-".
func ReadFile[T Object](path string, stdin io.Reader) (T, error) {
	var zero T
	data, err := ReadInput(path, stdin)
	if err != nil {
		return zero, InputError(path, err)
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
