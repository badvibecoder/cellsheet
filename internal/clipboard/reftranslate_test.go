package clipboard

import "testing"

func TestTranslateFormulaShiftsRelativeReferences(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		dr, dc int
		want   string
	}{
		// The reported case: =sum(b1:b5) copied one column right.
		{"lowercase range right", "=sum(b1:b5)", 0, 1, "=sum(c1:c5)"},
		{"range down", "=SUM(A1:B1)", 1, 0, "=SUM(A2:B2)"},
		{"range right", "=SUM(A1:B1)", 0, 1, "=SUM(B1:C1)"},
		{"two cells", "=A1+D1", 2, 3, "=D3+G3"},
		{"down and right", "=A1:B2", 1, 1, "=B2:C3"},

		// Absolute markers pin one axis at a time.
		{"absolute column", "=$A1", 0, 1, "=$A1"},
		{"absolute row", "=A$1", 1, 0, "=A$1"},
		{"absolute both", "=$A$1", 4, 4, "=$A$1"},
		{"mixed", "=$A1+B$2", 2, 2, "=$A3+D$2"},

		// Sheet-qualified references keep the sheet and shift the cell.
		{"sheet name", "=Sheet2!A1", 0, 1, "=Sheet2!B1"},
		{"quoted sheet name", "='Q3 (final)'!A1", 1, 0, "='Q3 (final)'!A2"},

		// Literals and function names are never rewritten.
		{"string literal", `=CONCAT("A1", A1)`, 0, 1, `=CONCAT("A1", B1)`},
		{"function name", "=SUM(1, 2)", 3, 3, "=SUM(1, 2)"},
		{"plain text", "hello A1", 0, 1, "hello B1"},

		// Leaving the sheet is the same failure Excel reports.
		{"off the left edge", "=A1", 0, -1, "=#REF!"},
		{"off the top edge", "=A1", -1, 0, "=#REF!"},

		// Column-name rollover uses bijective base-26.
		{"rollover", "=Z1", 0, 1, "=AA1"},
		{"rollback", "=AA10", 0, -1, "=Z10"},

		{"no offset is a no-op", "=SUM(B1:B5)", 0, 0, "=SUM(B1:B5)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TranslateFormula(c.src, c.dr, c.dc); got != c.want {
				t.Errorf("TranslateFormula(%q, %d, %d) = %q, want %q",
					c.src, c.dr, c.dc, got, c.want)
			}
		})
	}
}

// TestTranslateFormulaLeavesMalformedRefsAlone: a token that only resembles a
// reference must survive untouched, or a typo would be silently edited.
func TestTranslateFormulaLeavesMalformedRefsAlone(t *testing.T) {
	for _, src := range []string{"=A", "=1A", "=A1.5", "=A1B", "=ABCD1", "=A0"} {
		if got := TranslateFormula(src, 1, 1); got != src {
			t.Errorf("TranslateFormula(%q) = %q, want it unchanged", src, got)
		}
	}
}
