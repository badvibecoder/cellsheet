package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/formula/ast"
)

// sexpr renders a tree fully parenthesised, which is how the precedence and
// associativity tests assert shape rather than just a numeric result.
func sexpr(e ast.Expr) string {
	switch n := e.(type) {
	case ast.Literal:
		return n.Value.Display()
	case ast.Name:
		return n.Ident
	case ast.Ref:
		if n.Sheet != "" {
			return n.Sheet + "!" + colRow(n.Row, n.Col)
		}
		return colRow(n.Row, n.Col)
	case ast.Range:
		return "(" + sexpr(n.From) + ":" + sexpr(n.To) + ")"
	case *ast.Unary:
		if n.Op == ast.OpPercent {
			return "(" + sexpr(n.X) + "%)"
		}
		return "(" + n.Op.String() + sexpr(n.X) + ")"
	case *ast.Binary:
		return "(" + sexpr(n.X) + n.Op.String() + sexpr(n.Y) + ")"
	case *ast.Call:
		parts := make([]string, len(n.Args))
		for i, a := range n.Args {
			parts[i] = sexpr(a)
		}
		return n.Name + "(" + strings.Join(parts, ",") + ")"
	default:
		return "?"
	}
}

func colRow(row, col int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	c := col
	name := ""
	for {
		name = string(letters[c%26]) + name
		c = c/26 - 1
		if c < 0 {
			break
		}
	}
	return fmt.Sprintf("%s%d", name, row+1)
}

func TestParseShape(t *testing.T) {
	cases := []struct{ in, want string }{
		// Precedence and associativity, straight from PHASE-1-SPEC.md §7.6.
		{"=1+2*3", "(1+(2*3))"},
		{"=1*2+3", "((1*2)+3)"},
		{"=8/4/2", "((8/4)/2)"},
		{"=1-2-3", "((1-2)-3)"},
		{"=2^3^2", "(2^(3^2))"}, // ^ is right-associative
		{"=-2^2", "(-(2^2))"},   // ^ binds tighter than unary minus
		{"=-2*3", "((-2)*3)"},
		{"=(1+2)*3", "((1+2)*3)"},
		{"=-50%", "(-(50%))"}, // % binds tighter than negation
		{"=10%", "(10%)"},
		{"=100%%", "((100%)%)"},
		{"=1<2", "(1<2)"},
		{"=1+2=3", "((1+2)=3)"},
		{"=A1", "A1"},
		{"=AA100", "AA100"},
		{"=$B$7", "B7"}, // absolute markers are accepted and ignored for now
		{"=A1:D1", "(A1:D1)"},
		{"=D1:A1", "(A1:D1)"}, // normalised to top-left first
		{"=A1:C5", "(A1:C5)"},

		// Functions and the user's examples.
		{"=SUM(A1:D1)", "SUM((A1:D1))"},
		{"=SUM(A1 + D1)", "SUM((A1+D1))"},
		{"-=SUM(A1 + D1)", "(-SUM((A1+D1)))"},
		{"=-SUM(A1+D1)", "(-SUM((A1+D1)))"},
		{"=SUM(A1:D1, F1, 10)", "SUM((A1:D1),F1,10)"},
		{"=sum(a1:b1)", "SUM((A1:B1))"},
		{"=  Sum ( A1 : D1 )", "SUM((A1:D1))"},
		{"=SUM()", "SUM()"},
		{"=SUM(A1,SUM(B1,C1))", "SUM(A1,SUM(B1,C1))"},

		// Literals.
		{"=$500", "$500.00"},
		{"=$50000", "$50,000.00"},
		{"=$1,250.00", "$1,250.00"},
		{"=1.5", "1.5"},
		{"=1e3", "1000"},
		{"=\"hello\"", "hello"},
		{"=\"say \"\"hi\"\"\"", "say \"hi\""},

		// Cross-sheet syntax is lexed and preserved even though v1 will not
		// evaluate it (PHASE-1-SPEC.md §18.1, rule R1).
		{"=Sheet2!A1", "Sheet2!A1"},
		{"=sheet2!a1", "sheet2!A1"},
		{"='Q3 Budget'!A1", "Q3 Budget!A1"},
		{"='It''s Q3'!A1", "It's Q3!A1"},
		{"=Sheet2!A1:B5", "(Sheet2!A1:Sheet2!B5)"}, // sheet is inherited
	}
	for _, c := range cases {
		tree, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", c.in, err)
			continue
		}
		if got := sexpr(tree); got != c.want {
			t.Errorf("Parse(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"",
		"=1+",
		"=*2",
		"=SUM(A1",
		"=SUM(A1,)",
		"=SUM(A1 B1)",
		"=Sheet2!A1:Sheet3!B5", // a range cannot span two sheets
		"=Sheet2!5",            // not a cell reference
		"=1 2",
		"='unterminated",
		"=\"unterminated",
		"=@",
	}
	for _, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) should have failed", src)
		}
	}
}

func TestParseErrorHasPosition(t *testing.T) {
	_, err := Parse("=1+")
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, ok := err.(*Error); !ok {
		t.Fatalf("expected a *parser.Error, got %T", err)
	}
}
