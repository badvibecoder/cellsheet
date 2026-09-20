// Package cell defines what a cell can contain and how values combine.
//
// The central decision (PHASE-1-SPEC.md §7.2, decision D9) is that numbers are
// stored as an exact fixed-point decimal rather than a float. Floating point
// cannot represent 0.10 exactly, so $0.10 + $0.20 becomes 0.30000000000000004.
// For a program whose main job is budgets that is a defect, not a compatibility
// feature, so arithmetic here is exact.
package cell

import (
	"math"
	"math/big"
	"strings"
)

// Dec is an exact decimal: Mant x 10^-Scale.
//
//	50000.00 -> Mant: 5000000, Scale: 2
//	12.75    -> Mant: 1275,    Scale: 2
//	3        -> Mant: 3,       Scale: 0
//
// The zero value is the number 0.
type Dec struct {
	Mant  int64
	Scale int8
}

const (
	// MaxScale bounds the stored fractional digits, so that 10^Scale always
	// fits in an int64 with room for a mantissa.
	MaxScale = 18

	// divScale is the working precision for division: 1/3 becomes
	// 0.333333333333. Twelve places is well beyond the two that money needs
	// and well within an int64 for realistic magnitudes.
	divScale = 12
)

// pow10 returns 10^n for 0 <= n <= 18.
func pow10(n int) int64 {
	r := int64(1)
	for i := 0; i < n; i++ {
		r *= 10
	}
	return r
}

func pow10Big(n int) *big.Int {
	if n <= 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// mulOvf multiplies two int64s, reporting false on overflow.
func mulOvf(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, false
	}
	p := a * b
	if p/b != a {
		return 0, false
	}
	return p, true
}

// addOvf adds two int64s, reporting false on overflow.
func addOvf(a, b int64) (int64, bool) {
	s := a + b
	if (b > 0 && s < a) || (b < 0 && s > a) {
		return 0, false
	}
	return s, true
}

// NewDec builds a decimal from a mantissa and scale.
func NewDec(mant int64, scale int8) Dec {
	if scale < 0 {
		// Negative scales are normalised away: 5 x 10^2 == 500.
		for scale < 0 {
			m, ok := mulOvf(mant, 10)
			if !ok {
				return Dec{}
			}
			mant, scale = m, scale+1
		}
	}
	if scale > MaxScale {
		scale = MaxScale
	}
	return Dec{Mant: mant, Scale: scale}
}

// FromInt64 is the common case: a whole number.
func FromInt64(n int64) Dec { return Dec{Mant: n, Scale: 0} }

// Zero is the number 0.
func Zero() Dec { return Dec{} }

// ParseDec reads a plain decimal string: an optional sign, digits, an optional
// fractional part, and an optional exponent. It deliberately does NOT accept
// currency symbols or thousands separators; see Infer for that.
func ParseDec(s string) (Dec, bool) {
	if s == "" {
		return Dec{}, false
	}
	i := 0
	neg := false
	switch s[0] {
	case '+':
		i++
	case '-':
		neg = true
		i++
	}
	if i >= len(s) {
		return Dec{}, false
	}

	var mant int64
	digits := 0
	scale := 0
	seenDot := false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.':
			if seenDot {
				return Dec{}, false
			}
			seenDot = true
		case c >= '0' && c <= '9':
			m, ok := mulOvf(mant, 10)
			if !ok {
				return Dec{}, false
			}
			m2, ok := addOvf(m, int64(c-'0'))
			if !ok {
				return Dec{}, false
			}
			mant = m2
			digits++
			if seenDot {
				scale++
			}
		case c == 'e' || c == 'E':
			exp, ok := parseSmallInt(s[i+1:])
			if !ok {
				return Dec{}, false
			}
			scale -= exp
			i = len(s)
		default:
			return Dec{}, false
		}
	}
	if digits == 0 {
		return Dec{}, false
	}
	if neg {
		mant = -mant
	}
	d := NewDec(mant, int8(scale))
	if abs64(d.Mant) == 0 {
		// -0 is 0; and any residual scale is meaningless on zero.
		return Dec{}, true
	}
	return d, true
}

func parseSmallInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	i, neg := 0, false
	switch s[0] {
	case '+':
		i++
	case '-':
		neg = true
		i++
	}
	if i >= len(s) {
		return 0, false
	}
	n := 0
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > 1000 {
			return 0, false // far beyond any scale we support
		}
	}
	if neg {
		n = -n
	}
	return n, true
}

