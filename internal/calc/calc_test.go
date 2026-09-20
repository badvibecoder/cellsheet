package calc

import (
	"context"
	"fmt"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"

	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

func newEngine(t *testing.T) (*grid.Workbook, *grid.Sheet, *Engine) {
	t.Helper()
	wb := grid.NewWorkbook()
	e := New(wb)
	e.SetThreshold(1) // take the parallel path even for tiny sheets, so tests exercise it
	t.Cleanup(e.Close)
	return wb, wb.Active(), e
}

func mustRef(t *testing.T, s string) grid.Ref {
	t.Helper()
	r, ok := grid.ParseRef(s)
	if !ok {
		t.Fatalf("bad ref %q", s)
	}
	return r
}

func set(t *testing.T, e *Engine, sh *grid.Sheet, ref, typed string) cell.Value {
	t.Helper()
	return e.Set(context.Background(), sh, mustRef(t, ref), typed)
}

func show(t *testing.T, sh *grid.Sheet, ref string) string {
	t.Helper()
	return sh.GetRef(mustRef(t, ref)).Value.Display()
}

// TestDependencyTracking proves that a change recomputes only what depends on
// it — the whole reason the graph exists.
func TestDependencyTracking(t *testing.T) {
	_, sh, e := newEngine(t)

	set(t, e, sh, "A1", "1")
	set(t, e, sh, "B1", "=A1*2")  // depends on A1
	set(t, e, sh, "C1", "=B1+1")  // depends on B1
	set(t, e, sh, "D1", "=C1*2")  // depends on C1
	set(t, e, sh, "E1", "5")      // plain value
	set(t, e, sh, "F1", "=E1*3")  // depends on E1 only
	set(t, e, sh, "G1", "=10+10") // depends on nothing

	if got := show(t, sh, "D1"); got != "6" {
		t.Fatalf("D1 = %s, want 6", got)
	}
	if got := show(t, sh, "F1"); got != "15" {
		t.Fatalf("F1 = %s, want 15", got)
	}

	// Change A1 and check the chain follows. Setting is what triggers the
	// recalculation, and only the three dependents may be touched.
	set(t, e, sh, "A1", "2")
	if got := show(t, sh, "D1"); got != "10" {
		t.Errorf("after A1=1->2, D1 = %s, want 10", got)
	}
	if st := e.LastStats(); st.Evaluated != 3 {
		t.Errorf("changing A1 evaluated %d cells, want exactly 3 (B1, C1, D1)", st.Evaluated)
	}

	// Change E1: only F1 should move.
	set(t, e, sh, "E1", "10")
	if got := show(t, sh, "F1"); got != "30" {
		t.Errorf("F1 = %s, want 30", got)
	}
	if got := show(t, sh, "D1"); got != "10" {
		t.Errorf("D1 should be unaffected, got %s", got)
	}
}

// TestWavesAreOrdered checks the wave planner on a chain: each link must be its
// own wave, because each depends on the one before.
func TestWavesAreOrdered(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "1")
	set(t, e, sh, "B1", "=A1+1")
	set(t, e, sh, "C1", "=B1+1")
	set(t, e, sh, "D1", "=C1+1")

	dirty := e.Graph().Dependents(sh.ID, []grid.Ref{mustRef(t, "A1")})
	waves, cyclic := e.Graph().Waves(sh.ID, dirty)
	if len(cyclic) != 0 {
		t.Fatalf("unexpected cycle: %v", cyclic)
	}
	if len(waves) != 3 {
		t.Fatalf("a three-link chain should be three waves, got %d: %v", len(waves), waves)
	}
	for i, w := range waves {
		if len(w) != 1 {
			t.Errorf("wave %d has %d cells, want 1", i, len(w))
		}
	}
}

// TestIndependentCellsShareAWave is the parallel case: cells with no dependency
// on each other must land in the same wave.
func TestIndependentCellsSharingAWave(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "1")
	for i := 0; i < 10; i++ {
		ref := fmt.Sprintf("B%d", i+1)
		set(t, e, sh, ref, fmt.Sprintf("=A1+%d", i))
	}

	dirty := e.Graph().Dependents(sh.ID, []grid.Ref{mustRef(t, "A1")})
	waves, _ := e.Graph().Waves(sh.ID, dirty)
	if len(waves) != 1 {
		t.Fatalf("ten independent formulas should be one wave, got %d", len(waves))
	}
	if len(waves[0]) != 10 {
		t.Fatalf("wave should hold 10 cells, got %d", len(waves[0]))
	}
}

