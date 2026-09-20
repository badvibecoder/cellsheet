package cell

// Kind is what a cell value fundamentally is.
//
// Kind is not the same as "how it looks": a whole Number and a whole Currency
// both display without decimals, but they are different kinds because currency
// survives some operations and not others (PHASE-1-SPEC.md §7.4).
type Kind uint8

const (
	KindEmpty Kind = iota
	KindNumber
	KindCurrency
	KindText
	KindError
)

func (k Kind) String() string {
	switch k {
	case KindNumber:
		return "Number"
	case KindCurrency:
		return "Currency"
	case KindText:
		return "Text"
	case KindError:
		return "Error"
	default:
		return "Empty"
	}
}

// ErrorCode is the specific failure a cell holds.
type ErrorCode uint8

const (
	ErrNone ErrorCode = iota
	ErrValue
	ErrDivZero
	ErrNum
	ErrCirc
	ErrName
	ErrRef
	ErrInternal
)

// Display is the text shown in the cell.
func (e ErrorCode) Display() string {
	switch e {
	case ErrValue:
		return "#VALUE!"
	case ErrDivZero:
		return "#DIV/0!"
	case ErrNum:
		return "#NUM!"
	case ErrCirc:
		return "#CIRC!"
	case ErrName:
		return "#NAME?"
	case ErrRef:
		return "#REF!"
	case ErrInternal:
		return "#ERR!"
	default:
		return ""
	}
}

func (e ErrorCode) String() string { return e.Display() }

// Value is the typed content of a cell.
//
// Only one of Num, Str or Code is meaningful, selected by Kind. The
// constructors enforce that; do not build a Value by hand.
type Value struct {
	Kind Kind
	Num  Dec
	Str  string
	Code ErrorCode
}

// Empty is a cell with nothing in it.
func Empty() Value { return Value{Kind: KindEmpty} }

// Number is a plain quantity.
func Number(d Dec) Value { return Value{Kind: KindNumber, Num: d} }

// Currency is a quantity of US dollars.
func Currency(d Dec) Value { return Value{Kind: KindCurrency, Num: d} }

// Text is anything that is not a number: never used in arithmetic.
func Text(s string) Value { return Value{Kind: KindText, Str: s} }

// Error is a failed calculation.
func Error(code ErrorCode) Value { return Value{Kind: KindError, Code: code} }

// ZeroNumber is the number 0, used as a starting point for accumulation.
func ZeroNumber() Value { return Number(Zero()) }

// IsEmpty reports whether the cell has no content.
func (v Value) IsEmpty() bool { return v.Kind == KindEmpty }

// IsNumeric reports whether the value is a Number or Currency.
func (v Value) IsNumeric() bool { return v.Kind == KindNumber || v.Kind == KindCurrency }

// IsError reports whether the value is an error.
func (v Value) IsError() bool { return v.Kind == KindError }

// Skip reports whether the value is ignored when it appears inside a range.
// Empty cells and text are skipped; that is what makes =SUM(A1:A3) work when A2
// is a text header, while =SUM(A2, 1) is still #VALUE! (PHASE-1-SPEC.md §7.7).
func (v Value) Skip() bool { return v.Kind == KindEmpty || v.Kind == KindText }

// AsNumber returns the numeric value, treating Empty as zero.
func (v Value) AsNumber() (Dec, bool) {
	switch v.Kind {
	case KindNumber, KindCurrency:
		return v.Num, true
	case KindEmpty:
		return Zero(), true
	default:
		return Dec{}, false
	}
}

// Apply performs one arithmetic operation, implementing the type promotion
// table in PHASE-1-SPEC.md §7.4.
//
// The order of the checks matters: errors propagate first, then text is
// rejected, then Empty is treated as zero, and only then is arithmetic
// attempted. A text cell can therefore never produce a nonsense number.
func Apply(op Op, a, b Value) Value {
	if a.Kind == KindError {
		return a
	}
	if b.Kind == KindError {
		return b
	}
	if a.Kind == KindText || b.Kind == KindText {
		return Error(ErrValue)
	}
	av, _ := a.AsNumber()
	bv, _ := b.AsNumber()

	if (op == OpDiv || op == OpMod) && bv.IsZero() {
		return Error(ErrDivZero)
	}

	var (
		r  Dec
		ok bool
	)
	switch op {
	case OpAdd:
		r, ok = av.Add(bv)
	case OpSub:
		r, ok = av.Sub(bv)
	case OpMul:
		r, ok = av.Mul(bv)
	case OpDiv:
		r, ok = av.Div(bv)
	case OpMod:
		r, ok = decMod(av, bv)
	case OpPow:
		n, isInt := bv.Int64()
		if !isInt {
			return Error(ErrNum)
		}
		r, ok = av.Pow(n)
	default:
		return Error(ErrInternal)
	}
	if !ok {
		return Error(ErrNum)
	}
	return Value{Kind: Promote(op, a.Kind, b.Kind), Num: r}
}

// decMod is the remainder, with the sign of the divisor to match Excel's MOD.
func decMod(a, b Dec) (Dec, bool) {
	s := max(a.Scale, b.Scale)
	x, ok := a.Rescale(s)
	if !ok {
		return Dec{}, false
	}
	y, ok := b.Rescale(s)
	if !ok {
		return Dec{}, false
	}
	r := x.Mant % y.Mant
	if r != 0 && (r < 0) != (y.Mant < 0) {
		r += y.Mant
	}
	return Dec{Mant: r, Scale: s}, true
}

// Promote decides the kind of a result from the operation and its operands.
//
// In one sentence: currency survives addition, subtraction and scaling by a
// plain number, but not multiplication by currency or division by currency,
// because the units no longer make sense.
func Promote(op Op, a, b Kind) Kind {
	ac := a == KindCurrency
	bc := b == KindCurrency
	switch op {
	case OpAdd, OpSub:
		if ac || bc {
			return KindCurrency
		}
		return KindNumber
	case OpMul:
		// Currency x Currency is a squared quantity, not dollars.
		if ac != bc {
			return KindCurrency
		}
		return KindNumber
	case OpDiv:
		// Currency / Currency is a ratio.
		if ac && !bc {
			return KindCurrency
		}
		return KindNumber
	case OpMod:
		if ac {
			return KindCurrency
		}
		return KindNumber
	case OpPow:
		if ac && !bc {
			return KindCurrency
		}
		return KindNumber
	default:
		return KindNumber
	}
}

// Op is an arithmetic operator.
type Op uint8

const (
	OpAdd Op = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
)

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
	case OpMod:
		return "MOD"
	case OpPow:
		return "^"
	default:
		return "?"
	}
}

// Cell is one box in the grid: what was typed, and what it evaluates to.
//
// Source holds the formula text (including the leading "=") when the cell is a
// formula, or the literal text the user typed otherwise. Keeping the source is
// what lets the .cell file store formulas rather than cached answers, so a file
// can never contain a stale result (PHASE-1-SPEC.md §12.3).
type Cell struct {
	Source string
	Value  Value
}

// IsFormula reports whether the cell holds a formula.
//
// It delegates to the package-level check rather than testing for a leading '='
// itself. Duplicating that test here meant the "-=" form was recognised when a
// cell was typed but not when it was saved or re-indexed, so such a formula
// would be written to disk as a frozen value and would never recalculate again.
func (c Cell) IsFormula() bool { return IsFormula(c.Source) }
