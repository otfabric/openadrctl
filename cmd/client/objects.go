// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openadr "github.com/otfabric/go-openadr"
	"github.com/otfabric/go-openadr/client"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/runner"
)

// noun is a kind of object and what a role may do with it. A nil operation
// is one the role does not have, and gets no command.
type noun[T oadr.Object, Q any] struct {
	kind    *oadr.Kind
	use     string
	aliases []string
	short   string
	long    string

	list   func(*api, context.Context, Q) ([]T, error)
	get    func(*api, context.Context, string) (T, error)
	create func(*api, context.Context, T) (T, error)
	update func(*api, context.Context, T) (T, error)
	del    func(*api, context.Context, string) (T, error)

	// parent is the flag of the object through which one is reached by
	// ID, where a revision reaches it so: the VEN of a resource in OpenADR
	// 3.0.1. getOf and delOf then take its value before the ID.
	parent       string
	parentUsage  string
	getOf, delOf func(a *api, ctx context.Context, parent, id string) (T, error)

	// query registers the filters of a search and returns how to make
	// the query of a page.
	query func(*cobra.Command) func(client.Page) Q
	// fields registers the flags that set properties of an object, and
	// returns how to apply those that were given.
	fields func(*cobra.Command) func(cmd *cobra.Command, v *T) error
	// sensitive says a create or update needs --yes: VENs act on it.
	sensitive bool
	// examples per verb.
	examples map[string]string
}

func (n noun[T, Q]) command(role Role, extra ...*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: n.use, Aliases: n.aliases, Short: n.short, Long: n.long}
	if n.list != nil {
		cmd.AddCommand(n.listCmd(role))
	}
	if n.get != nil {
		cmd.AddCommand(n.getCmd(role))
	}
	if n.create != nil {
		cmd.AddCommand(n.writeCmd(role, true))
	}
	if n.update != nil {
		cmd.AddCommand(n.writeCmd(role, false))
	}
	if n.del != nil {
		cmd.AddCommand(n.deleteCmd(role))
	}
	cmd.AddCommand(extra...)
	return cmd
}

// parentFlag registers the flag of n.parent, if n has one and the command
// does not already, and returns how to read it.
func (n noun[T, Q]) parentFlag(cmd *cobra.Command) func() string {
	if n.parent == "" {
		return func() string { return "" }
	}
	if cmd.Flags().Lookup(n.parent) == nil {
		cmd.Flags().String(n.parent, "", n.parentUsage)
	}
	return func() string {
		v, _ := cmd.Flags().GetString(n.parent)
		return v
	}
}

func (n noun[T, Q]) fetch(a *api, ctx context.Context, parent, id string) (T, error) {
	if n.getOf != nil {
		return n.getOf(a, ctx, parent, id)
	}
	return n.get(a, ctx, id)
}

func (n noun[T, Q]) remove(a *api, ctx context.Context, parent, id string) (T, error) {
	if n.delOf != nil {
		return n.delOf(a, ctx, parent, id)
	}
	return n.del(a, ctx, id)
}

type listConfig struct {
	Common
	Skip  int `flag:"skip" desc:"Skip this many objects (one page only)"`
	Limit int `flag:"limit" desc:"Ask for at most this many objects, 50 at most (one page only); default: all, page by page"`
}

