// SPDX-License-Identifier: MIT

package oadr

import (
	"bytes"
	"testing"

	openadr "github.com/otfabric/go-openadr"
)

// roundTrip checks what the CLI does with the objects it reads from files,
// stdin and the network: what encodes decodes again, to the same encoding.
func roundTrip[T Object](t *testing.T, data []byte) {
	t.Helper()
	v, err := Decode[T](data)
	if err != nil {
		return
	}
	enc, err := Encode(v)
	if err != nil {
		return // the encoder refuses what the revision requires and the input lacks
	}
	back, err := Decode[T](enc)
	if err != nil {
		t.Fatalf("own encoding does not decode: %v\n%s", err, enc)
	}
	again, err := Encode(back)
	if err != nil {
		t.Fatalf("second encoding: %v", err)
	}
	if !bytes.Equal(enc, again) {
		t.Fatalf("encoding is not stable:\n%s\n%s", enc, again)
	}
}

// FuzzDecode feeds JSON to the decoder of every object type, as both
// revisions read it.
func FuzzDecode(f *testing.F) {
	for _, s := range []string{
		`{}`,
		`null`,
		`[]`,
		`{"programName":"p"}`,
		`{"programName":"p","retailerName":"r","payloadDescriptors":[{"objectType":"EVENT_PAYLOAD_DESCRIPTOR","payloadType":"PRICE"}]}`,
		`{"programID":"1","intervals":[{"id":0,"payloads":[{"type":"PRICE","values":[0.17]}]}]}`,
		`{"venName":"v","attributes":[{"type":"LOCATION","values":[1,true,"x"]}]}`,
		`{"resourceName":"r","venID":"1"}`,
		`{"clientName":"c","programID":"1","objectOperations":[{"objects":["EVENT"],"operations":["POST"],"callbackUrl":"https://x"}]}`,
		`{"nope":1}`,
		`{"createdDateTime":"2026-01-01T00:00:00Z","id":"1"}`,
	} {
		f.Add([]byte(s))
	}
	before := Revision()
	f.Cleanup(func() { SetRevision(before) })
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, rev := range []openadr.Version{openadr.Version310, openadr.Version301} {
			SetRevision(rev)
			roundTrip[openadr.Program](t, data)
			roundTrip[openadr.Event](t, data)
			roundTrip[openadr.Report](t, data)
			roundTrip[openadr.VEN](t, data)
			roundTrip[openadr.Resource](t, data)
			roundTrip[openadr.Subscription](t, data)
			roundTrip[openadr.ResourceGroup](t, data)
		}
	})
}
