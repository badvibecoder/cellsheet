package cell

import "strings"

// FormatCurrency renders a decimal as US dollars: always two decimal places,
// always thousands separators, sign in front of the dollar sign.
//
//	50000   -> $50,000.00
//	1250    -> $1,250.00
//	-500    -> -$500.00
//	0.5     -> $0.50
//
// Display only. The stored value keeps its full precision.
func FormatCurrency(d Dec) string {
	r := d.Round(2)
	if r.Scale != 2 {
		// Magnitudes so large that rescaling overflowed. Fall back to the plain
		// form rather than printing something wrong.
		if r.Sign() < 0 {
			return "-$" + r.Abs().String()
		}
		return "$" + r.String()
	}
	neg := r.Mant < 0
	m := abs64(r.Mant)
	whole := m / 100
	frac := m % 100

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteByte('$')
	b.WriteString(group3(uitoa(whole)))
	b.WriteByte('.')
	b.WriteByte(byte('0' + frac/10))
	b.WriteByte(byte('0' + frac%10))
	return b.String()
}

// FormatNumber renders a plain number: no currency symbol, no thousands
// separators, trailing zeros removed.
//
//	450    -> 450
//	12.75  -> 12.75
//	10.00  -> 10
//	5737.5 -> 5737.5
//
// The absence of grouping is deliberate and matches the approved mock, where
// $50,000.00 is grouped but 5737.50 is not (PHASE-1-SPEC.md §7.3).
func FormatNumber(d Dec) string {
	return d.String()
}

// Display is the text to draw in a cell.
func (v Value) Display() string {
	switch v.Kind {
	case KindNumber:
		return FormatNumber(v.Num)
	case KindCurrency:
		return FormatCurrency(v.Num)
	case KindText:
		return v.Str
	case KindError:
		return v.Code.Display()
	default:
		return ""
	}
}

// AlignRight reports the default alignment for a kind. Decision D11: text is
// left-aligned and numbers are right-aligned, the Excel convention.
func (v Value) AlignRight() bool { return v.IsNumeric() }

// AlignCenter reports whether the value is centred; errors are.
func (v Value) AlignCenter() bool { return v.IsError() }

// group3 inserts thousands separators into a run of digits.
func group3(digits string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	b.Grow(n + n/3)
	pre := n % 3
	if pre > 0 {
		b.WriteString(digits[:pre])
	}
	for i := pre; i < n; i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}
