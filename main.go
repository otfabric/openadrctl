// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/otfabric/openadrctl/cmd"
)

// Build-time variables injected via ldflags; empty means a development build
// (see internal/buildinfo).
var (
	version   = ""
	tag       = ""
	commit    = ""
	buildDate = ""
)

// exit ends the process; a test replaces it.
var exit = os.Exit

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	exit(cmd.Execute())
}
