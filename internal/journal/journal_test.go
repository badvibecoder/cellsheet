package journal

import (
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

func setCell(sh *grid.Sheet, ref grid.Ref, typed string) cell.Cell {
	cl := cell.Cell{Source: typed, Value: cell.Infer(typed)}
	sh.Set(ref.Row, ref.Col, cl)
	return cl
}

func TestRecorderDiffsACell(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	rec := NewRecorder()
	rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
	setCell(sh, ref, "$500.00")

	d := rec.Build(wb, KindCellEdited, "Cell A1 edited", time.Now())
	if d.Empty() {
		t.Fatal("the change was not recorded")
	}
	if len(d.Cells) != 1 {
		t.Fatalf("recorded %d cell changes, want 1", len(d.Cells))
	}
	c := d.Cells[0]
	if c.Before.Value.Display() != "1" || c.After.Value.Display() != "$500.00" {
		t.Errorf("before=%q after=%q", c.Before.Value.Display(), c.After.Value.Display())
	}
}

// TestRecorderIgnoresNoOps keeps the history free of entries that change
// nothing, which is what lets the timer skip an empty checkpoint.
func TestRecorderIgnoresNoOps(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	rec := NewRecorder()
	rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
	// Write the same thing back.
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	if d := rec.Build(wb, KindCellEdited, "no-op", time.Now()); !d.Empty() {
		t.Errorf("writing the same value must not produce a delta: %+v", d.Cells)
	}
}

func TestDeltaForwardAndReverse(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	rec := NewRecorder()
	rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
	setCell(sh, ref, "42")
	d := rec.Build(wb, KindCellEdited, "edit", time.Now())

	if got := sh.Get(0, 0).Value.Display(); got != "42" {
		t.Fatalf("A1 = %s", got)
	}
	d.ApplyReverse(wb)
	if got := wb.Active().Get(0, 0).Value.Display(); got != "1" {
		t.Errorf("after reverse, A1 = %s, want 1", got)
	}
	d.ApplyForward(wb)
	if got := wb.Active().Get(0, 0).Value.Display(); got != "42" {
		t.Errorf("after forward, A1 = %s, want 42", got)
	}
}

func TestDeltaReverseIsItsOwnInverse(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	rec := NewRecorder()
	rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
	setCell(sh, ref, "9")
	d := rec.Build(wb, KindCellEdited, "edit", time.Now())

	r := d.Reverse()
	r.ApplyForward(wb) // same as d.ApplyReverse
	if got := wb.Active().Get(0, 0).Value.Display(); got != "1" {
		t.Errorf("Reversed().ApplyForward gave %s, want 1", got)
	}
}

func TestRowAndColumnResizeIsRecorded(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()

	rec := NewRecorder()
	rec.BeforeRow(sh.ID, 2, sh.RowHeight(2))
	sh.SetRowHeight(2, 5)
	rec.BeforeCol(sh.ID, 1, sh.ColWidth(1))
	sh.SetColWidth(1, 30)

	d := rec.Build(wb, KindStructural, "resize", time.Now())
	if len(d.Rows) != 1 || d.Rows[0].Before != 1 || d.Rows[0].After != 5 {
		t.Errorf("row change = %+v", d.Rows)
	}
	if len(d.Cols) != 1 || d.Cols[0].Before != 18 || d.Cols[0].After != 30 {
		t.Errorf("column change = %+v", d.Cols)
	}
	d.ApplyReverse(wb)
	if got := wb.Active().RowHeight(2); got != 1 {
		t.Errorf("after reverse, row height = %d, want 1", got)
	}
	if got := wb.Active().ColWidth(1); got != 18 {
		t.Errorf("after reverse, column width = %d, want 18", got)
	}
}

func TestStructuralSnapshot(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	for r := uint32(0); r < 5; r++ {
		setCell(sh, grid.Ref{Row: r, Col: 0}, string(rune('a'+r)))
	}
	rec := NewRecorder()
	rec.BeforeSnapshot(sh)

	// Simulate a delete-row: everything below row 1 shifts up.
	for r := uint32(1); r < 4; r++ {
		sh.Set(r, 0, sh.Get(r+1, 0))
	}
	sh.Clear(4, 0)

	d := rec.Build(wb, KindStructural, "Row 2 deleted", time.Now())
	if len(d.Snapshots) != 1 {
		t.Fatalf("expected one snapshot change, got %d", len(d.Snapshots))
	}
	d.ApplyReverse(wb)
	for r := uint32(0); r < 5; r++ {
		want := string(rune('a' + r))
		if got := wb.Active().Get(r, 0).Value.Display(); got != want {
			t.Errorf("after reverse, row %d = %q, want %q", r, got, want)
		}
	}
}

// TestCombineFoldsARunOfEdits is what makes an automatic checkpoint cheap: the
// earliest "before" and the latest "after" is all that matters.
func TestCombineFoldsARunOfEdits(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "0", Value: cell.Number(cell.FromInt64(0))})

	var deltas []Delta
	for i := 1; i <= 5; i++ {
		rec := NewRecorder()
		rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
		setCell(sh, ref, string(rune('0'+i)))
		deltas = append(deltas, rec.Build(wb, KindCellEdited, "edit", time.Now()))
	}
	combined := Combine(deltas)
	if len(combined.Cells) != 1 {
		t.Fatalf("combined into %d cell changes, want 1", len(combined.Cells))
	}
	c := combined.Cells[0]
	if c.Before.Value.Display() != "0" {
		t.Errorf("combined Before = %q, want the earliest state 0", c.Before.Value.Display())
	}
	if c.After.Value.Display() != "5" {
		t.Errorf("combined After = %q, want the latest state 5", c.After.Value.Display())
	}
	// Applying it in reverse must reach the original state in one step.
	combined.ApplyReverse(wb)
	if got := wb.Active().Get(0, 0).Value.Display(); got != "0" {
		t.Errorf("after reversing the combined delta, A1 = %s, want 0", got)
	}
}

