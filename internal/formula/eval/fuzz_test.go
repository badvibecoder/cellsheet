package eval

import (
	"strings"
	"sync"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

var (
	sharedOnce  sync.Once
	sharedBook  *grid.Workbook
	sharedSheet *grid.Sheet
)

// sharedContext builds one read-only workbook for every fuzz execution.
// Evaluation never writes to the sheet, so sharing it is safe and keeps the
// fuzzer fast enough to explore properly.
func sharedContext() *Context {
	sharedOnce.Do(func() {
		wb := grid.NewWorkbook()
		sh := wb.Active()
		sh.Set(0, 0, cell.Cell{Source: "$50,000.00", Value: cell.Infer("$50,000.00")})
		sh.Set(0, 1, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
		sh.Set(0, 2, cell.Cell{Source: "=SUM(A1:B1)", Value: cell.Currency(cell.NewDec(5125000, 2))})
		sh.Set(1, 0, cell.Cell{Source: "Operations", Value: cell.Infer("Operations")})
		sh.Set(1, 1, cell.Cell{Source: "12.75", Value: cell.Infer("12.75")})
		sh.Set(2, 0, cell.Cell{Source: "$0.0001", Value: cell.Infer("$0.0001")})
		sh.Set(3, 0, cell.Cell{Value: cell.Error(cell.ErrDivZero)})
		if _, err := wb.AddSheet("Sheet2"); err != nil {
			panic(err)
		}
		sharedBook, sharedSheet = wb, sh
	})
	return &Context{Book: sharedBook, Sheet: sharedSheet}
}

// FuzzEvalSource is the broadest safety net in the suite: it puts arbitrary
// text through the whole pipeline — lexer, parser, evaluator, type promotion,
// the function registry — against a workbook containing every value kind.
//
// The properties are that it must never panic, that a value is always
// returned, and that an error value is always well formed.
func FuzzEvalSource(f *testing.F) {
	seeds := []string{
		"=SUM(A1:B1)",
		"=SUM(A1 + B1)",
		"-=SUM(A1+B1)",
		"=A1/B2",
		"=A1/A1",
		"=A1/A2",
		"=A4+1",
		"=SUM(A2:B2)",
		"=SUM(A4, 1)",
		"=Z99+5",
		"=-Z99",
		"=A3",
		"=1/0",
		"=0/0",
		"=NOPE(A1)",
		"=SUM()",
		"=SUM(A1:B1,A1:B1,A1:B1)",
		"=Sheet2!A1",
		"=Sheet1!A1",
		"=A1%",
		"=100%%",
		"=2^10000",
		"=1e300*1e300",
		"=99999999999999999999999999",
		"=\"text\"+1",
		"=A1&\"x\"",
		"=(((((((((1)))))))))",
		"",
		"=",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			t.Skip("the parser has its own length guard")
		}
		res, err := EvalSource(src, sharedContext())
		if err != nil {
			// A syntax error must still produce a displayable value.
			if !res.Value.IsError() {
				t.Fatalf("EvalSource(%q) reported %v but returned %v", src, err, res.Value.Kind)
			}
		}
		if res.Value.Kind == cell.KindError && res.Value.Code == cell.ErrNone {
			t.Fatalf("EvalSource(%q) returned a malformed error value", src)
		}
		// Displaying the result must also be safe, because that is what the
		// grid does with it.
		_ = res.Value.Display()
		// Every reference recorded must be within the sheet's limits, or the
		// dependency graph would index a cell that cannot exist.
		for _, r := range res.Refs {
			if r.From.Row >= grid.MaxRows || r.To.Row >= grid.MaxRows ||
				r.From.Col >= grid.MaxCols || r.To.Col >= grid.MaxCols {
				t.Fatalf("EvalSource(%q) recorded an out-of-range reference %+v", src, r)
			}
			if r.From.Row > r.To.Row || r.From.Col > r.To.Col {
				t.Fatalf("EvalSource(%q) recorded a reversed rectangle %+v", src, r)
			}
		}
	})
}

// FuzzEvalIsDeterministic: the same formula over the same data must always give
// the same answer. This is what makes the parallel calculation engine safe to
// reason about.
func FuzzEvalIsDeterministic(f *testing.F) {
	for _, s := range []string{"=SUM(A1:B2)", "=A1/A2", "=A1%+B2*2", "=-A3"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 512 || strings.Count(src, "(") > 32 {
			t.Skip()
		}
		ctx := sharedContext()
		a, errA := EvalSource(src, ctx)
		b, errB := EvalSource(src, ctx)
		if (errA == nil) != (errB == nil) {
			t.Fatalf("EvalSource(%q) disagreed with itself about validity", src)
		}
		if a.Value.Display() != b.Value.Display() || a.Value.Kind != b.Value.Kind {
			t.Fatalf("EvalSource(%q) is not deterministic: %v %q then %v %q",
				src, a.Value.Kind, a.Value.Display(), b.Value.Kind, b.Value.Display())
		}
		if len(a.Refs) != len(b.Refs) {
			t.Fatalf("EvalSource(%q) recorded a different number of references", src)
		}
	})
}
