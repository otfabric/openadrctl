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

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	os.Exit(cmd.Execute())
}
