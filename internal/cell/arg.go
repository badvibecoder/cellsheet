package cell

// Arg is one evaluated function argument. It is either a single value or a
// range of values, and keeping the two cases separate is what makes the
// range-versus-scalar rule in PHASE-1-SPEC.md §7.7 expressible:
//
//	=SUM(A1:A3)  tolerates text inside the range (it is skipped)
//	=SUM(A2, 1)  is #VALUE! when A2 is text
//
// At avoids allocating for the common single-value case.
type Arg struct {
	isRange bool
	single  Value
	rng     []Value
}

// Single wraps one value as an argument.
func Single(v Value) Arg { return Arg{single: v} }

// Range wraps a sequence of values as an argument.
func Range(vs []Value) Arg { return Arg{isRange: true, rng: vs} }

// IsRange reports whether the argument is a range.
func (a Arg) IsRange() bool { return a.isRange }

// Len is how many values the argument contributes.
func (a Arg) Len() int {
	if a.isRange {
		return len(a.rng)
	}
	return 1
}

// At returns the i'th value.
func (a Arg) At(i int) Value {
	if a.isRange {
		return a.rng[i]
	}
	return a.single
}

// Value returns the single value; for a range it returns the first element, so
// callers that want the whole range must use Len and At.
func (a Arg) Value() Value {
	if a.isRange {
		if len(a.rng) == 0 {
			return Empty()
		}
		return a.rng[0]
	}
	return a.single
}
