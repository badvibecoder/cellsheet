// Package ast is the syntax tree for formulas. It deliberately depends only on
// the cell package, so the parser can be tested without a workbook.
package ast

import "github.com/badvibecoder/cellsheet/internal/cell"

// Expr is any formula expression.
type Expr interface{ isExpr() }

// Literal is a number, currency amount or quoted string.
type Literal struct{ Value cell.Value }

// Name is an identifier that is not a cell reference, e.g. a typo like SUMM.
// It evaluates to #NAME? unless it is a known constant.
type Name struct{ Ident string }

// Ref is a cell reference. Sheet is empty for the sheet that owns the formula.
// Row and Col are zero-based, so A1 is Ref{Row: 0, Col: 0}.
type Ref struct {
	Sheet string
	Row   int
	Col   int
}

// Range is a rectangular range. From and To are normalised so that From is
// always the top-left corner.
type Range struct {
	From Ref
	To   Ref
}

// Unary is a prefix or postfix operator applied to one operand.
type Unary struct {
	Op    Op
	X     Expr
	IsPre bool
}

// Binary is an infix operator.
type Binary struct {
	Op   Op
	X, Y Expr
}

// Call is a function invocation.
type Call struct {
	Name string
	Args []Expr
}

// Op identifies an operator. The zero value is invalid so that a missing
// operator is detectable.
type Op uint8

const (
	OpInvalid Op = iota

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpPow
	OpPercent // postfix
	OpNeg     // unary minus
	OpPos     // unary plus

	OpEq
	OpNe
	OpLt
	OpGt
	OpLe
	OpGe
)

func (Op) isExpr() {}

func (Literal) isExpr() {}
func (Name) isExpr()    {}
func (Ref) isExpr()     {}
func (Range) isExpr()   {}
func (*Unary) isExpr()  {}
func (*Binary) isExpr() {}
func (*Call) isExpr()   {}

// String renders an operator as it is written.
func (o Op) String() string {
	switch o {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpPow:
		return "^"
	case OpPercent:
		return "%"
	case OpNeg:
		return "-"
	case OpPos:
		return "+"
	case OpEq:
		return "="
	case OpNe:
		return "<>"
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpLe:
		return "<="
	case OpGe:
		return ">="
	default:
		return "?"
	}
}

// IsComparison reports whether the operator yields a truth value (1 or 0).
func (o Op) IsComparison() bool {
	return o >= OpEq && o <= OpGe
}
