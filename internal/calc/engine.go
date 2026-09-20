// Package calc schedules and executes recalculation.
//
// The design is in PHASE-1-SPEC.md §8: editing a cell marks only its transitive
// dependents dirty, the dirty set is sorted into waves, and cells *within* a
// wave are evaluated concurrently because they cannot affect one another. The
// answer therefore never depends on scheduling, and a fifty-cell recalculation
// genuinely uses several cores.
package calc

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/formula/eval"
	"github.com/badvibecoder/cellsheet/internal/grid"

	// The engine links the standard function library, so that any program
	// which calculates has =SUM() and friends available. This is the single
	// wiring point for the library: adding a file to that package adds a
	// function with no change here (PHASE-1-SPEC.md §14.3).
	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

// Tuning constants. The threshold is the important one: fanning three cells out
// to eight workers costs more than computing them.
const (
	DefaultWorkers   = 8
	DefaultThreshold = 64

	// DefaultMaxIndexedRange bounds how many individual cells a single range
	// may contribute to the dependency index. Beyond it the formula is treated
	// as "broad" and recomputed on every pass: correct, if slower.
	DefaultMaxIndexedRange = 100_000
)

// Stats describes one recalculation, for the status bar and for tests.
type Stats struct {
	Evaluated  int
	Waves      int
	Cycles     int
	Broad      int
	Concurrent bool
	Duration   time.Duration
}

// Engine owns the dependency graph, the worker pool and the recalculation
// policy for one workbook.
type Engine struct {
	wb         *grid.Workbook
	graph      *Graph
	pool       *pool
	workers    int
	threshold  int
	maxIndexed int

	mu       sync.Mutex
	lastErr  error
	lastStat Stats
}

// New creates an engine for a workbook.
func New(wb *grid.Workbook) *Engine {
	workers := min(runtime.NumCPU(), DefaultWorkers)
	if workers < 1 {
		workers = 1
	}
	return &Engine{
		wb:         wb,
		graph:      NewGraph(),
		pool:       newPool(workers),
		workers:    workers,
		threshold:  DefaultThreshold,
		maxIndexed: DefaultMaxIndexedRange,
	}
}

// Close shuts down the worker pool.
func (e *Engine) Close() { e.pool.Close() }

// Graph exposes the dependency graph, for tests and inspection.
func (e *Engine) Graph() *Graph { return e.graph }

// Workers is the pool size.
func (e *Engine) Workers() int { return e.workers }

// LastStats returns the most recent recalculation's statistics.
func (e *Engine) LastStats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastStat
}

// SetThreshold overrides the inline-versus-parallel cutoff; used by tests.
func (e *Engine) SetThreshold(n int) { e.threshold = n }

// SetMaxIndexedRange overrides how large a range may be before its formula is
// treated as broad; used by tests.
func (e *Engine) SetMaxIndexedRange(n int) { e.maxIndexed = n }

// EvalContext builds the evaluator context for a sheet.
func (e *Engine) EvalContext(sh *grid.Sheet) *eval.Context {
	return &eval.Context{Book: e.wb, Sheet: sh}
}

// Evaluate parses and evaluates a formula without storing anything, which is
// what the formula bar preview and tests use.
func (e *Engine) Evaluate(sh *grid.Sheet, src string) (cell.Value, error) {
	res, err := eval.EvalSource(src, e.EvalContext(sh))
	return res.Value, err
}

// Set stores a typed value in a cell and recalculates everything affected.
//
// It returns the cell's resulting value. A formula's precedents are registered
// before the recalculation so that the graph is correct on the very first pass.
func (e *Engine) Set(ctx context.Context, sh *grid.Sheet, ref grid.Ref, typed string) cell.Value {
	cl := cell.Cell{Source: typed}
	var precedents []grid.Ref
	broad := false

	if cell.IsFormula(typed) {
		res, err := eval.EvalSource(typed, e.EvalContext(sh))
		cl.Value = res.Value
		if err != nil {
			e.setErr(err)
			cl.Value = cell.Error(cell.ErrValue)
		} else {
			e.setErr(nil)
		}
		precedents, broad = e.indexRefs(sh, res.Refs)
	} else {
		cl.Value = cell.Infer(typed)
	}

	sh.Set(ref.Row, ref.Col, cl)

	if cl.IsFormula() {
		e.graph.SetFormula(sh.ID, ref, typed, precedents, broad)
	} else {
		e.graph.ClearFormula(sh.ID, ref)
	}

	e.Recalc(ctx, sh, []grid.Ref{ref})
	return sh.GetRef(ref).Value
}

