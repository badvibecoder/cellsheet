// Package perf turns the performance budgets in PHASE-1-SPEC.md §16 into
// executable assertions.
//
// The point is not to make the test suite slow: it is that a budget nobody
// measures is a wish. These run in a few seconds and are skipped by -short.
package perf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/clipboard"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/sheetfile"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/screens"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"

	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

// measure runs fn repeats times and reports the fastest run, which is the
// closest in-process approximation of the machine's actual capability. The
// budgets are about what the program can do, not what a loaded build machine
// does on its worst pass.
func measure(t *testing.T, name string, budget time.Duration, repeats int, fn func()) time.Duration {
	t.Helper()
	if testing.Short() {
		t.Skip("performance budgets are skipped in short mode")
	}
	if raceEnabled {
		t.Skip("performance budgets describe the shipped binary; the race detector " +
			"changes them by an order of magnitude, so run without -race")
	}
	best := time.Duration(1<<62 - 1)
	for i := 0; i < repeats; i++ {
		start := time.Now()
		fn()
		if d := time.Since(start); d < best {
			best = d
		}
	}
	status := "ok"
	if best > budget {
		status = "OVER BUDGET"
		t.Errorf("%s took %v, budget is %v", name, best, budget)
	}
	t.Logf("%-46s %10v   budget %-10v %s", name, best.Round(time.Microsecond), budget, status)
	return best
}

func newScreen(t *testing.T, w, h int) *screens.Model {
	t.Helper()
	wb := grid.NewWorkbook()
	eng := calc.New(wb)
	t.Cleanup(eng.Close)
	hist := checkpoint.New(time.Now(), 3*time.Minute)
	m := screens.New(wb, eng, theme.Plain(), render.ColorNone, hist)
	m.Width, m.Height = w, h
	m.Rec = journal.NewRecorder()
	m.Undo = checkpoint.NewUndoStack(1000)
	m.Clip = &clipboard.Buffer{}
	return m
}

// TestRenderBudget covers "keystroke to screen update" and "scroll one row".
func TestRenderBudget(t *testing.T) {
	m := newScreen(t, 120, 40)
	for r := uint32(0); r < 500; r++ {
		m.Sheet().Set(r, 0, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
		m.Sheet().Set(r, 1, cell.Cell{Source: "Operations", Value: cell.Infer("Operations")})
		m.Sheet().Set(r, 2, cell.Cell{Source: "=SUM(A1:B1)", Value: cell.Number(cell.FromInt64(0))})
	}
	measure(t, "render 120x40", 16*time.Millisecond, 200, func() { _ = m.View() })

	// Driven through the real key path, so this measures what a user feels
	// when they hold an arrow key down.
	i := 0
	measure(t, "scroll one row and render", 8*time.Millisecond, 200, func() {
		i++
		if i%40 == 0 {
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlHome})
		}
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		_ = m.View()
	})
}

// TestEditRecalcBudget covers "recalculation after a single edit".
func TestEditRecalcBudget(t *testing.T) {
	wb := grid.NewWorkbook()
	eng := calc.New(wb)
	defer eng.Close()
	sh := wb.Active()
	eng.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, "1")
	for r := uint32(1); r < 100; r++ {
		eng.Set(context.Background(), sh, grid.Ref{Row: r, Col: 0}, fmt.Sprintf("=A%d+1", r))
	}
	i := 0
	measure(t, "one edit through a 100-deep chain", 2*time.Millisecond, 300, func() {
		i++
		eng.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, fmt.Sprintf("%d", i+2))
	})
}

