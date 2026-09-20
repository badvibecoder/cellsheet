package checkpoint

import (
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// edit applies a change to a cell and returns the delta describing it.
func edit(wb *grid.Workbook, sh *grid.Sheet, ref grid.Ref, typed string, at time.Time) journal.Delta {
	before := sh.GetRef(ref)
	cl := cell.Cell{Source: typed, Value: cell.Infer(typed)}
	sh.Set(ref.Row, ref.Col, cl)
	return journal.Delta{
		Kind:  KindCellEdited,
		Label: "Cell edited",
		At:    at,
		Cells: []journal.CellChange{{Sheet: sh.ID, Ref: ref, Before: before, After: cl}},
	}
}

func TestRingSemantics(t *testing.T) {
	base := time.Date(2026, 3, 4, 21, 47, 12, 0, time.Local)
	h := New(base, 3*time.Minute)

	if h.Len() != 0 {
		t.Fatalf("a new history has no rollback steps, got %d", h.Len())
	}
	if got := h.Label(0); got != "Current State" {
		t.Errorf("index 0 label = %q, want Current State", got)
	}

	h.Push(journal.Delta{Kind: KindRowEdited, Label: "Row 4 edited", At: base.Add(time.Minute)})
	if h.Len() != 1 {
		t.Fatalf("after one checkpoint, Len = %d, want 1", h.Len())
	}
	if got := h.Label(1); got != "Row 4 edited" {
		t.Errorf("index 1 label = %q", got)
	}
	if got := h.Label(0); got != "Current State" {
		t.Errorf("index 0 should still be the current state, got %q", got)
	}
	if !h.When(1).Equal(base.Add(time.Minute)) {
		t.Errorf("timestamp not recorded: %v", h.When(1))
	}
}

// TestDueOnlyWhenChanged is the "only when a change is detected" requirement.
func TestDueOnlyWhenChanged(t *testing.T) {
	base := time.Date(2026, 3, 4, 21, 47, 12, 0, time.Local)
	h := New(base, 3*time.Minute)

	if h.Due(base.Add(10 * time.Minute)) {
		t.Error("an unchanged workbook must not produce a checkpoint")
	}
	if h.Len() != 0 {
		t.Errorf("Len = %d, want 0", h.Len())
	}

	h.TouchAt(base)
	if h.Due(base.Add(time.Minute)) {
		t.Error("the interval has not elapsed yet")
	}
	if !h.Due(base.Add(4 * time.Minute)) {
		t.Error("after the interval and a change, a checkpoint is due")
	}
	h.Push(journal.Delta{Kind: KindAuto, Label: "Auto-Checkpoint", At: base.Add(4 * time.Minute)})
	if h.Len() != 1 {
		t.Errorf("Len = %d, want 1", h.Len())
	}
	if h.Dirty() {
		t.Error("a checkpoint clears the changed flag")
	}
	if h.Due(base.Add(4 * time.Minute)) {
		t.Error("the timer should have restarted")
	}
}

func TestRingRollsOverAt99(t *testing.T) {
	base := time.Date(2026, 3, 4, 21, 47, 12, 0, time.Local)
	h := New(base, time.Minute)
	for i := 1; i <= 150; i++ {
		h.Push(journal.Delta{Kind: KindAuto, Label: "Auto-Checkpoint", At: base.Add(time.Duration(i) * time.Minute)})
	}
	if h.Len() != Max {
		t.Fatalf("Len = %d, want %d; the ring must not grow without bound", h.Len(), Max)
	}
	if h.Label(Max) == "" {
		t.Error("the last index should have a label")
	}
	if h.Label(Max+1) != "" {
		t.Error("an out-of-range index should return nothing")
	}
}

// fresh3 builds a workbook whose A1 has been edited to "1", "2" then "3", with
// a history containing those three steps. base is a fixed clock.
func fresh3(t *testing.T) (*grid.Workbook, *History, func() string) {
	t.Helper()
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	h := New(base, time.Minute)
	sh.Set(0, 0, cell.Cell{Source: "0", Value: cell.Number(cell.FromInt64(0))})
	for i := 1; i <= 3; i++ {
		h.Push(edit(wb, sh, ref, itoa(i), base.Add(time.Duration(i)*time.Minute)))
	}
	return wb, h, func() string { return wb.Active().Get(0, 0).Value.Display() }
}

// TestRollbackIndexIsStepsBack is the core behaviour: index k is k steps into
// the past, and index 1 is the immediate revert the brief asks for.
func TestRollbackIndexIsStepsBack(t *testing.T) {
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	for k := 1; k <= 3; k++ {
		wb, h, a1 := fresh3(t)
		if err := h.Rollback(wb, k, base.Add(10*time.Minute)); err != nil {
			t.Fatalf("rollback %d: %v", k, err)
		}
		if want := itoa(3 - k); a1() != want {
			t.Errorf("rollback index %d gave A1 = %s, want %s", k, a1(), want)
		}
	}
}

// TestRollbackIsReversible is decision D13: a mis-click must never destroy work.
func TestRollbackIsReversible(t *testing.T) {
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	wb, h, a1 := fresh3(t)

	if err := h.Rollback(wb, 3, base.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := a1(); got != "0" {
		t.Fatalf("after rolling back 3, A1 = %s, want 0", got)
	}
	// Index 1 now holds the state we rolled back from.
	if got := h.Label(1); got != "Rolled back to index 3" {
		t.Errorf("index 1 label = %q", got)
	}
	// Rolling back one step returns us to it: the rollback is itself undoable.
	if err := h.Rollback(wb, 1, base.Add(11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := a1(); got != "3" {
		t.Errorf("undoing the rollback gave A1 = %s, want 3", got)
	}
}

// TestRollbackKeepsTheOlderHistory: a rollback consumes the steps it walked
// through, but everything older survives, and the rollback itself becomes one
// undoable step.
func TestRollbackKeepsTheOlderHistory(t *testing.T) {
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	wb, h, _ := fresh3(t)
	before := h.Len() // three edits
	if err := h.Rollback(wb, 2, base.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Two steps were consumed and one ("undo the rollback") was added.
	if want := before - 2 + 1; h.Len() != want {
		t.Errorf("history depth = %d, want %d", h.Len(), want)
	}
	if got := h.Label(1); got != "Rolled back to index 2" {
		t.Errorf("index 1 = %q", got)
	}
	// The oldest state is still reachable.
	if got := h.Label(h.Len()); got == "" {
		t.Error("the oldest checkpoint was discarded")
	}
}

func TestRollbackRejectsOutOfRange(t *testing.T) {
	base := time.Now()
	wb := grid.NewWorkbook()
	h := New(base, time.Minute)
	for _, k := range []int{-1, 0, 1, 99} {
		if err := h.Rollback(wb, k, base); err == nil {
			t.Errorf("Rollback(%d) should have failed", k)
		}
	}
}

func TestRollbackAcrossSheets(t *testing.T) {
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	wb := grid.NewWorkbook()
	h := New(base, time.Minute)

	before := journal.CaptureWorkbook(wb)
	if _, err := wb.AddSheet("Sheet2"); err != nil {
		t.Fatal(err)
	}
	after := journal.CaptureWorkbook(wb)
	h.Push(journal.Delta{
		Kind: KindStructural, Label: "Sheet added", At: base.Add(time.Minute),
		BeforeWorkbook: &before, AfterWorkbook: &after,
	})
	if wb.Len() != 2 {
		t.Fatalf("workbook should have two sheets, got %d", wb.Len())
	}
	// Rolling back removes the sheet again.
	if err := h.Rollback(wb, 1, base.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if wb.Len() != 1 {
		t.Errorf("rollback should have removed Sheet2, Len = %d", wb.Len())
	}
	if _, ok := wb.SheetByName("Sheet2"); ok {
		t.Error("Sheet2 should be gone")
	}
}

// TestConcurrentUse mirrors the real program: a timer goroutine and the user
// both touch the history. Under -race this is the safety check.
func TestConcurrentUse(t *testing.T) {
	base := time.Now()
	h := New(base, time.Minute)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			h.Touch()
			h.Push(journal.Delta{Kind: KindAuto, Label: "Auto-Checkpoint", At: base})
		}
	}()
	for i := 0; i < 200; i++ {
		h.Push(journal.Delta{Kind: KindManual, Label: "Manual checkpoint", At: base})
		_ = h.Len()
		_ = h.Label(1)
		_ = h.NextIn()
		_ = h.Uncommitted()
	}
	<-done
}

func TestUndoRedoStack(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	u := NewUndoStack(100)

	sh.Set(0, 0, cell.Cell{Source: "0", Value: cell.Number(cell.FromInt64(0))})
	now := time.Now()
	for i := 1; i <= 3; i++ {
		u.Push(edit(wb, sh, ref, itoa(i), now))
	}
	if got := sh.Get(0, 0).Value.Display(); got != "3" {
		t.Fatalf("A1 = %s, want 3", got)
	}
	if !u.CanUndo() || u.CanRedo() {
		t.Fatal("undo/redo availability is wrong")
	}
	if _, ok := u.Undo(wb); !ok {
		t.Fatal("undo failed")
	}
	if got := sh.Get(0, 0).Value.Display(); got != "2" {
		t.Errorf("after undo, A1 = %s, want 2", got)
	}
	if _, ok := u.Undo(wb); !ok {
		t.Fatal("second undo failed")
	}
	if got := sh.Get(0, 0).Value.Display(); got != "1" {
		t.Errorf("after two undos, A1 = %s, want 1", got)
	}
	if _, ok := u.Redo(wb); !ok {
		t.Fatal("redo failed")
	}
	if got := sh.Get(0, 0).Value.Display(); got != "2" {
		t.Errorf("after redo, A1 = %s, want 2", got)
	}
	// A new edit after undoing discards the redone branch, or redo would lie.
	u.Push(edit(wb, sh, ref, "99", now))
	if u.CanRedo() {
		t.Error("a new edit must discard the redo branch")
	}
	if got := sh.Get(0, 0).Value.Display(); got != "99" {
		t.Errorf("A1 = %s, want 99", got)
	}
}

func TestUndoIsBounded(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	u := NewUndoStack(3)
	now := time.Now()
	for i := 1; i <= 10; i++ {
		u.Push(edit(wb, sh, ref, itoa(i), now))
	}
	if u.Len() != 3 {
		t.Errorf("undo depth = %d, want 3", u.Len())
	}
}

func TestEmptyDeltaIsIgnored(t *testing.T) {
	u := NewUndoStack(10)
	u.Push(journal.Delta{})
	if u.CanUndo() {
		t.Error("a delta that changes nothing must not be undoable")
	}
}
