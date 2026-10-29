// SPDX-License-Identifier: MIT

// Package cmd is the command tree of openadrctl.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	openadr "github.com/otfabric/go-openadr"

	"github.com/otfabric/openadrctl/cmd/client"
	"github.com/otfabric/openadrctl/cmd/vtn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/runner"
)

// buildMeta holds link-time metadata set from main via SetBuildMeta.
var buildMeta struct {
	version, tag, commit, buildDate string
}

// SetBuildMeta is called from main before Execute; values come from ldflags -X main.*.
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version = version
	buildMeta.tag = tag
	buildMeta.commit = commit
	buildMeta.buildDate = buildDate
}

// NewRootCmd builds the command tree. Every call returns a fresh tree, with the flags of
// every command at the defaults the environment gives them.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "openadrctl",
		Short: "OpenADR 3 command-line VEN, Business Logic and VTN",
		Long: `openadrctl speaks OpenADR 3.1.0, the protocol between a utility or aggregator (the VTN and
its Business Logic) and the sites that respond to demand-response signals (VENs), in every
role.

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
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of openadrctl",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			out := cmd.OutOrStdout()
			ver := strings.TrimSpace(buildMeta.version)
			if ver == "" {
				ver = "(unknown)"
			}
			_, _ = fmt.Fprintf(out, "openadrctl version: %s\n", ver)
			if s := strings.TrimSpace(buildMeta.tag); s != "" {
				_, _ = fmt.Fprintf(out, "tag:         %s\n", s)
			}
			if s := strings.TrimSpace(buildMeta.commit); s != "" {
				_, _ = fmt.Fprintf(out, "commit:      %s\n", s)
			}
			if s := strings.TrimSpace(buildMeta.buildDate); s != "" {
				_, _ = fmt.Fprintf(out, "build date:  %s\n", s)
			}
			_, _ = fmt.Fprintf(out, "openadr:     %s (go-openadr %s)\n", openadr.Version310, goOpenADRVersion())
			_, _ = fmt.Fprintf(out, "go version:  %s\n", runtime.Version())
			_, _ = fmt.Fprintf(out, "platform:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for openadrctl.

To load completions:

  bash:       source <(openadrctl completion bash)
  zsh:        openadrctl completion zsh > "${fpath[1]}/_openadrctl"
  fish:       openadrctl completion fish | source
  powershell: openadrctl completion powershell | Out-String | Invoke-Expression`,
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
