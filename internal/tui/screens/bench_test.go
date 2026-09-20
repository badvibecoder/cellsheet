package screens

import (
	"fmt"
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/clipboard"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"
)

// benchModel builds a model of a given size without any test framework state.
func benchModel(b *testing.B, w, h int) *Model {
	b.Helper()
	wb := grid.NewWorkbook()
	eng := calc.New(wb)
	b.Cleanup(eng.Close)
	hist := checkpoint.New(time.Date(2026, 3, 4, 22, 0, 0, 0, time.UTC), 3*time.Minute)
	m := New(wb, eng, theme.Plain(), render.ColorNone, hist)
	m.Width, m.Height = w, h
	m.Rec = journal.NewRecorder()
	m.Undo = checkpoint.NewUndoStack(1000)
	m.Clip = &clipboard.Buffer{}
	m.OnEdit = func(kind journal.Kind, label string) {
		m.Rec.Build(wb, kind, label, time.Now())
	}
	return m
}

// BenchmarkRender is the budget that matters most: a keystroke must reach the
// screen inside one frame. The §16 budget is 16 ms, and this renders a whole
// 120x40 frame.
func BenchmarkRender(b *testing.B) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
		b.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(b *testing.B) {
			m := benchModel(b, size[0], size[1])
			// A realistically busy sheet, so the renderer is not measuring
			// an empty grid.
			for r := uint32(0); r < 200; r++ {
				m.Sheet().Set(r, 0, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
				m.Sheet().Set(r, 1, cell.Cell{Source: "Operations", Value: cell.Infer("Operations")})
				m.Sheet().Set(r, 2, cell.Cell{Source: "=SUM(A1:B1)", Value: cell.Number(cell.FromInt64(0))})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.View()
			}
		})
	}
}

// BenchmarkRenderScrolled measures the same frame after moving the cursor, which
// is what happens on every arrow key.
func BenchmarkRenderScrolled(b *testing.B) {
	m := benchModel(b, 120, 40)
	for r := uint32(0); r < 2000; r++ {
		m.Sheet().Set(r, 0, cell.Cell{Source: "value", Value: cell.Text("value")})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Cur = grid.Ref{Row: uint32(i % 2000), Col: 0}
		m.ensureVisible()
		_ = m.View()
	}
}
