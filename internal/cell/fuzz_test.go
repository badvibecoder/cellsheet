package cell

import "testing"

// FuzzParseDecRoundTrip: anything ParseDec accepts must survive being printed
// and read back unchanged. A decimal that cannot round-trip would silently
// alter a saved workbook.
func FuzzParseDecRoundTrip(f *testing.F) {
	for _, s := range []string{
		"0", "1", "-1", "450", "12.75", "-0.5", "1e3", "0.001",
		"9223372036854775807", "-9223372036854775808", "1e18", "1e-18",
		"0.000000000000000001", "123456789012345678",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, ok := ParseDec(s)
		if !ok {
			return
		}
		printed := d.String()
		back, ok := ParseDec(printed)
		if !ok {
			t.Fatalf("ParseDec(%q) printed %q, which will not parse back", s, printed)
		}
		if !d.Equal(back) {
			t.Fatalf("round trip changed the value: %q -> %q -> %q", s, printed, back.String())
		}
	})
}

// FuzzInferProperties: inference must never panic, Display must never panic,
// and the three formatting paths must be mutually consistent.
func FuzzInferProperties(f *testing.F) {
	for _, s := range []string{
		"", " ", "$50000", "$50,000.00", "-$500", "$.50", "450", "-3", "12.75",
		"50,000", "1e3", "10.00", "Operations", "YES", "50%", "(500)",
		"3/4/2026", "1,00", "$", "abc123", "  450  ", "9223372036854775807",
		"$999999999999999999999", "1e400", "-0", ".5", "..", "1.2.3",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v := Infer(s)
		_ = v.Display()
		_ = v.AlignRight()
		_ = v.AlignCenter()
		_ = v.Skip()
		_ = v.IsNumeric()
		_, _ = v.AsNumber()

		switch v.Kind {
		case KindNumber:
			// A plain number's display is exact, so it must round-trip.
			again := Infer(v.Display())
			if again.Kind != KindNumber || !again.Num.Equal(v.Num) {
				t.Fatalf("Infer(%q) = %s, but its display %q re-infers as %v %s",
					s, v.Num.String(), v.Display(), again.Kind, again.Num.String())
			}
		case KindCurrency:
			// Currency always displays two decimals, so a sub-cent amount is
			// deliberately not recoverable from its display. What must hold is
			// that the display is the correct two-decimal rounding.
			again := Infer(v.Display())
			if again.Kind != KindCurrency {
				t.Fatalf("Infer(%q) = %s, but its display %q re-infers as %v",
					s, v.Num.String(), v.Display(), again.Kind)
			}
			if want := v.Num.Round(2); !again.Num.Equal(want) {
				t.Fatalf("Infer(%q) = %s displays as %q, which re-infers as %s, want %s",
					s, v.Num.String(), v.Display(), again.Num.String(), want.String())
			}
		case KindError:
		default:
			// Text is stored verbatim, so reading it back is exact.
			if got := Infer(v.Str); got.Kind == KindNumber {
				t.Fatalf("text %q was read back as a number", v.Str)
			}
		}
	})
}

// FuzzApplyNeverPanics exercises the promotion table with arbitrary operand
// kinds, because a mismatch there is what would take the calculation engine
// down on a user's machine.
func FuzzApplyNeverPanics(f *testing.F) {
	f.Add(uint8(0), uint8(0), int64(1), int8(0), int64(2), int8(0))
	f.Add(uint8(2), uint8(2), int64(50000), int8(0), int64(0), int8(0))
	f.Add(uint8(1), uint8(4), int64(-1), int8(18), int64(7), int8(3))
	f.Fuzz(func(t *testing.T, op, kindA uint8, mantA int64, scaleA int8, mantB int64, scaleB int8) {
		if scaleA < 0 {
			scaleA = -scaleA
		}
		if scaleB < 0 {
			scaleB = -scaleB
		}
		if scaleA > 18 {
			scaleA %= 19
		}
		if scaleB > 18 {
			scaleB %= 19
		}
		mk := func(k uint8, m int64, s int8) Value {
			switch k % 6 {
			case 0:
				return Empty()
			case 1:
				return Number(NewDec(m, s))
			case 2:
				return Currency(NewDec(m, s))
			case 3:
				return Text("x")
			case 4:
				return Error(ErrValue)
			default:
				return Number(NewDec(m, s))
			}
		}
		a := mk(kindA, mantA, scaleA)
		b := mk(kindA+op, mantB, scaleB)
		got := Apply(Op(op%6), a, b)
		_ = got.Display()
		if got.Kind == KindError && got.Code == ErrNone {
			t.Fatalf("Apply returned a malformed error value: %+v", got)
		}
	})
}