func TestRangesRecalculate(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "10")
	set(t, e, sh, "A2", "20")
	set(t, e, sh, "A3", "30")
	set(t, e, sh, "B1", "=SUM(A1:A3)")
	if got := show(t, sh, "B1"); got != "60" {
		t.Fatalf("B1 = %s, want 60", got)
	}
	// Editing a cell in the middle of the range must invalidate the sum.
	set(t, e, sh, "A2", "200")
	if got := show(t, sh, "B1"); got != "240" {
		t.Errorf("B1 = %s, want 240", got)
	}
	// And a cell outside the range must not.
	before := e.LastStats().Evaluated
	set(t, e, sh, "A9", "999")
	if e.LastStats().Evaluated != 0 {
		t.Errorf("editing A9 should not recalculate anything, evaluated %d", before)
	}
}

// TestFormulaChangeUpdatesEdges checks that replacing a formula removes its old
// dependencies, which is the easiest thing to get wrong in a dependency graph.
func TestFormulaChangeUpdatesEdges(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "1")
	set(t, e, sh, "B1", "100")
	set(t, e, sh, "C1", "=A1+1")
	if got := show(t, sh, "C1"); got != "2" {
		t.Fatalf("C1 = %s, want 2", got)
	}
	// Point C1 at B1 instead. A1 must no longer affect it.
	set(t, e, sh, "C1", "=B1+1")
	if got := show(t, sh, "C1"); got != "101" {
		t.Fatalf("C1 = %s, want 101", got)
	}
	set(t, e, sh, "A1", "5")
	if got := show(t, sh, "C1"); got != "101" {
		t.Errorf("C1 should no longer depend on A1, got %s", got)
	}
	set(t, e, sh, "B1", "200")
	if got := show(t, sh, "C1"); got != "201" {
		t.Errorf("C1 should depend on B1, got %s", got)
	}
}

func TestCycleBecomesCircError(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "=B1")
	set(t, e, sh, "B1", "=A1")
	if got := show(t, sh, "A1"); got != "#CIRC!" {
		t.Errorf("A1 = %s, want #CIRC!", got)
	}
	if got := show(t, sh, "B1"); got != "#CIRC!" {
		t.Errorf("B1 = %s, want #CIRC!", got)
	}
	// A three-cell cycle, and a self-reference.
	set(t, e, sh, "C1", "=D1")
	set(t, e, sh, "D1", "=E1")
	set(t, e, sh, "E1", "=C1")
	if got := show(t, sh, "E1"); got != "#CIRC!" {
		t.Errorf("E1 = %s, want #CIRC!", got)
	}
	set(t, e, sh, "F1", "=F1+1")
	if got := show(t, sh, "F1"); got != "#CIRC!" {
		t.Errorf("a self-reference should be #CIRC!, got %s", got)
	}
}

// TestDeterminism is the guarantee that makes concurrency safe to reason about:
// the answer must not depend on scheduling.
func TestDeterminism(t *testing.T) {
	build := func() (*grid.Sheet, *Engine) {
		wb := grid.NewWorkbook()
		e := New(wb)
		sh := wb.Active()
		return sh, e
	}

	seqSheet, seqEng := build()
	parSheet, parEng := build()
	defer seqEng.Close()
	defer parEng.Close()
	seqEng.SetThreshold(1 << 30) // always inline
	parEng.SetThreshold(1)       // always parallel

	// A wide, layered sheet: 20 roots feeding 60 formulas feeding 20 more.
	seed := func(e *Engine, sh *grid.Sheet) {
		for i := 0; i < 20; i++ {
			e.Set(context.Background(), sh, grid.Ref{Row: uint32(i), Col: 0}, fmt.Sprintf("%d", i+1))
		}
		for i := 0; i < 60; i++ {
			e.Set(context.Background(), sh, grid.Ref{Row: uint32(i % 20), Col: uint32(1 + i/20)},
				fmt.Sprintf("=A%d*%d", i%20+1, i+2))
		}
		for i := 0; i < 20; i++ {
			e.Set(context.Background(), sh, grid.Ref{Row: uint32(i), Col: 5},
				fmt.Sprintf("=SUM(B%d:D%d)", i+1, i+1))
		}
	}
	seed(seqEng, seqSheet)
	seed(parEng, parSheet)

	// Now change every root and compare the whole grid.
	for i := 0; i < 20; i++ {
		seqEng.Set(context.Background(), seqSheet, grid.Ref{Row: uint32(i), Col: 0}, fmt.Sprintf("%d", (i+1)*7))
		parEng.Set(context.Background(), parSheet, grid.Ref{Row: uint32(i), Col: 0}, fmt.Sprintf("%d", (i+1)*7))
	}

	diffs := 0
	for r := uint32(0); r < 20; r++ {
		for c := uint32(0); c < 8; c++ {
			a := seqSheet.Get(r, c).Value.Display()
			b := parSheet.Get(r, c).Value.Display()
			if a != b {
				diffs++
				if diffs < 5 {
					t.Errorf("cell %s differs: sequential %q, parallel %q", grid.RefName(r, c), a, b)
				}
			}
		}
	}
	if diffs > 0 {
		t.Errorf("%d cells differed between sequential and parallel evaluation", diffs)
	}
}

