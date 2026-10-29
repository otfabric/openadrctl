// SPDX-License-Identifier: MIT

// Package client is the client side of openadrctl: the commands of a VEN
// ("openadrctl ven") and of Business Logic ("openadrctl bl"), which talk to a
// VTN over its REST API.
package client

import (
	"context"
	"fmt"

	"github.com/otfabric/go-openadr/client"
	"github.com/spf13/cobra"

	"github.com/otfabric/openadrctl/internal/conn"
	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
	"github.com/otfabric/openadrctl/internal/runner"
)

// Role is the side of a client: a VEN or Business Logic.
type Role string

// The two roles of a client of a VTN.
const (
	VEN Role = "ven"
	BL  Role = "bl"
)

const envNote = `

With --spec 3.0.1 (or OPENADRCTL_SPEC) the VTN is one of OpenADR 3.0.1: --target is
TYPE=value, such as GROUP=group-1; resources are reached through their VEN (--ven); a
subscription names a program and notifies by webhook; notifiers, MQTT topics and resource
groups are not there.

The connection comes from flags or the environment: OPENADRCTL_URL, OPENADRCTL_CLIENT_ID,
OPENADRCTL_CLIENT_SECRET (or --client-secret-file), OPENADRCTL_TOKEN (or --token-file),
OPENADRCTL_CA, and so on. Without credentials or a token, no token is sent: that is how a
client reads a VTN that answers anonymous requests.`

// NewVENCmd returns the ven command group.
func NewVENCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ven",
		Short: "Talk to a VTN as a VEN",
		Long: `Commands of a Virtual End Node: read the programs and events meant for it, report on
events, manage its own VEN object and resources, subscribe to notifications and listen for
them. A VEN sees only what Business Logic granted it: a program or event with targets is
shown when the search names one of the VEN's targets (--target).` + envNote,
	}
	addGroups(cmd, VEN)
	return cmd
}

// NewBLCmd returns the bl command group.
func NewBLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bl",
		Short: "Talk to a VTN as Business Logic",
		Long: `Commands of Business Logic: create and change programs and events, read the reports
VENs send, manage VENs, resources and resource groups, subscribe to notifications and
listen for them. The VTN decides what a client may do by its token: Business Logic
commands with a VEN's credentials are answered 403.` + envNote,
	}
	addGroups(cmd, BL)
	return cmd
}

func addGroups(cmd *cobra.Command, role Role) {
	cmd.AddGroup(&cobra.Group{ID: "objects", Title: "Objects:"}, &cobra.Group{ID: "notify", Title: "Notifications:"})
	for _, c := range []*cobra.Command{programsCmd(role), eventsCmd(role), reportsCmd(role), vensCmd(role),
		resourcesCmd(role), resourceGroupsCmd(role)} {
		c.GroupID = "objects"
		cmd.AddCommand(c)
	}
	for _, c := range []*cobra.Command{subscriptionsCmd(role), newNotifiersCmd(role), newTopicsCmd(role), newListenCmd(role)} {
		c.GroupID = "notify"
		cmd.AddCommand(c)
	}
}

// Common are the flags of every client command.
type Common struct {
	conn.Config
	Format string `flag:"format" env:"OPENADRCTL_FORMAT" desc:"Output format: text, json or table"`
}

func newCommon() Common { return Common{Config: conn.Defaults(), Format: "text"} }

// use makes the revision of --spec the one objects are read and written in.
func (c *Common) use() {
	if c.Version().Supported() {
		oadr.SetRevision(c.Version())
	}
}

// api is a client of the VTN in a role.
type api struct {
	role Role
	ven  *client.VEN
	bl   *client.BL
	conn *conn.Conn
}

func (c *Common) connect(role Role) (*api, error) {
	cn, err := c.Open()
	if err != nil {
		return nil, err
	}
	a := &api{role: role, conn: cn}
	if role == VEN {
		a.ven, err = cn.VEN()
	} else {
		a.bl, err = cn.BL()
	}
	return a, err
}

// once is a client command with one result: it validates the flags,
// connects, runs within --timeout, and writes what run returns. A failure of
// run is classified (conn.Classify); write says whether run may change the
// VTN.
func once(cmd *cobra.Command, role Role, c *Common, validate func() error, write bool, run func(ctx context.Context, a *api) (any, error)) error {
	c.use()
	return runner.RunFormattedChecked(cmd, c.Format, func() error {
		if err := c.Validate(); err != nil {
			return err
		}
		if validate != nil {
			return validate()
		}
		return nil
	}, func(ctx context.Context) (any, error) {
		a, err := c.connect(role)
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, c.Timeout)
		defer cancel()
		v, err := run(ctx, a)
		if err != nil {
			return nil, conn.Classify(err, write)
		}
		return v, nil
	})
}

// confirmed refuses a command that changes the VTN without --yes.
func confirmed(yes bool, what string) error {
	if !yes {
		return errs.New(errs.KindUsage, errs.CodeConfirmationRequired, fmt.Sprintf("%s changes the VTN: confirm with --yes", what), nil)
	}
	return nil
}
