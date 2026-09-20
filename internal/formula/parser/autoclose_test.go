package parser

import "testing"

func TestAutoClose(t *testing.T) {
	cases := []struct{ in, want string }{
		// The reported case.
		{"=SUM(A1:A4", "=SUM(A1:A4)"},
		{"=SUM(A1:B1, A2", "=SUM(A1:B1, A2)"},
		{"=((1+2", "=((1+2))"},
		{"=SUM(SUM(A1", "=SUM(SUM(A1))"},
		// Already balanced: untouched.
		{"=SUM(A1:A4)", "=SUM(A1:A4)"},
		{"=1+2", "=1+2"},
		{"", ""},
		{"=()", "=()"},
		// Too many closers: left alone rather than silently dropping one,
		// because that could change what the user wrote.
		{"=SUM(A1))", "=SUM(A1))"},
		{"=)", "=)"},
		// Quoted brackets must not be counted.
		{`=CONCAT("(", A1`, `=CONCAT("(", A1)`},
		{`=CONCAT("((", A1`, `=CONCAT("((", A1)`},
		{`=IF(A1=")", 1, 2`, `=IF(A1=")", 1, 2)`},
		{`='Q3 (final)'!A1`, `='Q3 (final)'!A1`},
		{`=CONCAT("a""(b", A1`, `=CONCAT("a""(b", A1)`},
		// An unterminated quote swallows the rest of the line, so the bracket
		// inside it is not counted — but the function's own bracket still is,
		// and closing it is harmless because the formula is already broken.
		{`=CONCAT("(`, `=CONCAT("()`},
	}
	for _, c := range cases {
		if got := AutoClose(c.in); got != c.want {
			t.Errorf("AutoClose(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAutoCloseProducesParseableFormulas: the repair is only useful if the
// result actually parses.
func TestAutoCloseProducesParseableFormulas(t *testing.T) {
	for _, in := range []string{
		"=SUM(A1:A4", "=((1+2", "=SUM(SUM(A1", "=IF(A1>0, A1, -A1",
		`=CONCAT("(", A1`,
	} {
		fixed := AutoClose(in)
		if _, err := Parse(fixed); err != nil {
			t.Errorf("AutoClose(%q) = %q, which still does not parse: %v", in, fixed, err)
		}
	}
}

func TestAutoCloseIsIdempotent(t *testing.T) {
	for _, in := range []string{"=SUM(A1:A4", "=((1+2", "=SUM(A1:A4)", "="} {
		once := AutoClose(in)
		if twice := AutoClose(once); twice != once {
			t.Errorf("AutoClose is not idempotent: %q -> %q -> %q", in, once, twice)
		}
	}
}