// TestLargeFanOutUsesThePool exercises the shared worker pool under -race.
func TestLargeFanOutUsesThePool(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "2")
	const n = 500
	for i := 0; i < n; i++ {
		set(t, e, sh, fmt.Sprintf("B%d", i+1), fmt.Sprintf("=A1*%d", i+1))
	}
	st := e.Recalc(context.Background(), sh, []grid.Ref{mustRef(t, "A1")})
	if !st.Concurrent {
		t.Fatal("a 500-cell recalculation should have used the pool")
	}
	if st.Evaluated != n {
		t.Errorf("evaluated %d, want %d", st.Evaluated, n)
	}
	if got := show(t, sh, "B500"); got != "1000" {
		t.Errorf("B500 = %s, want 1000", got)
	}
}

func TestBroadRangeStillRecalculates(t *testing.T) {
	_, sh, e := newEngine(t)
	e.SetMaxIndexedRange(4) // A1:A10 is now too big to index cell by cell
	set(t, e, sh, "A1", "1")
	set(t, e, sh, "A5", "5")
	set(t, e, sh, "C1", "=SUM(A1:A10)")
	if got := show(t, sh, "C1"); got != "6" {
		t.Fatalf("C1 = %s, want 6", got)
	}
	// A broad formula is recomputed on any change, so this must follow.
	set(t, e, sh, "A7", "7")
	if got := show(t, sh, "C1"); got != "13" {
		t.Errorf("C1 = %s, want 13", got)
	}
}

func TestClearRemovesDependency(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "1")
	set(t, e, sh, "B1", "=A1*2")
	if got := show(t, sh, "B1"); got != "2" {
		t.Fatalf("B1 = %s, want 2", got)
	}
	e.Clear(context.Background(), sh, mustRef(t, "B1"))
	if got := show(t, sh, "B1"); got != "" {
		t.Errorf("B1 should be empty, got %q", got)
	}
	if e.Graph().IsFormula(sh.ID, mustRef(t, "B1")) {
		t.Error("the graph still thinks B1 is a formula")
	}
	st := e.Recalc(context.Background(), sh, []grid.Ref{mustRef(t, "A1")})
	if st.Evaluated != 0 {
		t.Errorf("nothing depends on A1 any more, but %d cells were evaluated", st.Evaluated)
	}
}

func TestSyntaxErrorIsStored(t *testing.T) {
	_, sh, e := newEngine(t)
	got := set(t, e, sh, "A1", "=1+")
	if !got.IsError() {
		t.Errorf("a malformed formula should store an error, got %s", got.Display())
	}
	if e.LastError() == nil {
		t.Error("the syntax error should be reported for the status bar")
	}
	// The source must survive so the user can fix it.
	if src := sh.Get(0, 0).Source; src != "=1+" {
		t.Errorf("source was lost: %q", src)
	}
}

func TestRecalcWorkbookAfterLoad(t *testing.T) {
	wb, sh, e := newEngine(t)
	// Simulate a file load: raw sources stored, no graph, no computed values.
	sh.Set(0, 0, cell.Cell{Source: "$10.00"})
	sh.Set(1, 0, cell.Cell{Source: "$20.00"})
	sh.Set(0, 1, cell.Cell{Source: "=SUM(A1:A2)"})
	sh.Set(1, 1, cell.Cell{Source: "=B1*2"})

	st := e.RecalcWorkbook(context.Background())
	if st.Evaluated != 2 {
		t.Errorf("evaluated %d, want 2", st.Evaluated)
	}
	if got := show(t, sh, "B1"); got != "$30.00" {
		t.Errorf("B1 = %s, want $30.00", got)
	}
	if got := show(t, sh, "B2"); got != "$60.00" {
		t.Errorf("B2 = %s, want $60.00", got)
	}
	// Non-formula sources are inferred, not evaluated, on load.
	if sh.Get(0, 0).Value.Display() != "$10.00" {
		t.Errorf("A1 = %s", sh.Get(0, 0).Value.Display())
	}
	_ = wb
}

// TestMinusEqualsFormulaStaysLive: a "-=..." formula must behave exactly like
// its long form, including staying wired into the dependency graph.
func TestMinusEqualsFormulaStaysLive(t *testing.T) {
	_, sh, e := newEngine(t)
	set(t, e, sh, "A1", "10")
	set(t, e, sh, "A2", "20")
	set(t, e, sh, "B1", "-=SUM(A1:A2)")
	if got := show(t, sh, "B1"); got != "-30" {
		t.Fatalf("B1 = %q, want -30", got)
	}
	set(t, e, sh, "A1", "100")
	if got := show(t, sh, "B1"); got != "-120" {
		t.Errorf("B1 = %q after editing A1, want -120; the formula is not live", got)
	}
	if !e.Graph().IsFormula(sh.ID, mustRef(t, "B1")) {
		t.Error("the graph does not know B1 is a formula")
	}
}
