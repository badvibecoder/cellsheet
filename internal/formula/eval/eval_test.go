package eval

import (
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"

	// Importing the functions package is what makes =SUM() exist. This blank
	// import is the single wiring point for the whole function library.
	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

// mockBook builds the workbook from the approved mock, with C1 computed by the
// evaluator itself so its stored value is genuine.
func mockBook(t *testing.T) (*grid.Workbook, *grid.Sheet) {
	t.Helper()
	wb := grid.NewWorkbook()
	sh := wb.Active()
	set := func(ref, src string) {
		r, ok := grid.ParseRef(ref)
		if !ok {
			t.Fatalf("bad ref %q", ref)
		}
		sh.Set(r.Row, r.Col, cell.Cell{Source: src, Value: cell.Infer(src)})
	}
	set("A1", "$50,000.00")
	set("B1", "$1,250.00")
	set("A2", "$14,200.50")
	set("B2", "$320.00")
	set("A3", "Operations")
	set("B3", "Equipment")
	set("C3", "Subtotal")
	set("D3", "Audited?")
	set("A4", "450")
	set("B4", "12.75")
	set("C4", "5737.50")
	set("D4", "YES")

	// C1 = SUM(A1:B1), computed for real.
	res, err := EvalSource("=SUM(A1:B1)", &Context{Book: wb, Sheet: sh})
	if err != nil {
		t.Fatalf("computing C1: %v", err)
	}
	sh.Set(0, 2, cell.Cell{Source: "=SUM(A1:B1)", Value: res.Value})
	return wb, sh
}

func evalOn(t *testing.T, wb *grid.Workbook, sh *grid.Sheet, src string) cell.Value {
	t.Helper()
	res, err := EvalSource(src, &Context{Book: wb, Sheet: sh})
	if err != nil {
		t.Fatalf("EvalSource(%q): %v", src, err)
	}
	return res.Value
}

// TestMockWorkbook is the full worked-examples table from PHASE-1-SPEC.md §7.8,
// plus the arithmetic and promotion cases from §7.2 and §7.4.
func TestMockWorkbook(t *testing.T) {
	wb, sh := mockBook(t)

	cases := []struct{ src, want string }{
		// The user's examples.
		{"=SUM(A1:D1)", "$102,500.00"}, // A1 + B1 + C1, D1 is empty
		{"=SUM(A1 + B1)", "$51,250.00"},
		{"-=SUM(A1 + B1)", "-$51,250.00"},
		{"=-SUM(A1 + B1)", "-$51,250.00"},
		{"=SUM(A1:B1, 10)", "$51,260.00"},
		{"=SUM(A1:D1, F1, 10)", "$102,510.00"},

		// Currency survives addition and scaling; not currency x currency.
		{"=A1*2", "$100,000.00"},
		{"=A1*B1", "62500000"},
		{"=A1/B2", "156.25"},
		{"=A1-A2", "$35,799.50"},

		// Plain numbers.
		{"=A4*B4", "5737.5"},
		{"=A4+B4", "462.75"},
		{"=C4/2", "2868.75"},

		// Text is never used in maths.
		{"=A3+1", "#VALUE!"},
		{"=SUM(A3, 1)", "#VALUE!"}, // a scalar text argument
		{"=SUM(A3:A4)", "450"},     // text inside a range is skipped
		{"=SUM(A3:A4, 50)", "500"}, // ... and a literal adds to it
		{"=SUM(D3:D4)", "0"},       // all text: no numbers to add

		// Errors.
		{"=B2/0", "#DIV/0!"},
		{"=SUMM(A1)", "#NAME?"},
		{"=NOPE(A1)", "#NAME?"},
		{"=1/0", "#DIV/0!"},
		{"=A1:D1", "#VALUE!"}, // a bare range is not a value

		// Order of operations, end to end.
		{"=1+2*3", "7"},
		{"=(1+2)*3", "9"},
		{"=8/4/2", "1"},
		{"=2^3^2", "512"},
		{"=-2^2", "-4"},
		{"=2^10", "1024"},
		{"=1-2-3", "-4"},
		{"=10%", "0.1"},
		{"=-50%", "-0.5"},
		{"=100*10%", "10"},

		// Comparisons yield 1 or 0.
		{"=1<2", "1"},
		{"=1>2", "0"},
		{"=1=1", "1"},
		{"=1<>1", "0"},
		{"=A4>=450", "1"},
		{"=A3=\"Operations\"", "1"},
		{"=A3=\"operations\"", "1"}, // comparisons are case-insensitive
		{"=1<\"a\"", "1"},           // numbers sort before text

		// Currency literals.
		{"=$500+$250", "$750.00"},
		{"=$1,000*2", "$2,000.00"},

		// Blank cells are zero in arithmetic.
		{"=Z99+5", "5"},
		{"=Z99", ""},
		{"=-Z99", "0"},
	}

	for _, c := range cases {
		got := evalOn(t, wb, sh, c.src)
		if got.Display() != c.want {
			t.Errorf("%s = %q (%v), want %q", c.src, got.Display(), got.Kind, c.want)
		}
	}
}

func TestSignedFloatPromotion(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	sh.Set(0, 0, cell.Cell{Value: cell.Number(cell.FromInt64(450))})
	sh.Set(1, 0, cell.Cell{Value: cell.Number(cell.NewDec(1275, 2))})

	// Requiring a signed float: a sign and a fraction must both survive.
	got := evalOn(t, wb, sh, "=-(A1+A2)")
	if got.Display() != "-462.75" {
		t.Errorf("got %q, want -462.75", got.Display())
	}
	// The stored value keeps full precision: no lossy conversion happened.
	if !got.Num.Equal(cell.NewDec(-46275, 2)) {
		t.Errorf("stored value lost precision: %+v", got.Num)
	}
}

func TestRefsAreRecorded(t *testing.T) {
	wb, sh := mockBook(t)

	res, err := EvalSource("=SUM(A1:B2)+C4", &Context{Book: wb, Sheet: sh})
	if err != nil {
		t.Fatal(err)
	}
	// One rectangle for the range and one single cell.
	var haveRange, haveSingle bool
	for _, r := range res.Refs {
		if r.From == (grid.Ref{Row: 0, Col: 0}) && r.To == (grid.Ref{Row: 1, Col: 1}) {
			haveRange = true
		}
		if r.From == (grid.Ref{Row: 3, Col: 2}) && r.To == r.From {
			haveSingle = true
		}
	}
	if !haveRange {
		t.Errorf("the range A1:B2 was not recorded as a precedent: %+v", res.Refs)
	}
	if !haveSingle {
		t.Errorf("the single reference C4 was not recorded: %+v", res.Refs)
	}
}

// TestCrossSheetIsRefusedInV1 covers rule R3: v1 must fail honestly rather than
// guess, and the source text is preserved for when the feature arrives.
func TestCrossSheetIsRefusedInV1(t *testing.T) {
	wb, sh := mockBook(t)
	if _, err := wb.AddSheet("Sheet2"); err != nil {
		t.Fatal(err)
	}
	got := evalOn(t, wb, sh, "=Sheet2!A1")
	if got.Code != cell.ErrRef {
		t.Errorf("cross-sheet reference should be #REF! in v1, got %s", got.Display())
	}
	// A missing sheet is also #REF!, and must not panic.
	got = evalOn(t, wb, sh, "=NoSuchSheet!A1")
	if got.Code != cell.ErrRef {
		t.Errorf("missing sheet should be #REF!, got %s", got.Display())
	}
}

func TestHugeRangeIsBounded(t *testing.T) {
	wb, sh := mockBook(t)
	got := evalOn(t, wb, sh, "=SUM(A1:XFD1048576)")
	if got.Code != cell.ErrNum {
		t.Errorf("an oversized range should be #NUM!, got %s", got.Display())
	}
}

// TestDeepNestingIsBounded guards the evaluator's recursion cap. Parentheses
// alone are not the risk (they produce no tree nodes); nested unary operators
// and calls are, because each one is a recursive eval.
func TestDeepNestingIsBounded(t *testing.T) {
	wb, sh := mockBook(t)

	src := "="
	for i := 0; i < 400; i++ {
		src += "-"
	}
	src += "1"
	if got := evalOn(t, wb, sh, src); !got.IsError() {
		t.Errorf("400 nested unary operators should be bounded, got %s", got.Display())
	}

	src = "=SUM("
	for i := 0; i < 400; i++ {
		src += "SUM("
	}
	src += "1"
	for i := 0; i < 401; i++ {
		src += ")"
	}
	if got := evalOn(t, wb, sh, src); !got.IsError() {
		t.Errorf("400 nested calls should be bounded, got %s", got.Display())
	}
}

// TestSelfReferenceBySheetNameIsAllowed: writing =Sheet1!A1 while on Sheet1 is
// harmless and should not be refused just because cross-sheet reads are off.
func TestSelfReferenceBySheetNameIsAllowed(t *testing.T) {
	wb, sh := mockBook(t)
	got := evalOn(t, wb, sh, "=Sheet1!A4")
	if got.Display() != "450" {
		t.Errorf("self-reference by sheet name = %q, want 450", got.Display())
	}
}

func TestSyntaxErrorIsReported(t *testing.T) {
	wb, sh := mockBook(t)
	res, err := EvalSource("=1+", &Context{Book: wb, Sheet: sh})
	if err == nil {
		t.Error("a malformed formula should report a syntax error")
	}
	if !res.Value.IsError() {
		t.Error("a malformed formula should still produce an error value")
	}
}
