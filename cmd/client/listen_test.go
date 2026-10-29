// SPDX-License-Identifier: MIT

package client

import (
	"testing"

	openadr "github.com/otfabric/go-openadr"
)

func TestMQTTMethod(t *testing.T) {
	for in, want := range map[string]string{
		"OAUTH2_BEARER_TOKEN": openadr.MQTTBearerToken,
		"OAUTH2BEARERTOKEN":   openadr.MQTTBearerToken, // OpenLEADR
		"oauth2_bearer_token": openadr.MQTTBearerToken,
		"ANONYMOUS":           openadr.MQTTAnonymous,
		"CERTIFICATE":         openadr.MQTTCertificate,
		"SOMETHING_ELSE":      "SOMETHING_ELSE",
	} {
		if got := mqttMethod(in); got != want {
			t.Errorf("mqttMethod(%q) = %q, want %q", in, got, want)
		}
	}
}