func (n noun[T, Q]) listCmd(role Role) *cobra.Command {
	c := listConfig{Common: newCommon()}
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   fmt.Sprintf("Search the %s", n.kind.Plural),
		Long: fmt.Sprintf(`Search the %s the VTN shows this client. Without --skip or --limit every page is
fetched, 50 objects at a time; with either, one page.`, n.kind.Plural),
		Example: n.examples["list"],
		Args:    cobra.NoArgs,
	}
	query := n.query(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return once(cmd, role, &c.Common, func() error {
			if c.Skip < 0 || c.Limit < 0 || c.Limit > 50 {
				return errs.InvalidInput(errs.CodeInvalidInput, "--skip must not be negative, and --limit is 0 to 50", nil)
			}
			return nil
		}, false, func(ctx context.Context, a *api) (any, error) {
			var items []T
			if cmd.Flags().Changed("skip") || cmd.Flags().Changed("limit") {
				var err error
				q := query(client.Page{Skip: c.Skip, Limit: c.Limit})
				if err := retargetQuery(c.Version(), &q); err != nil {
					return nil, err
				}
				if items, err = n.list(a, ctx, q); err != nil {
					return nil, err
				}
			} else {
				for v, err := range client.Each(ctx, func(ctx context.Context, p client.Page) ([]T, error) {
					q := query(p)
					if err := retargetQuery(c.Version(), &q); err != nil {
						return nil, err
					}
					return n.list(a, ctx, q)
				}) {
					if err != nil {
						return nil, err
					}
					items = append(items, v)
				}
			}
			out := &oadr.Many{Kind: n.kind, Values: make([]any, 0, len(items))}
			for _, v := range items {
				out.Values = append(out.Values, v)
			}
			return out, nil
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

func (n noun[T, Q]) getCmd(role Role) *cobra.Command {
	c := newCommon()
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   fmt.Sprintf("Read a %s by its ID", n.kind.Name),
		Example: n.examples["get"],
		Args:    cobra.ExactArgs(1),
	}
	parent := n.parentFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return once(cmd, role, &c, func() error { return checkID(args[0]) }, false, func(ctx context.Context, a *api) (any, error) {
			v, err := n.fetch(a, ctx, parent(), args[0])
			if err != nil {
				return nil, err
			}
			return &oadr.One{Kind: n.kind, Value: v}, nil
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

type writeConfig struct {
	Common
	File   string `flag:"file" desc:"The object as JSON (of the revision of --spec), - for stdin; flags change what it says"`
	DryRun bool   `flag:"dry-run" desc:"Validate and print the object that would be sent; send nothing"`
	Yes    bool   `flag:"yes" desc:"Confirm the change"`
}

func (n noun[T, Q]) writeCmd(role Role, create bool) *cobra.Command {
	c := writeConfig{Common: newCommon()}
	verb, use, args := "update", "update <id>", cobra.ExactArgs(1)
	short := fmt.Sprintf("Change a %s: read it, apply --file and the flags, send it back", n.kind.Name)
	if create {
		verb, use, args = "create", "create", cobra.NoArgs
		short = fmt.Sprintf("Create a %s from flags or a JSON file", n.kind.Name)
	}
	long := short + `.

The object is validated against the OpenADR schema before anything is sent; one that
is invalid exits 3 and nothing is sent. --dry-run prints it instead of sending it.`
	if !create {
		long += `

An update replaces the whole object: properties the flags do not name keep the values the
VTN has, and --file replaces them all.`
	}
	if n.sensitive {
		long += "\n\nVENs act on " + n.kind.Plural + ": confirm with --yes."
	}
	cmd := &cobra.Command{Use: use, Short: short, Long: long, Example: n.examples[verb], Args: args}
	apply := n.fields(cmd)
	parent := n.parentFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		build := func(ctx context.Context, a *api) (T, error) {
			var v T
			if !create {
				cur, err := n.fetch(a, ctx, parent(), args[0])
				if err != nil {
					return v, err
				}
				v = cur
			}
			if c.File != "" {
				fromFile, err := oadr.ReadFile[T](c.File, cmd.InOrStdin())
				if err != nil {
					return v, err
				}
				if !create {
					setMetadata(&fromFile, metadataOf(v))
				}
				v = fromFile
			}
			if err := apply(cmd, &v); err != nil {
				return v, err
			}
			if err := retarget(c.Version(), &v); err != nil {
				return v, err
			}
			if val, ok := any(v).(interface{ Validate() error }); ok {
				if err := val.Validate(); err != nil {
					return v, err
				}
			}
			return v, nil
		}
		if c.DryRun && create {
			// Nothing to read from the VTN: no connection either.
			c.use()
			return runner.RunFormattedChecked(cmd, c.Format, func() error { return nil }, func(ctx context.Context) (any, error) {
				v, err := build(ctx, nil)
				if err != nil {
					return nil, classifyLocal(err)
				}
				return &oadr.One{Kind: n.kind, Value: v}, nil
			})
		}
		return once(cmd, role, &c.Common, func() error {
			if !create {
				if err := checkID(args[0]); err != nil {
					return err
				}
			}
			if n.sensitive && !c.DryRun {
				return confirmed(c.Yes, n.use+" "+verb)
			}
			return nil
		}, !c.DryRun, func(ctx context.Context, a *api) (any, error) {
			v, err := build(ctx, a)
			if err != nil {
				return nil, err
			}
			if c.DryRun {
				return &oadr.One{Kind: n.kind, Value: v}, nil
			}
			send := n.update
			if create {
				send = n.create
			}
			out, err := send(a, ctx, v)
			if err != nil {
				return nil, err
			}
			return &oadr.One{Kind: n.kind, Value: out, Action: verb + "d"}, nil
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	_ = cmd.MarkFlagFilename("file", "json")
	if !n.sensitive {
		// Accepted, so that a script may confirm every change, but not needed.
		_ = cmd.Flags().MarkHidden("yes")
	}
	return cmd
}

type deleteConfig struct {
	Common
	Yes bool `flag:"yes" desc:"Confirm the deletion"`
}

func (n noun[T, Q]) deleteCmd(role Role) *cobra.Command {
	c := deleteConfig{Common: newCommon()}
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   fmt.Sprintf("Delete a %s", n.kind.Name),
		Long:    fmt.Sprintf("Delete a %s. The VTN answers with the object as it last was, which is printed.", n.kind.Name),
		Example: n.examples["delete"],
		Args:    cobra.ExactArgs(1),
	}
	parent := n.parentFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return once(cmd, role, &c.Common, func() error {
			if err := confirmed(c.Yes, n.use+" delete"); err != nil {
				return err
			}
			return checkID(args[0])
		}, true, func(ctx context.Context, a *api) (any, error) {
			v, err := n.remove(a, ctx, parent(), args[0])
			if err != nil {
				return nil, err
			}
			return &oadr.One{Kind: n.kind, Value: v, Action: "deleted"}, nil
		})
	}
	runner.RegisterCommandCfg(cmd, &c)
	return cmd
}

func checkID(id string) error {
	if !openadr.ValidID(id) {
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("%q is not an object ID: 1 to 128 characters of a-z, A-Z, 0-9, _ and -", id), nil)
	}
	return nil
}

// classifyLocal classifies an error of building an object, with no VTN.
func classifyLocal(err error) error {
	if _, ok := errs.As(err); ok {
		return err
	}
	var valErr *openadr.ValidationError
	if errors.As(err, &valErr) {
		// With the property at fault, as for an object a VTN was to get.
		return conn.Classify(err, false)
	}
	return errs.InvalidInput(errs.CodeInvalidObject, strings.TrimPrefix(err.Error(), "openadr: ")+" (nothing was sent)", err)
}

func metadataOf[T oadr.Object](v T) openadr.Metadata { return oadr.MetadataOf(v) }

// setMetadata gives an object read from a file the identity of the one it
// replaces.
func setMetadata[T oadr.Object](v *T, m openadr.Metadata) {
	switch p := any(v).(type) {
	case *openadr.Program:
		p.Metadata = m
	case *openadr.Event:
		p.Metadata = m
	case *openadr.Report:
		p.Metadata = m
	case *openadr.VEN:
		p.Metadata = m
	case *openadr.Resource:
		p.Metadata = m
	case *openadr.Subscription:
		p.Metadata = m
	case *openadr.ResourceGroup:
		p.Metadata = m
	}
}
