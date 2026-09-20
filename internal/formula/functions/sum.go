// Package functions holds the formula functions. One file per function.
//
// Adding a file here adds a function to the program: init() runs at startup and
// registers it. No other file anywhere needs to change. See
// PHASE-1-SPEC.md §14.4 for the walkthrough.
package functions

import (
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/formula/registry"
)

func init() {
	registry.Register(registry.Spec{
		Name:    "SUM",
		Summary: "Adds all the numbers in a range or list of values. Text and empty cells inside a range are ignored.",
		MinArgs: 1,
		MaxArgs: -1,
		Numeric: true,
		Fn:      sum,
	})
}

func sum(_ *registry.Ctx, args []cell.Arg) cell.Value {
	total := cell.ZeroNumber()
	for _, a := range args {
		for i := 0; i < a.Len(); i++ {
			v := a.At(i)
			if v.IsError() {
				return v
			}
			// Inside a range, empty cells and text are skipped. That is what
			// makes a subtotal over a column with a text header work.
			if a.IsRange() && v.Skip() {
				continue
			}
			total = cell.Apply(cell.OpAdd, total, v)
			if total.IsError() {
				return total
			}
		}
	}
	return total
}