// Clear empties a cell and recalculates.
func (e *Engine) Clear(ctx context.Context, sh *grid.Sheet, ref grid.Ref) {
	sh.Clear(ref.Row, ref.Col)
	e.graph.ClearFormula(sh.ID, ref)
	e.Recalc(ctx, sh, []grid.Ref{ref})
}

// RecalcAll recomputes every formula on a sheet, used after loading a file.
func (e *Engine) RecalcAll(ctx context.Context, sh *grid.Sheet) Stats {
	dirty := e.graph.AllFormulas(sh.ID)
	return e.run(ctx, sh, dirty)
}

// Recalc recomputes everything affected by a change to the given cells.
func (e *Engine) Recalc(ctx context.Context, sh *grid.Sheet, changed []grid.Ref) Stats {
	dirty := e.graph.Dependents(sh.ID, changed)
	// A formula that was itself edited must be recomputed too.
	for _, c := range changed {
		if e.graph.IsFormula(sh.ID, c) {
			dirty = append(dirty, c)
		}
	}
	return e.run(ctx, sh, dirty)
}

func (e *Engine) run(ctx context.Context, sh *grid.Sheet, dirty []grid.Ref) Stats {
	start := time.Now()
	dirty = dedupeRefs(dirty)
	stat := Stats{}

	if len(dirty) == 0 {
		e.record(stat, start)
		return stat
	}

	waves, cyclic := e.graph.Waves(sh.ID, dirty)

	// Cells in a cycle can never settle; mark them rather than hang.
	for _, r := range cyclic {
		cl := sh.GetRef(r)
		if !cl.IsFormula() {
			continue
		}
		cl.Value = cell.Error(cell.ErrCirc)
		sh.Set(r.Row, r.Col, cl)
	}
	stat.Cycles = len(cyclic)

	stat.Broad = len(e.graph.broad[sh.ID])
	stat.Concurrent = len(dirty) >= e.threshold

	for _, wave := range waves {
		if ctx != nil {
			select {
			case <-ctx.Done():
				// A newer edit superseded this one; discard the results.
				e.record(stat, start)
				return stat
			default:
			}
		}
		// Compute in parallel, reading the sheet but never writing it.
		values := make([]cell.Value, len(wave))
		run := func(i int) {
			ref := wave[i]
			src, ok := e.graph.Source(sh.ID, ref)
			if !ok {
				values[i] = sh.GetRef(ref).Value
				return
			}
			res, err := eval.EvalSource(src, e.EvalContext(sh))
			if err != nil {
				values[i] = cell.Error(cell.ErrValue)
				return
			}
			values[i] = res.Value
		}
		if stat.Concurrent && len(wave) > 1 {
			e.pool.Run(len(wave), run)
		} else {
			for i := range wave {
				run(i)
			}
		}
		// Apply sequentially: the sheet's maps are not safe for concurrent
		// writes, and a wave's cells are independent so order does not matter.
		for i, ref := range wave {
			cl := sh.GetRef(ref)
			cl.Value = values[i]
			sh.Set(ref.Row, ref.Col, cl)
		}
		stat.Evaluated += len(wave)
		stat.Waves++
	}

	e.record(stat, start)
	return stat
}

func (e *Engine) record(stat Stats, start time.Time) {
	stat.Duration = time.Since(start)
	e.mu.Lock()
	e.lastStat = stat
	e.mu.Unlock()
}

func (e *Engine) setErr(err error) {
	e.mu.Lock()
	e.lastErr = err
	e.mu.Unlock()
}

// LastError returns the most recent formula syntax error, for the status bar.
func (e *Engine) LastError() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastErr
}