func abs64(v int64) int64 {
	if v < 0 {
		if v == math.MinInt64 {
			return math.MaxInt64 // saturate; callers treat this as overflow
		}
		return -v
	}
	return v
}

// Rescale returns the value expressed with exactly scale digits after the
// point, rounding half away from zero when reducing precision.
func (d Dec) Rescale(scale int8) (Dec, bool) {
	if scale < 0 {
		scale = 0
	}
	if scale > MaxScale {
		scale = MaxScale
	}
	if scale == d.Scale {
		return d, true
	}
	if scale < d.Scale {
		return d.roundTo(scale)
	}
	m, ok := mulPow10(d.Mant, int(scale-d.Scale))
	if !ok {
		return Dec{}, false
	}
	return Dec{Mant: m, Scale: scale}, true
}

func mulPow10(m int64, exp int) (int64, bool) {
	for i := 0; i < exp; i++ {
		var ok bool
		m, ok = mulOvf(m, 10)
		if !ok {
			return 0, false
		}
	}
	return m, true
}

// roundTo rounds to scale, half away from zero.
func (d Dec) roundTo(scale int8) (Dec, bool) {
	if scale >= d.Scale {
		return d.Rescale(scale)
	}
	drop := int(d.Scale - scale)
	div := pow10(drop)
	q := d.Mant / div
	r := d.Mant % div
	if r != 0 {
		twice := abs64(r) * 2
		if twice >= div {
			if d.Mant < 0 {
				q--
			} else {
				q++
			}
		}
	}
	return Dec{Mant: q, Scale: scale}, true
}

// Round rounds to scale decimal places, half away from zero.
func (d Dec) Round(scale int8) Dec {
	r, ok := d.roundTo(scale)
	if !ok {
		return d
	}
	return r
}

// Add returns d + o. ok is false on overflow.
func (d Dec) Add(o Dec) (Dec, bool) {
	s := max(d.Scale, o.Scale)
	a, ok := d.Rescale(s)
	if !ok {
		return Dec{}, false
	}
	b, ok := o.Rescale(s)
	if !ok {
		return Dec{}, false
	}
	m, ok := addOvf(a.Mant, b.Mant)
	if !ok {
		return Dec{}, false
	}
	return Dec{Mant: m, Scale: s}, true
}

// Sub returns d - o.
func (d Dec) Sub(o Dec) (Dec, bool) { return d.Add(o.Neg()) }

// Mul returns d x o. Scales add, so 1.5 x 1.25 is exactly 1.875.
func (d Dec) Mul(o Dec) (Dec, bool) {
	m, ok := mulOvf(d.Mant, o.Mant)
	if !ok {
		return Dec{}, false
	}
	s := int(d.Scale) + int(o.Scale)
	if s > MaxScale {
		// Reduce precision rather than lose the value entirely.
		return Dec{Mant: m, Scale: int8(s)}.roundTo(MaxScale)
	}
	return Dec{Mant: m, Scale: int8(s)}, true
}

// Div returns d / o computed to at least divScale decimal places and rounded
// half away from zero. ok is false when o is zero or the result is out of range.
func (d Dec) Div(o Dec) (Dec, bool) {
	if o.Mant == 0 {
		return Dec{}, false
	}
	s := max(int(d.Scale), int(o.Scale)) + 1
	if s < divScale {
		s = divScale
	}
	if s > MaxScale {
		s = MaxScale
	}
	// value = d.Mant x 10^(s - d.Scale + o.Scale) / o.Mant
	shift := s - int(d.Scale) + int(o.Scale)
	num := new(big.Int).SetInt64(d.Mant)
	if shift > 0 {
		num.Mul(num, pow10Big(shift))
	}
	den := big.NewInt(o.Mant)

	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() != 0 {
		twice := new(big.Int).Abs(r)
		twice.Lsh(twice, 1)
		if twice.CmpAbs(den) >= 0 {
			// Round away from zero, using the true sign of the quotient
			// rather than the sign of a possibly-zero q.
			if (num.Sign() < 0) != (den.Sign() < 0) {
				q.Sub(q, big.NewInt(1))
			} else {
				q.Add(q, big.NewInt(1))
			}
		}
	}
	if !q.IsInt64() {
		return Dec{}, false
	}
	return Dec{Mant: q.Int64(), Scale: int8(s)}, true
}

// Percent divides by 100, matching Excel's postfix % operator.
func (d Dec) Percent() (Dec, bool) { return d.Mul(NewDec(1, 2)) }

