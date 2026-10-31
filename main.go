// SPDX-License-Identifier: MIT

package main

import (
	"os"

	"github.com/otfabric/openadrctl/cmd"
)

// Build-time variables injected via ldflags.
var (
	version   = "dev"
	tag       = "none"
	commit    = "unknown"
	buildDate = "unknown"
)

// exit ends the process; a test replaces it.
var exit = os.Exit

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	exit(cmd.Execute())
}
