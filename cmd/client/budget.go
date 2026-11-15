// SPDX-License-Identifier: MIT

package client

import (
	"fmt"

	"github.com/otfabric/openadrctl/internal/errs"
	"github.com/otfabric/openadrctl/internal/oadr"
)

// The most one search keeps of what the VTN sends, page after page, before
// it ends with an error: far more than a VTN holds for one client, and a
// bound on the memory of a search whatever --timeout allows. --max-objects
// sets a lower bound on the count, defaultMaxObjects unless it is given (0:
// up to maxSearchObjects). They are variables for the tests.
var (
	maxSearchObjects        = 1_000_000
	maxSearchBytes    int64 = 1 << 30
	defaultMaxObjects       = 100_000
)

// budget counts the objects of one search, and their size as JSON.
type budget struct {
	max     int // --max-objects; 0 for none
	objects int
	bytes   int64
}

// newBudget is the budget of one search with --max-objects n.
func newBudget(n int) *budget { return &budget{max: n} }

// take counts v, and refuses it when it is one past a bound: the search
// ends with a protocol error, exit 6.
func (b *budget) take(v any) error {
	b.objects++
	b.bytes += int64(len(oadr.MustEncode(v)))
	switch {
	case b.max > 0 && b.objects > b.max:
		return errs.New(errs.KindProtocol, errs.CodeInvalidResponse,
			fmt.Sprintf("the VTN sent more than %d objects for one search (--max-objects): narrow the search", b.max), nil)
	case b.objects > maxSearchObjects:
		return errs.New(errs.KindProtocol, errs.CodeInvalidResponse,
			fmt.Sprintf("the VTN sent more than %d objects for one search, the most openadrctl keeps: narrow the search", maxSearchObjects), nil)
	case b.bytes > maxSearchBytes:
		return errs.New(errs.KindProtocol, errs.CodeInvalidResponse,
			fmt.Sprintf("the objects the VTN sent for one search take more than %d MiB as JSON, the most openadrctl keeps: narrow the search", maxSearchBytes>>20), nil)
	}
	return nil
}

// checkMaxObjects checks --max-objects.
func checkMaxObjects(n int) error {
	if n < 0 || n > maxSearchObjects {
		return errs.InvalidInput(errs.CodeInvalidInput, fmt.Sprintf("--max-objects is 0 to %d", maxSearchObjects), nil)
	}
	return nil
}