func TestCombineDropsNetNoOps(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	sh.Set(0, 0, cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))})

	// Change to 2 and back to 1.
	var deltas []Delta
	for _, v := range []string{"2", "1"} {
		rec := NewRecorder()
		rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
		setCell(sh, ref, v)
		deltas = append(deltas, rec.Build(wb, KindCellEdited, "edit", time.Now()))
	}
	if got := Combine(deltas); !got.Empty() {
		t.Errorf("a change that ended where it started should combine to nothing: %+v", got.Cells)
	}
}

func TestWorkbookStateRoundTrip(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	setCell(sh, grid.Ref{Row: 0, Col: 0}, "$10.00")
	sh.SetRowHeight(1, 4)
	if _, err := wb.AddSheet("Two"); err != nil {
		t.Fatal(err)
	}
	wb.Sheets()[1].Set(0, 0, cell.Cell{Source: "hi", Value: cell.Text("hi")})
	wb.SetActive(1)

	st := CaptureWorkbook(wb)
	if !st.Same(CaptureWorkbook(wb)) {
		t.Error("a snapshot should equal itself")
	}
	// Wipe the workbook and restore.
	for wb.Len() > 0 {
		wb.RemoveSheetAt(0)
	}
	st.Restore(wb)
	if wb.Len() != 2 || wb.ActiveIndex() != 1 {
		t.Fatalf("restored %d sheets, active %d", wb.Len(), wb.ActiveIndex())
	}
	if got := wb.Sheets()[0].Get(0, 0).Value.Display(); got != "$10.00" {
		t.Errorf("A1 = %q", got)
	}
	if got := wb.Sheets()[0].RowHeight(1); got != 4 {
		t.Errorf("row height = %d, want 4", got)
	}
	if got := wb.Sheets()[1].Name; got != "Two" {
		t.Errorf("sheet name = %q", got)
	}
}

func TestSheetLifecycleRecording(t *testing.T) {
	wb := grid.NewWorkbook()
	rec := NewRecorder()
	rec.BeforeSheetAdd(1, SheetState{ID: 2, Name: "Two", Rows: 100, Cols: 26, DefRowH: 1, DefColW: 18})
	if _, err := wb.AddSheet("Two"); err != nil {
		t.Fatal(err)
	}
	d := rec.Build(wb, KindStructural, "Sheet added", time.Now())
	if len(d.Added) != 1 {
		t.Fatalf("added = %d, want 1", len(d.Added))
	}
	d.ApplyReverse(wb)
	if wb.Len() != 1 {
		t.Errorf("reversing a sheet add should remove it, Len = %d", wb.Len())
	}
	d.ApplyForward(wb)
	if wb.Len() != 2 {
		t.Errorf("reapplying should add it back, Len = %d", wb.Len())
	}
}

func TestRenameRecording(t *testing.T) {
	wb := grid.NewWorkbook()
	rec := NewRecorder()
	rec.BeforeSheetRename(wb.Sheets()[0].ID, "Sheet1")
	if err := wb.RenameSheet(0, "Budget"); err != nil {
		t.Fatal(err)
	}
	d := rec.Build(wb, KindStructural, "rename", time.Now())
	if len(d.Renamed) != 1 || d.Renamed[0].After != "Budget" {
		t.Fatalf("rename = %+v", d.Renamed)
	}
	d.ApplyReverse(wb)
	if wb.Sheets()[0].Name != "Sheet1" {
		t.Errorf("after reverse, name = %q", wb.Sheets()[0].Name)
	}
}
