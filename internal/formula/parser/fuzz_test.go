package parser

import (
	"testing"

	"github.com/badvibecoder/cellsheet/internal/formula/ast"
)

// FuzzParse asserts the two properties a parser must never break: it must not
// panic, and it must not hang. Returning an error is always an acceptable
// outcome; crashing is not.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"=SUM(A1:B1)",
		"=SUM(A1 + D1)",
		"-=SUM(A1+B1)",
		"=1+2*3^4/5%",
		"=-2^2",
		"=2^3^2",
		"=$500+$1,250.00",
		"=Sheet2!A1:B5",
		"='Q3 Budget'!A1",
		"='It''s Q3'!A1",
		"=SUM(A1:D1, F1, 10)",
		"=\"text\"",
		"=IF(A1>0,A1,-A1)",
		"=((((1))))",
		"",
		"=",
		"=1+",
		"=SUM(",
		"=!!!!",
		"=A1:",
		"=:::",
		"=$",
		"=1e",
		"=1e+",
		"=\x00",
		"=A1\tB2",
		"=SUM(A1,B1,)",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			t.Skip("inputs this long are rejected by the length guard")
		}
		tree, err := Parse(src)
		if err != nil {
			return // a rejected formula is a correct result
		}
		if tree == nil {
			t.Fatal("Parse returned no error and no tree")
		}
		// Walking a successfully parsed tree must also be safe.
		refs := 0
		Refs(tree, func(string, ast.Ref) { refs++ })
	})
}

// FuzzAutoCloseLeavesValidFormulasAlone states the property that makes the
// repair safe: a formula that already parses is never modified. If it were, a
// typo in the repair could silently change the meaning of working formulas
// across the whole program.
func FuzzAutoCloseLeavesValidFormulasAlone(f *testing.F) {
	for _, s := range []string{
		"=SUM(A1:B1)", "=1+2", "=((1+2))*3", `=CONCAT("(", A1)`, `=")"`,
		"=SUM(A1:A4", "=((1+2", "", "=", "-=SUM(A1)",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 1024 {
			t.Skip()
		}
		if _, err := Parse(src); err != nil {
			return // only formulas that already parse are constrained
		}
		if got := AutoClose(src); got != src {
			t.Fatalf("AutoClose changed a formula that already parses: %q -> %q", src, got)
		}
	})
}
