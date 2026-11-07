// SPDX-License-Identifier: MIT

// Package cmd is the command tree of openadrctl.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/cmd/client"
	"github.com/otfabric/openadrctl/cmd/vtn"
	"github.com/otfabric/openadrctl/internal/buildinfo"
	"github.com/otfabric/openadrctl/internal/config"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/format"
	"github.com/otfabric/openadrctl/internal/runner"
)

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildinfo.Set(version, tag, commit, buildDate)
}

// NewRootCmd builds the command tree. Every call returns a fresh tree, with the flags of
// every command at the defaults the environment gives them.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "openadrctl",
		Short: "OpenADR 3 command-line VEN, Business Logic and VTN",
		Long: `openadrctl speaks OpenADR 3.1.0, the protocol between a utility or aggregator (the VTN and
its Business Logic) and the sites that respond to demand-response signals (VENs), in every
role. As a VEN and as Business Logic it also speaks OpenADR 3.0.1, with --spec 3.0.1.

As a VEN ("openadrctl ven") it reads the programs and events meant for it, reports on
them, manages its VEN object and resources, and receives notifications by webhook, MQTT or
WebSocket. As Business Logic ("openadrctl bl") it creates programs and events, reads
reports, and manages VENs, resources and resource groups. As a VTN ("openadrctl vtn
start") it serves the API from a seed file, with its own token endpoint.

"ping" checks a VTN step by step, "token" fetches an OAuth2 token, "discover" finds VTNs on
the local network, "validate" checks objects against the schema, and "timeline" lays out
the intervals of an event.

Results go to stdout as text, a table or JSON (--format); progress and diagnostics go to
stderr. Exit codes are stable: see docs/exitcodes.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("debug", false, "Show the causes of an error on stderr")
	root.AddGroup(&cobra.Group{ID: "roles", Title: "Roles:"}, &cobra.Group{ID: "tools", Title: "Tools:"})
	for _, c := range []*cobra.Command{client.NewVENCmd(), client.NewBLCmd(), vtn.NewCmd()} {
		c.GroupID = "roles"
		root.AddCommand(c)
	}
	for _, c := range []*cobra.Command{newPingCmd(), newTokenCmd(), newDiscoverCmd(), newValidateCmd(), newTimelineCmd(),
		newCompletionCmd(), newVersionCmd()} {
		c.GroupID = "tools"
		root.AddCommand(c)
	}
	root.SetHelpCommandGroupID("tools")
	root.SetCompletionCommandGroupID("tools")
	registerDefaultCompletions(root)
	return root
}

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of openadrctl",
		Long: `Print the version, tag, commit, build date, Go version and platform of openadrctl on one
line, or as one JSON object with --json or OPENADRCTL_FORMAT=json.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := buildinfo.Get("openadrctl")
			line := info.String()
			if f, err := format.Parse(config.Getenv("OPENADRCTL_FORMAT")); asJSON || (err == nil && f == format.FormatJSON) {
				line = info.JSON()
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), line)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the build information as one JSON object")
	return cmd
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate a shell completion script for openadrctl: bash, zsh, fish or PowerShell.
Installation: see "Shell completion" in the README.`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			default:
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			}
		},
	}
}

// Execute runs the root command and returns the process exit code (main should call os.Exit).
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// Run executes openadrctl with args, reading stdin and writing to stdout and stderr, and
// returns the exit code. It is Execute without the process: what the tests drive.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root, err := buildRoot()
	if err != nil {
		return runner.RenderFatal(nil, err, stdout, stderr, false)
	}
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	inv := &runner.Invocation{}
	ctx = runner.WithInvocation(ctx, inv)
	err = root.ExecuteContext(ctx)
	debug, _ := root.PersistentFlags().GetBool("debug")
	if err != nil {
		return runner.RenderFatal(inv, usageError(err), stdout, stderr, debug)
	}
	if inv.SuccessExitSet {
		return inv.SuccessExitCode
	}
	return errs.ExitOK
}

// buildRoot is NewRootCmd with a malformed environment variable reported as invalid input
// instead of a panic: the configuration of every command is read when the tree is built.
func buildRoot() (root *cobra.Command, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := strings.TrimPrefix(fmt.Sprint(r), "openadrctl: ")
			err = errs.InvalidInput(errs.CodeInvalidInput, msg, nil)
		}
	}()
	return NewRootCmd(), nil
}

// usageError classifies what Cobra itself rejects (an unknown command or flag, a missing
// required flag, a value a flag cannot hold) as a usage error: exit code 2.
func usageError(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	return errs.New(errs.KindUsage, errs.CodeInvalidInput, err.Error(), err)
}
