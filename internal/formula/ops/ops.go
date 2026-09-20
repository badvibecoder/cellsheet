// Package ops is the operator table: which tokens are operators, what they mean,
// and how tightly they bind.
//
// This is an extension point (PHASE-1-SPEC.md §14.2). Adding an operator means
// adding one row here, because the parser is precedence-driven rather than
// written as a fixed grammar. The operator's *semantics* live in the cell
// package, which owns type promotion.
package ops

import (
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/formula/ast"
	"github.com/badvibecoder/cellsheet/internal/formula/lexer"
)

// Binding powers, highest first, exactly as tabulated in PHASE-1-SPEC.md §7.6.
const (
	PrecComparison = 1
	PrecAdd        = 2
	PrecMul        = 3
	PrecUnary      = 4
	PrecPow        = 5
	PrecPercent    = 6
)

// Infix describes an operator that sits between two operands.
type Infix struct {
	Op     ast.Op
	CellOp cell.Op
	Prec   int
	Right  bool // right-associative
}

var infixTable = map[lexer.Kind]Infix{
	lexer.Plus:  {Op: ast.OpAdd, CellOp: cell.OpAdd, Prec: PrecAdd},
	lexer.Minus: {Op: ast.OpSub, CellOp: cell.OpSub, Prec: PrecAdd},
	lexer.Star:  {Op: ast.OpMul, CellOp: cell.OpMul, Prec: PrecMul},
	lexer.Slash: {Op: ast.OpDiv, CellOp: cell.OpDiv, Prec: PrecMul},
	// ^ is right-associative, so 2^3^2 is 2^(3^2) = 512, matching Excel.
	lexer.Caret: {Op: ast.OpPow, CellOp: cell.OpPow, Prec: PrecPow, Right: true},
	// Comparisons are lowest and yield 1 or 0.
	lexer.Eq: {Op: ast.OpEq, Prec: PrecComparison},
	lexer.Ne: {Op: ast.OpNe, Prec: PrecComparison},
	lexer.Lt: {Op: ast.OpLt, Prec: PrecComparison},
	lexer.Gt: {Op: ast.OpGt, Prec: PrecComparison},
	lexer.Le: {Op: ast.OpLe, Prec: PrecComparison},
	lexer.Ge: {Op: ast.OpGe, Prec: PrecComparison},
}

// LookupInfix returns the description of an infix operator token.
func LookupInfix(k lexer.Kind) (Infix, bool) {
	in, ok := infixTable[k]
	return in, ok
}

// Prefix describes a prefix (unary) operator.
type Prefix struct {
	Op   ast.Op
	Prec int
}

var prefixTable = map[lexer.Kind]Prefix{
	lexer.Minus: {Op: ast.OpNeg, Prec: PrecUnary},
	lexer.Plus:  {Op: ast.OpPos, Prec: PrecUnary},
}

// LookupPrefix returns the description of a prefix operator token.
func LookupPrefix(k lexer.Kind) (Prefix, bool) {
	p, ok := prefixTable[k]
	return p, ok
}

// Percent reports whether the token is the postfix percent operator.
func Percent(k lexer.Kind) bool { return k == lexer.Percent }

// astToCell maps a tree operator onto the arithmetic it performs. Comparison
// operators are absent: they are handled by the evaluator, which needs to
// compare rather than compute.
var astToCell = map[ast.Op]cell.Op{
	ast.OpAdd: cell.OpAdd,
	ast.OpSub: cell.OpSub,
	ast.OpMul: cell.OpMul,
	ast.OpDiv: cell.OpDiv,
	ast.OpPow: cell.OpPow,
}

// CellOp returns the arithmetic operation for a tree operator.
func CellOp(o ast.Op) (cell.Op, bool) {
	c, ok := astToCell[o]
	return c, ok
}

// Operators lists the registered operator spellings, for help text and tests.
func Operators() []string {
	out := make([]string, 0, len(infixTable)+1)
	for _, in := range infixTable {
		out = append(out, in.Op.String())
	}
	out = append(out, "%")
	return out
}
