package calc

import (
	"context"
	"fmt"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"

	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

func cellSource(src string) cell.Cell { return cell.Cell{Source: src} }
func cellValue(src string) cell.Cell {
	return cell.Cell{Source: src, Value: cell.Infer(src)}
}

// BenchmarkRecalcChain measures one edit rippling down a 100-deep dependency
// chain, which is the cost of typing into a live spreadsheet. The PHASE-1-SPEC
// §16 budget is 2 ms.
func BenchmarkRecalcChain(b *testing.B) {
	wb := grid.NewWorkbook()
	e := New(wb)
	b.Cleanup(e.Close)
	sh := wb.Active()

	e.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, "1")
	for r := uint32(1); r < 100; r++ {
		e.Set(context.Background(), sh, grid.Ref{Row: r, Col: 0}, fmt.Sprintf("=A%d+1", r))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, fmt.Sprintf("%d", i+2))
	}
}

// BenchmarkRecalcWide measures many independent formulas recomputing at once,
// which is where the worker pool earns its keep. The §16 budget for 10,000
// formulas is 500 ms.
func BenchmarkRecalcWide(b *testing.B) {
	wb := grid.NewWorkbook()
	e := New(wb)
	b.Cleanup(e.Close)
	sh := wb.Active()
	sh.GrowToFit(30, 400)

	e.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, "2")
	for i := 0; i < 10000; i++ {
		r := uint32(i/400) + 1
		c := uint32(i % 400)
		e.Set(context.Background(), sh, grid.Ref{Row: r, Col: c}, "=$A$1*2")
	}
	st := e.LastStats()
	b.Logf("initial build: %d formulas, %d waves, concurrent=%v", st.Evaluated, st.Waves, st.Concurrent)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Set(context.Background(), sh, grid.Ref{Row: 0, Col: 0}, fmt.Sprintf("%d", i+3))
	}
	b.StopTimer()
	last := e.LastStats()
	b.ReportMetric(float64(last.Evaluated), "cells")
	b.ReportMetric(float64(last.Waves), "waves")
	b.ReportMetric(float64(last.Duration.Microseconds()), "us")
}

// BenchmarkFullRecalc measures rebuilding every formula, which is what opening
// a file does. The §16 budget for 10,000 formulas is 500 ms.
func BenchmarkFullRecalc(b *testing.B) {
	wb := grid.NewWorkbook()
	e := New(wb)
	b.Cleanup(e.Close)
	sh := wb.Active()
	sh.GrowToFit(30, 400)
	sh.Set(0, 0, cellValue("2"))
	for i := 0; i < 10000; i++ {
		r := uint32(i/400) + 1
		c := uint32(i % 400)
		sh.Set(r, c, cellSource("=$A$1*2"))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.RecalcWorkbook(context.Background())
	}
}