// TestFullRecalcBudget covers "full recalculation of 10,000 formulas".
func TestFullRecalcBudget(t *testing.T) {
	wb := grid.NewWorkbook()
	eng := calc.New(wb)
	defer eng.Close()
	sh := wb.Active()
	sh.GrowToFit(30, 400)
	sh.Set(0, 0, cell.Cell{Source: "2", Value: cell.Infer("2")})
	for i := 0; i < 10000; i++ {
		sh.Set(uint32(i/400)+1, uint32(i%400), cell.Cell{Source: "=$A$1*2"})
	}
	st := eng.RecalcWorkbook(context.Background())
	t.Logf("workbook: %d sheet(s), %d populated cells, %d formulas registered",
		wb.Len(), wb.CellCount(), eng.Graph().FormulaCount(sh.ID))
	t.Logf("one pass: %d formulas, %d waves, concurrent=%v in %v",
		st.Evaluated, st.Waves, st.Concurrent, st.Duration.Round(time.Microsecond))

	measure(t, "full recalculation of 10,000 formulas", 500*time.Millisecond, 5, func() {
		eng.RecalcWorkbook(context.Background())
	})
}

// TestSaveLoadBudget covers the 1 MB / 50,000-cell load and save budgets.
func TestSaveLoadBudget(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	sh.GrowToFit(500, 100)
	n := 0
	for r := uint32(0); r < 500 && n < 50000; r++ {
		for c := uint32(0); c < 100 && n < 50000; c++ {
			switch {
			case c%4 == 0:
				sh.Set(r, c, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
			case c%4 == 1:
				sh.Set(r, c, cell.Cell{Source: "Operations", Value: cell.Infer("Operations")})
			case c%4 == 2:
				sh.Set(r, c, cell.Cell{Source: "=SUM(A1:B1)", Value: cell.Number(cell.FromInt64(0))})
			default:
				sh.Set(r, c, cell.Cell{Source: "450", Value: cell.Infer("450")})
			}
			n++
		}
	}
	t.Logf("workbook: %d populated cells", wb.CellCount())

	dir := t.TempDir()
	path := filepath.Join(dir, "perf.cell")
	doc := &sheetfile.Document{Workbook: wb, AppVersion: "perf", Created: time.Now()}

	measure(t, "save 50,000 cells", 200*time.Millisecond, 5, func() {
		if err := sheetfile.Save(path, doc); err != nil {
			t.Fatal(err)
		}
	})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("file size: %d bytes", info.Size())

	measure(t, "load 50,000 cells", 300*time.Millisecond, 5, func() {
		if _, err := sheetfile.Load(path); err != nil {
			t.Fatal(err)
		}
	})

	// Opening a workbook also means recalculating it, which is what the user
	// actually waits for.
	measure(t, "open: load and recalculate", 500*time.Millisecond, 5, func() {
		d, err := sheetfile.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		e := calc.New(d.Workbook)
		e.RecalcWorkbook(context.Background())
		e.Close()
	})
}

// TestMemoryBudget covers "100,000 populated cells in under 60 MB".
func TestMemoryBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("performance budgets are skipped in short mode")
	}
	if raceEnabled {
		t.Skip("the race detector changes allocation behaviour; run without -race")
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	wb := grid.NewWorkbook()
	sh := wb.Active()
	sh.GrowToFit(1000, 100)
	for r := uint32(0); r < 1000; r++ {
		for c := uint32(0); c < 100; c++ {
			if c%3 == 0 {
				sh.Set(r, c, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
			} else {
				sh.Set(r, c, cell.Cell{Source: "a label", Value: cell.Infer("a label")})
			}
		}
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	used := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	const budget = 60 << 20
	t.Logf("%-46s %7.1f MB   budget %-10s %s", "100,000 populated cells",
		float64(used)/(1<<20), "60 MB", status(used, budget))
	if used > budget {
		t.Errorf("100,000 cells used %.1f MB, budget is 60 MB", float64(used)/(1<<20))
	}
	if got := wb.CellCount(); got != 100000 {
		t.Errorf("built %d cells, wanted 100,000", got)
	}
}

func status(used, budget int64) string {
	if used > budget {
		return "OVER BUDGET"
	}
	return "ok"
}
