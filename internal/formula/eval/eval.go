// Package eval walks a syntax tree and produces a value.
//
// It reads *stored* cell values rather than re-evaluating other formulas. That
// is deliberate: ordering is the job of the calculation engine, which schedules
// waves so that a cell's precedents are always computed before it. Keeping the
// two separate is what makes the engine parallelisable and the evaluator pure.
package eval

import (
	"strings"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/formula/ast"
	"github.com/badvibecoder/cellsheet/internal/formula/ops"
	"github.com/badvibecoder/cellsheet/internal/formula/parser"
	"github.com/badvibecoder/cellsheet/internal/formula/registry"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

const (
	// maxDepth stops a pathologically nested formula from exhausting the stack.
	maxDepth = 128

	// maxRangeCells bounds a single range expansion so that a typo like
	// A1:XFD1048576 reports an error instead of allocating 17 billion values.
	maxRangeCells = 1_000_000
)

// Ref is a rectangle a formula reads, used to build the dependency graph.
// Sheet is empty for the sheet that owns the formula.
type Ref struct {
	Sheet string
	From  grid.Ref
	To    grid.Ref
}

// Context is what the evaluator needs to resolve references.
type Context struct {
	Book  *grid.Workbook
	Sheet *grid.Sheet

	// AllowCrossSheet enables reading another sheet. It is OFF in v1 by design
	// (decision D12): the dependency graph is per-sheet, so a cross-sheet read
	// would not be recalculated when the other sheet changed and the cell would
	// quietly hold a stale answer. Until the graph is workbook-scoped, such a
	// reference reports #REF! instead. See PHASE-1-SPEC.md §18.
	AllowCrossSheet bool
}

// Result is an evaluated formula plus everything it read.
type Result struct {
	Value cell.Value
	Refs  []Ref
}

// EvalSource parses and evaluates formula source. The returned error is a
// syntax error for the user interface; the value is still usable so the cell
// can display an error.
func EvalSource(src string, ctx *Context) (Result, error) {
	tree, err := parser.Parse(src)
	if err != nil {
		return Result{Value: cell.Error(cell.ErrValue)}, err
	}
	return Eval(tree, ctx), nil
}

// Eval evaluates a parsed tree.
func Eval(tree ast.Expr, ctx *Context) Result {
	ev := &evaluator{ctx: ctx}
	v := ev.eval(tree, 0)
	return Result{Value: v, Refs: ev.refs}
}

type evaluator struct {
	ctx  *Context
	refs []Ref
}

func (ev *evaluator) eval(e ast.Expr, depth int) cell.Value {
	if depth > maxDepth {
		return cell.Error(cell.ErrNum)
	}
	switch n := e.(type) {
	case ast.Literal:
		return n.Value
	case ast.Name:
		// An identifier that is neither a reference nor a function.
		return cell.Error(cell.ErrName)
	case ast.Ref:
		return ev.readRef(n)
	case ast.Range:
		// A range is only meaningful as a function argument; on its own it is
		// not a value (PHASE-1-SPEC.md §7.7).
		return cell.Error(cell.ErrValue)
	case *ast.Unary:
		return ev.evalUnary(n, depth)
	case *ast.Binary:
		return ev.evalBinary(n, depth)
	case *ast.Call:
		return ev.evalCall(n, depth)
	default:
		return cell.Error(cell.ErrInternal)
	}
}

// sheetFor resolves a sheet name; the empty name means "this sheet".
func (ev *evaluator) sheetFor(name string) (*grid.Sheet, bool) {
	if name == "" {
		if ev.ctx == nil {
			return nil, false
		}
		return ev.ctx.Sheet, ev.ctx.Sheet != nil
	}
	if ev.ctx == nil || ev.ctx.Book == nil {
		return nil, false
	}
	sh, ok := ev.ctx.Book.SheetByName(name)
	if !ok {
		return nil, false
	}
	// A sheet naming itself is harmless even in single-sheet mode.
	if sh == ev.ctx.Sheet {
		return sh, true
	}
	if !ev.ctx.AllowCrossSheet {
		return nil, false
	}
	return sh, true
}

func (ev *evaluator) readRef(r ast.Ref) cell.Value {
	if r.Row < 0 || r.Col < 0 || r.Row >= grid.MaxRows || r.Col >= grid.MaxCols {
		return cell.Error(cell.ErrRef)
	}
	sh, ok := ev.sheetFor(r.Sheet)
	if !ok {
		return cell.Error(cell.ErrRef)
	}
	ref := grid.Ref{Row: uint32(r.Row), Col: uint32(r.Col)}
	ev.refs = append(ev.refs, Ref{Sheet: r.Sheet, From: ref, To: ref})
	return sh.GetRef(ref).Value
}

func (ev *evaluator) expandRange(r ast.Range) ([]cell.Value, cell.ErrorCode) {
	sh, ok := ev.sheetFor(r.From.Sheet)
	if !ok {
		return nil, cell.ErrRef
	}
	rows := r.To.Row - r.From.Row + 1
	cols := r.To.Col - r.From.Col + 1
	if rows <= 0 || cols <= 0 {
		return nil, cell.ErrRef
	}
	if rows*cols > maxRangeCells {
		return nil, cell.ErrNum
	}
	ev.refs = append(ev.refs, Ref{
		Sheet: r.From.Sheet,
		From:  grid.Ref{Row: uint32(r.From.Row), Col: uint32(r.From.Col)},
		To:    grid.Ref{Row: uint32(r.To.Row), Col: uint32(r.To.Col)},
	})
	out := make([]cell.Value, 0, rows*cols)
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			out = append(out, sh.GetRef(grid.Ref{Row: uint32(row), Col: uint32(col)}).Value)
		}
	}
	return out, cell.ErrNone
}

