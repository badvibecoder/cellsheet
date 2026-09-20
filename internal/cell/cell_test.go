package cell

import (
	"math"
	"testing"
)

func dec(t *testing.T, s string) Dec {
	t.Helper()
	d, ok := ParseDec(s)
	if !ok {
		t.Fatalf("ParseDec(%q) failed", s)
	}
	return d
}

// TestDecimalExactness is the reason this package exists: money arithmetic must
// not drift the way floating point does.
func TestDecimalExactness(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"0.1", "0.2", "0.3"},
		{"1.5", "1.25", "2.75"},
		{"0.001", "0.002", "0.003"},
		{"50000.00", "1250.00", "51250"},
	}
	for _, c := range cases {
		got, ok := dec(t, c.a).Add(dec(t, c.b))
		if !ok {
			t.Fatalf("%s + %s overflowed", c.a, c.b)
		}
		if got.String() != c.want {
			t.Errorf("%s + %s = %s, want %s", c.a, c.b, got.String(), c.want)
		}
	}
	// The float64 behaviour we are avoiding, for the record.
	if 0.1+0.2 == 0.3 {
		t.Log("note: float64 0.1+0.2 happened to compare equal here")
	}
}

func TestDecimalArithmetic(t *testing.T) {
	cases := []struct {
		op       Op
		a, b     string
		want     string
		wantFail bool
	}{
		{OpAdd, "12.75", "450", "462.75", false},
		{OpSub, "1", "2", "-1", false},
		{OpMul, "1.5", "1.25", "1.875", false},
		{OpMul, "450", "12.75", "5737.5", false},
		{OpDiv, "10", "4", "2.5", false},
		{OpDiv, "1", "3", "0.333333333333", false},
		{OpDiv, "1", "0", "", true},
		{OpDiv, "10", "2", "5", false},
		{OpMod, "10", "3", "1", false},
		{OpMod, "-3", "2", "1", false}, // Excel's MOD takes the divisor's sign
		{OpMod, "3", "-2", "-1", false},
		{OpPow, "2", "10", "1024", false},
		{OpPow, "2", "-2", "0.25", false},
		{OpSub, "0.3", "0.1", "0.2", false},
	}
	for _, c := range cases {
		a := Number(dec(t, c.a))
		b := Number(dec(t, c.b))
		got := Apply(c.op, a, b)
		if c.wantFail {
			if !got.IsError() {
				t.Errorf("%s %s %s: want error, got %s", c.a, c.op, c.b, got.Display())
			}
			continue
		}
		if got.IsError() {
			t.Errorf("%s %s %s: unexpected %s", c.a, c.op, c.b, got.Display())
			continue
		}
		if got.Display() != c.want {
			t.Errorf("%s %s %s = %s, want %s", c.a, c.op, c.b, got.Display(), c.want)
		}
	}
}

func TestDecimalRoundTrip(t *testing.T) {
	for _, s := range []string{"0", "1", "-1", "450", "12.75", "-0.5", "1000000", "0.000001"} {
		d := dec(t, s)
		back, ok := ParseDec(d.String())
		if !ok {
			t.Fatalf("re-parsing %q failed", d.String())
		}
		if !d.Equal(back) {
			t.Errorf("round trip of %s gave %s", s, back.String())
		}
	}
}

func TestDecimalCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1", "2", -1},
		{"2", "1", 1},
		{"1.0", "1", 0},
		{"-1", "1", -1},
		{"0.30", "0.3", 0},
	}
	for _, c := range cases {
		if got := dec(t, c.a).Cmp(dec(t, c.b)); got != c.want {
			t.Errorf("Cmp(%s,%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestInferTable walks every row of PHASE-1-SPEC.md §7.3.
func TestInferTable(t *testing.T) {
	cases := []struct {
		in   string
		kind Kind
		disp string
	}{
		{"", KindEmpty, ""},
		{"   ", KindEmpty, ""},
		{"$50000", KindCurrency, "$50,000.00"},
		{"$50,000", KindCurrency, "$50,000.00"},
		{"$1,250.00", KindCurrency, "$1,250.00"},
		{"-$500", KindCurrency, "-$500.00"},
		{"$-500", KindCurrency, "-$500.00"},
		{"$0.5", KindCurrency, "$0.50"},
		{"$.50", KindCurrency, "$0.50"},
		{"$0", KindCurrency, "$0.00"},
		{"450", KindNumber, "450"},
		{"-3", KindNumber, "-3"},
		{"12.75", KindNumber, "12.75"},
		{"50,000", KindNumber, "50000"},
		{"1e3", KindNumber, "1000"},
		{"10.00", KindNumber, "10"},
		{"  450  ", KindNumber, "450"},
		{"Operations", KindText, "Operations"},
		{"YES", KindText, "YES"},
		{"TRUE", KindText, "TRUE"},
		{"50%", KindText, "50%"},
		{"(500)", KindText, "(500)"},
		{"3/4/2026", KindText, "3/4/2026"},
		{"1,00", KindText, "1,00"},
		{"$", KindText, "$"},
		{"abc123", KindText, "abc123"},
	}
	for _, c := range cases {
		got := Infer(c.in)
		if got.Kind != c.kind {
			t.Errorf("Infer(%q).Kind = %v, want %v", c.in, got.Kind, c.kind)
		}
		if d := got.Display(); d != c.disp {
			t.Errorf("Infer(%q).Display() = %q, want %q", c.in, d, c.disp)
		}
	}
}

func TestInferPreservesTextVerbatim(t *testing.T) {
	got := Infer("  hello  ")
	if got.Kind != KindText || got.Str != "  hello  " {
		t.Errorf("text should be stored verbatim, got %q (%v)", got.Str, got.Kind)
	}
}

// TestPromotionTable walks PHASE-1-SPEC.md §7.4.
func TestPromotionTable(t *testing.T) {
	type tc struct {
		op   Op
		a, b Kind
		want Kind
	}
	cases := []tc{
		{OpAdd, KindNumber, KindNumber, KindNumber},
		{OpAdd, KindCurrency, KindCurrency, KindCurrency},
		{OpAdd, KindCurrency, KindNumber, KindCurrency},
		{OpAdd, KindNumber, KindCurrency, KindCurrency},
		{OpSub, KindCurrency, KindNumber, KindCurrency},
		{OpMul, KindNumber, KindNumber, KindNumber},
		{OpMul, KindCurrency, KindNumber, KindCurrency},
		{OpMul, KindNumber, KindCurrency, KindCurrency},
		{OpMul, KindCurrency, KindCurrency, KindNumber},
		{OpDiv, KindNumber, KindNumber, KindNumber},
		{OpDiv, KindCurrency, KindNumber, KindCurrency},
		{OpDiv, KindCurrency, KindCurrency, KindNumber},
		{OpDiv, KindNumber, KindCurrency, KindNumber},
		{OpMod, KindCurrency, KindCurrency, KindCurrency},
		{OpPow, KindCurrency, KindNumber, KindCurrency},
		// Empty behaves as a number in promotion.
		{OpAdd, KindCurrency, KindEmpty, KindCurrency},
		{OpMul, KindCurrency, KindEmpty, KindCurrency},
	}
	for _, c := range cases {
		if got := Promote(c.op, c.a, c.b); got != c.want {
			t.Errorf("Promote(%s, %v, %v) = %v, want %v", c.op, c.a, c.b, got, c.want)
		}
	}
}

func TestApplyErrorsAndEdges(t *testing.T) {
	txt := Text("Operations")
	num := Number(FromInt64(450))
	cur := Currency(FromInt64(50000))
	empty := Empty()
	boom := Error(ErrDivZero)

	// Text anywhere in arithmetic is #VALUE!.
	if got := Apply(OpAdd, txt, num); got.Code != ErrValue {
		t.Errorf("text + number = %s, want #VALUE!", got.Display())
	}
	if got := Apply(OpAdd, num, txt); got.Code != ErrValue {
		t.Errorf("number + text = %s, want #VALUE!", got.Display())
	}
	// Errors propagate.
	if got := Apply(OpAdd, boom, num); got.Code != ErrDivZero {
		t.Errorf("error propagates: got %s", got.Display())
	}
	// Empty is zero.
	if got := Apply(OpAdd, empty, num); got.Display() != "450" {
		t.Errorf("empty + 450 = %s, want 450", got.Display())
	}
	// Division by zero, including an empty denominator.
	if got := Apply(OpDiv, num, empty); got.Code != ErrDivZero {
		t.Errorf("450 / empty = %s, want #DIV/0!", got.Display())
	}
	// Currency promotion end to end.
	if got := Apply(OpAdd, cur, num); got.Kind != KindCurrency || got.Display() != "$50,450.00" {
		t.Errorf("$50000 + 450 = %s (%v)", got.Display(), got.Kind)
	}
	if got := Apply(OpDiv, cur, num); got.Kind != KindCurrency {
		t.Errorf("$50000 / 450 should stay currency, got %v", got.Kind)
	}
	if got := Apply(OpMul, cur, cur); got.Kind != KindNumber {
		t.Errorf("$ x $ should be a plain number, got %v", got.Kind)
	}
}

func TestValueSkip(t *testing.T) {
	if !Empty().Skip() || !Text("x").Skip() {
		t.Error("empty and text must be skipped inside ranges")
	}
	if Number(FromInt64(1)).Skip() || Currency(FromInt64(1)).Skip() {
		t.Error("numbers must not be skipped")
	}
}

func TestFormatCurrency(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50000", "$50,000.00"},
		{"1250", "$1,250.00"},
		{"-500", "-$500.00"},
		{"0.5", "$0.50"},
		{"0", "$0.00"},
		{"1000000", "$1,000,000.00"},
		{"999", "$999.00"},
		{"1000", "$1,000.00"},
		{"1234567.891", "$1,234,567.89"},
		{"0.005", "$0.01"}, // half away from zero
	}
	for _, c := range cases {
		got := FormatCurrency(dec(t, c.in))
		if got != c.want {
			t.Errorf("FormatCurrency(%s) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestDecOverflowIsReported(t *testing.T) {
	// The largest representable magnitude, times ten, must fail rather than wrap.
	big := NewDec(math.MaxInt64, 0)
	if _, ok := big.Mul(FromInt64(10)); ok {
		t.Error("overflow must be reported, not wrapped")
	}
}

// TestSubCentCurrencyIsNotLossy is the bug the fuzzer found.
//
// Currency always displays two decimals, so $0.0001 shows as "$0.00". The value
// itself is exact and must stay exact; what must never happen is a cell being
// reconstructed from that display, because the amount would silently become
// zero the next time it was committed.
func TestSubCentCurrencyIsNotLossy(t *testing.T) {
	v := Infer("$0.0001")
	if v.Kind != KindCurrency {
		t.Fatalf("kind = %v, want Currency", v.Kind)
	}
	if want := NewDec(1, 4); !v.Num.Equal(want) {
		t.Fatalf("stored value = %s, want %s", v.Num.String(), want.String())
	}
	// The display is correct, and correctly lossy.
	if got := v.Display(); got != "$0.00" {
		t.Errorf("display = %q, want $0.00", got)
	}
	// Rounding to the displayed precision gives the displayed value, which is
	// the strongest statement that can be made about a currency display.
	if !v.Num.Round(2).Equal(Zero()) {
		t.Errorf("Round(2) = %s, want 0", v.Num.Round(2).String())
	}
	// A larger sub-cent amount rounds rather than truncating.
	if got := Infer("$0.005").Display(); got != "$0.01" {
		t.Errorf("$0.005 displays as %q, want $0.01 (half away from zero)", got)
	}
	if got := Infer("$0.004").Display(); got != "$0.00" {
		t.Errorf("$0.004 displays as %q, want $0.00", got)
	}
}