// indexRefs converts evaluated references into graph edges. Ranges are expanded
// so that editing a single cell inside one invalidates the formula reading it.
// A range too large to index marks the formula "broad" instead.
func (e *Engine) indexRefs(sh *grid.Sheet, refs []eval.Ref) ([]grid.Ref, bool) {
	var out []grid.Ref
	broad := false
	for _, r := range refs {
		if r.Sheet != "" && !sameSheetName(e.wb, sh, r.Sheet) {
			// Cross-sheet references are not evaluated in v1 (D12), so they
			// contribute no local edges.
			continue
		}
		rows := int(r.To.Row) - int(r.From.Row) + 1
		cols := int(r.To.Col) - int(r.From.Col) + 1
		if rows <= 0 || cols <= 0 {
			continue
		}
		if rows*cols > e.maxIndexed {
			broad = true
			continue
		}
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				out = append(out, grid.Ref{Row: row, Col: col})
			}
		}
	}
	return out, broad
}

func sameSheetName(wb *grid.Workbook, sh *grid.Sheet, name string) bool {
	other, ok := wb.SheetByName(name)
	return ok && other == sh
}

// RebuildGraph re-registers every formula on every sheet and repairs literal
// cells. It is the "just loaded a file" path, where formulas exist only as
// source text and must be both indexed and recomputed.
//
// The repair is cheap insurance: a literal cell with a source but no value, or
// a value but no source, is normalised rather than left blank.
func (e *Engine) RebuildGraph() {
	// Start from nothing: a formula that has since been overwritten or deleted
	// must not survive in the graph.
	e.graph.Reset()
	for _, sh := range e.wb.Sheets() {
		sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
			if !cl.IsFormula() {
				// A cell with text but no value is repaired by inference. The
				// reverse repair is deliberately absent: reconstructing the
				// source from the display would be lossy for currency.
				if cl.Value.IsEmpty() && cl.Source != "" {
					cl.Value = cell.Infer(cl.Source)
					sh.Set(ref.Row, ref.Col, cl)
				}
				return true
			}
			res, err := eval.EvalSource(cl.Source, e.EvalContext(sh))
			if err != nil {
				e.graph.SetFormula(sh.ID, ref, cl.Source, nil, false)
				return true
			}
			prec, broad := e.indexRefs(sh, res.Refs)
			e.graph.SetFormula(sh.ID, ref, cl.Source, prec, broad)
			return true
		})
	}
}

// PutQuiet stores a cell without recalculating, for bulk operations such as a
// paste. The caller must follow up with RebuildGraph and a recalculation, so
// that a 5,000-cell paste costs one pass rather than 5,000.
func (e *Engine) PutQuiet(sh *grid.Sheet, ref grid.Ref, cl cell.Cell) {
	if cl.IsFormula() {
		// Store the source; the value is computed by the recalculation.
		sh.Set(ref.Row, ref.Col, cell.Cell{Source: cl.Source})
		return
	}
	sh.Set(ref.Row, ref.Col, cl)
}

// RecalcWorkbook recomputes every formula in every sheet, in a deterministic
// order, used once after loading a file.
func (e *Engine) RecalcWorkbook(ctx context.Context) Stats {
	e.RebuildGraph()
	total := Stats{}
	start := time.Now()
	for _, sh := range e.wb.Sheets() {
		st := e.RecalcAll(ctx, sh)
		total.Evaluated += st.Evaluated
		total.Waves += st.Waves
		total.Cycles += st.Cycles
		total.Broad += st.Broad
		if st.Concurrent {
			total.Concurrent = true
		}
	}
	total.Duration = time.Since(start)
	return total
}

// SortedRefs is a small helper used by tests and diagnostics.
func SortedRefs(rs []grid.Ref) []grid.Ref {
	out := append([]grid.Ref(nil), rs...)
	sortRefs(out)
	return out
}

// ---------------------------------------------------------------------------
// Worker pool
// ---------------------------------------------------------------------------

// pool is a fixed set of goroutines shared by every recalculation in the
// program, so an edit never pays for spawning workers.
type pool struct {
	jobs chan func()
	wg   sync.WaitGroup
}

func newPool(n int) *pool {
	p := &pool{jobs: make(chan func())}
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for job := range p.jobs {
				job()
			}
		}()
	}
	return p
}

// Run executes fn(i) for every i in [0,n) across the pool and waits.
func (p *pool) Run(n int, fn func(int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		p.jobs <- func() {
			defer wg.Done()
			fn(i)
		}
	}
	wg.Wait()
}

func (p *pool) Close() { close(p.jobs); p.wg.Wait() }