// Pow raises d to an integer power. Fractional and zero-negative exponents are
// rejected, giving #NUM! rather than a silently wrong answer.
func (d Dec) Pow(n int64) (Dec, bool) {
	if n == 0 {
		return FromInt64(1), true
	}
	if n < 0 {
		p, ok := d.Pow(-n)
		if !ok {
			return Dec{}, false
		}
		return FromInt64(1).Div(p)
	}
	result := FromInt64(1)
	base := d
	for n > 0 {
		if n&1 == 1 {
			var ok bool
			result, ok = result.Mul(base)
			if !ok {
				return Dec{}, false
			}
		}
		n >>= 1
		if n == 0 {
			break
		}
		var ok bool
		base, ok = base.Mul(base)
		if !ok {
			return Dec{}, false
		}
	}
	return result, true
}

// Neg returns -d.
func (d Dec) Neg() Dec { return Dec{Mant: -d.Mant, Scale: d.Scale} }

// Abs returns |d|.
func (d Dec) Abs() Dec {
	if d.Mant < 0 {
		return d.Neg()
	}
	return d
}

// Sign returns -1, 0 or 1.
func (d Dec) Sign() int {
	switch {
	case d.Mant < 0:
		return -1
	case d.Mant > 0:
		return 1
	default:
		return 0
	}
}

// IsZero reports whether the value is exactly zero.
func (d Dec) IsZero() bool { return d.Mant == 0 }

// IsNeg reports whether the value is negative.
func (d Dec) IsNeg() bool { return d.Mant < 0 }

// IsInteger reports whether the value has no fractional part. 10.00 is an
// integer; 10.5 is not.
func (d Dec) IsInteger() bool {
	if d.Scale == 0 {
		return true
	}
	return d.Mant%pow10(int(d.Scale)) == 0
}

// Int64 returns the value as an int64 when it is an exact integer.
func (d Dec) Int64() (int64, bool) {
	if !d.IsInteger() {
		return 0, false
	}
	if d.Scale == 0 {
		return d.Mant, true
	}
	return d.Mant / pow10(int(d.Scale)), true
}

// IsWhole is an alias for IsInteger, kept for readability at call sites that
// ask "does this display without a decimal point?".
func (d Dec) IsWhole() bool { return d.IsInteger() }

// scaledBig returns Mant x 10^(scale-Scale) as a big integer, used for
// comparison when rescaling to a common scale would overflow.
func (d Dec) scaledBig(scale int) *big.Int {
	m := big.NewInt(d.Mant)
	if e := scale - int(d.Scale); e > 0 {
		m.Mul(m, pow10Big(e))
	}
	return m
}

// Cmp compares d and o: -1 if d < o, 0 if equal, +1 if d > o.
func (d Dec) Cmp(o Dec) int {
	s := max(int(d.Scale), int(o.Scale))
	if a, ok := d.Rescale(int8(s)); ok {
		if b, ok2 := o.Rescale(int8(s)); ok2 {
			switch {
			case a.Mant < b.Mant:
				return -1
			case a.Mant > b.Mant:
				return 1
			default:
				return 0
			}
		}
	}
	return d.scaledBig(s).Cmp(o.scaledBig(s))
}

// Less reports whether d < o.
func (d Dec) Less(o Dec) bool { return d.Cmp(o) < 0 }

// Equal reports whether d == o.
func (d Dec) Equal(o Dec) bool { return d.Cmp(o) == 0 }

// Float returns the nearest float64. For display and approximation only; never
// used to compute a result that gets stored.
func (d Dec) Float() float64 {
	f := float64(d.Mant)
	for i := int8(0); i < d.Scale; i++ {
		f /= 10
	}
	return f
}

// String renders the decimal in plain form with trailing zeros removed:
// 5750.25, 450, 0.5. No thousands separators, no currency symbol.
func (d Dec) String() string {
	if d.Mant == 0 {
		return "0"
	}
	neg := d.Mant < 0
	digits := uitoa(abs64(d.Mant))
	scale := int(d.Scale)
	if scale == 0 {
		if neg {
			return "-" + digits
		}
		return digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	intPart := digits[:len(digits)-scale]
	fracPart := strings.TrimRight(digits[len(digits)-scale:], "0")
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString(intPart)
	if fracPart != "" {
		b.WriteByte('.')
		b.WriteString(fracPart)
	}
	return b.String()
}

func uitoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
