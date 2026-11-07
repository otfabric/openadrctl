package client

import "testing"

func TestWithArticle(t *testing.T) {
	for name, want := range map[string]string{
		"event":          "an event",
		"VEN":            "a VEN",
		"program":        "a program",
		"resource group": "a resource group",
		"subscription":   "a subscription",
	} {
		if got := withArticle(name); got != want {
			t.Errorf("withArticle(%q) = %q, want %q", name, got, want)
		}
	}
}
