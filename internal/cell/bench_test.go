package cell

import "testing"

// BenchmarkInfer measures the inner loop of every keystroke and every pasted
// cell: deciding what a piece of text means.
func BenchmarkInfer(b *testing.B) {
	inputs := []string{"$50,000.00", "450", "12.75", "Operations", "-3", "50,000", "1e3"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Infer(inputs[i%len(inputs)])
	}
}

// BenchmarkDecimalAdd is the fast path, used by every sum.
func BenchmarkDecimalAdd(b *testing.B) {
	x := NewDec(5000000, 2)
	y := NewDec(125000, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = x.Add(y)
	}
}

// BenchmarkDecimalMul is also exact, so it costs a 64-bit multiply plus an
// overflow check.
func BenchmarkDecimalMul(b *testing.B) {
	x := NewDec(1500, 2)
	y := NewDec(125, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = x.Mul(y)
	}
}

// BenchmarkDecimalDiv is the slow path: it goes through math/big to keep the
// result exact and correctly rounded. It is worth knowing how much slower it
// is, because a sheet of divisions costs more than a sheet of sums.
func BenchmarkDecimalDiv(b *testing.B) {
	x := NewDec(5000000, 2)
	y := NewDec(3, 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = x.Div(y)
	}
}

// BenchmarkFormatCurrency is called for every visible currency cell on every
// frame.
func BenchmarkFormatCurrency(b *testing.B) {
	d := NewDec(5125000, 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FormatCurrency(d)
	}
}