// resultKind keeps the operand's kind, except that Empty becomes a Number so
// that -A1 on a blank cell is the number 0 rather than a blank.
func resultKind(v cell.Value) cell.Kind {
	if v.Kind == cell.KindEmpty {
		return cell.KindNumber
	}
	return v.Kind
}

func (ev *evaluator) evalUnary(n *ast.Unary, depth int) cell.Value {
	v := ev.eval(n.X, depth+1)
	if v.IsError() {
		return v
	}
	if v.Kind == cell.KindText {
		return cell.Error(cell.ErrValue)
	}
	d, _ := v.AsNumber()
	switch n.Op {
	case ast.OpPos:
		return cell.Value{Kind: resultKind(v), Num: d}
	case ast.OpNeg:
		return cell.Value{Kind: resultKind(v), Num: d.Neg()}
	case ast.OpPercent:
		p, ok := d.Percent()
		if !ok {
			return cell.Error(cell.ErrNum)
		}
		return cell.Value{Kind: resultKind(v), Num: p}
	default:
		return cell.Error(cell.ErrInternal)
	}
}

func (ev *evaluator) evalBinary(n *ast.Binary, depth int) cell.Value {
	if n.Op.IsComparison() {
		l := ev.eval(n.X, depth+1)
		r := ev.eval(n.Y, depth+1)
		return compare(n.Op, l, r)
	}
	l := ev.eval(n.X, depth+1)
	if l.IsError() {
		return l
	}
	r := ev.eval(n.Y, depth+1)
	if r.IsError() {
		return r
	}
	co, ok := ops.CellOp(n.Op)
	if !ok {
		return cell.Error(cell.ErrInternal)
	}
	return cell.Apply(co, l, r)
}

func (ev *evaluator) evalCall(n *ast.Call, depth int) cell.Value {
	args := make([]cell.Arg, 0, len(n.Args))
	for _, a := range n.Args {
		if rng, ok := a.(ast.Range); ok {
			vals, code := ev.expandRange(rng)
			if code != cell.ErrNone {
				return cell.Error(code)
			}
			args = append(args, cell.Range(vals))
			continue
		}
		args = append(args, cell.Single(ev.eval(a, depth+1)))
	}
	return registry.Call(n.Name, &registry.Ctx{}, args)
}

func compare(op ast.Op, l, r cell.Value) cell.Value {
	if l.IsError() {
		return l
	}
	if r.IsError() {
		return r
	}
	c := compareValues(l, r)
	var b bool
	switch op {
	case ast.OpEq:
		b = c == 0
	case ast.OpNe:
		b = c != 0
	case ast.OpLt:
		b = c < 0
	case ast.OpGt:
		b = c > 0
	case ast.OpLe:
		b = c <= 0
	case ast.OpGe:
		b = c >= 0
	}
	if b {
		return cell.Number(cell.FromInt64(1))
	}
	return cell.Number(cell.FromInt64(0))
}

// compareValues orders two values the way a spreadsheet does: numbers before
// text, text compared case-insensitively, and a blank equal to zero.
func compareValues(a, b cell.Value) int {
	at := a.Kind == cell.KindText
	bt := b.Kind == cell.KindText
	switch {
	case at && bt:
		return strings.Compare(strings.ToUpper(a.Str), strings.ToUpper(b.Str))
	case at:
		return 1 // numbers sort before text
	case bt:
		return -1
	}
	an, _ := a.AsNumber()
	bn, _ := b.AsNumber()
	return an.Cmp(bn)
}
