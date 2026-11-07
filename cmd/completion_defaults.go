// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	openadr "github.com/otfabric/go-openadr"
)

// Completion for the flags and arguments the commands do not complete
// themselves. registerDefaultCompletions runs over the whole tree once every
// command registered its own; it never replaces a completion function or a
// file-name annotation (MarkFlagFilename).

var (
	specValues    = []string{string(openadr.Version310), string(openadr.Version301)}
	objectValues  = []string{"PROGRAM", "EVENT", "REPORT", "SUBSCRIPTION", "VEN", "RESOURCE"}
	opValues      = []string{"CREATE", "UPDATE", "DELETE", "READ"}
	urlSchemes    = []string{"https://", "http://"}
	brokerSchemes = []string{"mqtt://", "mqtts://"}
)

// fixedFlagValues are the value sets of the enum flags without a completion
// of their own.
var fixedFlagValues = map[string][]string{
	"spec":             specValues,
	"via":              {"webhook", "mqtt", "websocket"},
	"mqtt-client-auth": {"anonymous", "token"},
	"object":           objectValues,
	"operation":        opValues,
	"at":               {"now"},
	"start":            {"now"},
}

// fileFlagExtensions are the flags that name a file of a known type.
var fileFlagExtensions = map[string][]string{
	"ca":       {"pem", "crt"},
	"cert":     {"pem", "crt"},
	"key":      {"pem", "key"},
	"mqtt-ca":  {"pem", "crt"},
	"tls-cert": {"pem", "crt"},
	"tls-key":  {"pem", "key"},
}

// anyFileFlags name a file of any type.
var anyFileFlags = map[string]bool{
	"output": true, "store": true, "token-file": true, "client-secret-file": true, "mqtt-password-file": true,
}

func registerDefaultCompletions(c *cobra.Command) {
	if c.Runnable() && c.ValidArgs == nil && c.ValidArgsFunction == nil && !strings.Contains(c.Use, "file>") {
		c.ValidArgsFunction = cobra.NoFileCompletions
	}
	c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) { registerDefaultFlagCompletion(c, f) })
	c.PersistentFlags().VisitAll(func(f *pflag.Flag) { registerDefaultFlagCompletion(c, f) })
	for _, sub := range c.Commands() {
		registerDefaultCompletions(sub)
	}
}

// hasCompletion reports whether flag f of c completes already: with a
// function or with a file-name annotation.
func hasCompletion(c *cobra.Command, f *pflag.Flag) bool {
	if _, ok := c.GetFlagCompletionFunc(f.Name); ok {
		return true
	}
	_, ok := f.Annotations[cobra.BashCompFilenameExt]
	return ok
}

func registerDefaultFlagCompletion(c *cobra.Command, f *pflag.Flag) {
	if f.Value.Type() == "bool" || hasCompletion(c, f) {
		return
	}
	var fn cobra.CompletionFunc
	switch name := f.Name; {
	case fixedFlagValues[name] != nil:
		fn = cobra.FixedCompletions(fixedFlagValues[name], cobra.ShellCompDirectiveNoFileComp)
	case fileFlagExtensions[name] != nil:
		fn = cobra.FixedCompletions(fileFlagExtensions[name], cobra.ShellCompDirectiveFilterFileExt)
	case anyFileFlags[name]:
		fn = cobra.FixedCompletions(nil, cobra.ShellCompDirectiveDefault)
	case name == "url" || name == "token-url" || name == "public-url" || name == "callback-url":
		fn = cobra.FixedCompletions(urlSchemes, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace)
	case name == "broker" || name == "mqtt" || name == "mqtt-public-url":
		fn = cobra.FixedCompletions(brokerSchemes, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace)
	case name == "child":
		fn = cobra.FixedCompletions([]string{"resource:", "group:"}, cobra.ShellCompDirectiveNoFileComp|cobra.ShellCompDirectiveNoSpace)
	default:
		fn = cobra.NoFileCompletions
	}
	_ = c.RegisterFlagCompletionFunc(f.Name, fn)
}

// jsonFileArg completes JSON files (and - for stdin is typed by hand).
func jsonFileArg(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return []string{"json"}, cobra.ShellCompDirectiveFilterFileExt
}
