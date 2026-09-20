package screens

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/clipboard"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"
)

var update = flag.Bool("update", false, "rewrite the golden render files")

func newModel(t *testing.T) *Model {
	m, _ := newModelWithHistory(t)
	return m
}

func newModelWithHistory(t *testing.T) (*Model, *checkpoint.History) {
	t.Helper()
	wb := grid.NewWorkbook()
	eng := calc.New(wb)
	t.Cleanup(eng.Close)

	// The sample data from the approved mock, built through the engine so the
	// formula cell holds a genuine computed value.
	set := func(ref, typed string) {
		r, _ := grid.ParseRef(ref)
		eng.Set(context.Background(), wb.Active(), r, typed)
	}
	set("A1", "$50,000.00")
	set("B1", "$1,250.00")
	set("C1", "=SUM(A1:B1)")
	set("A2", "$14,200.50")
	set("B2", "$320.00")
	set("C2", "$14,520.50")
	set("A3", "Operations")
	set("B3", "Equipment")
	set("C3", "Subtotal")
	set("D3", "Audited?")
	set("A4", "450")
	set("B4", "12.75")
	set("C4", "5737.50")
	set("D4", "YES")
	wb.Active().SetRowHeight(4, 2) // r5 is two lines tall

	hist := checkpoint.New(time.Date(2026, 3, 4, 22, 11, 0, 0, time.UTC), 3*time.Minute)
	m := New(wb, eng, theme.Plain(), render.ColorNone, hist)
	m.Width, m.Height = 120, 32
	m.Path = "Q3_budget.cell"
	m.Dirty = true
	m.Cur = grid.Ref{Row: 0, Col: 2}
	m.Clip = &clipboard.Buffer{}
	m.Undo = checkpoint.NewUndoStack(1000)
	m.Rec = journal.NewRecorder()
	m.OnEdit = func(kind journal.Kind, label string) {
		if d := m.Rec.Build(wb, kind, label, time.Now()); !d.Empty() {
			m.Undo.Push(d)
		}
	}
	return m, hist
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(keyMsg(k))
	}
}

// keyMsg converts the canonical key names used in tests into Bubble Tea events.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "shift+up":
		return tea.KeyMsg{Type: tea.KeyShiftUp}
	case "shift+down":
		return tea.KeyMsg{Type: tea.KeyShiftDown}
	case "shift+left":
		return tea.KeyMsg{Type: tea.KeyShiftLeft}
	case "shift+right":
		return tea.KeyMsg{Type: tea.KeyShiftRight}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+home":
		return tea.KeyMsg{Type: tea.KeyCtrlHome}
	case "ctrl+end":
		return tea.KeyMsg{Type: tea.KeyCtrlEnd}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+pgup":
		return tea.KeyMsg{Type: tea.KeyCtrlPgUp}
	case "ctrl+pgdown":
		return tea.KeyMsg{Type: tea.KeyCtrlPgDown}
	case "ctrl+t":
		return tea.KeyMsg{Type: tea.KeyCtrlT}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+x":
		return tea.KeyMsg{Type: tea.KeyCtrlX}
	case "ctrl+v":
		return tea.KeyMsg{Type: tea.KeyCtrlV}
	case "ctrl+z":
		return tea.KeyMsg{Type: tea.KeyCtrlZ}
	case "ctrl+y":
		return tea.KeyMsg{Type: tea.KeyCtrlY}
	case "ctrl+left":
		return tea.KeyMsg{Type: tea.KeyCtrlLeft}
	case "ctrl+right":
		return tea.KeyMsg{Type: tea.KeyCtrlRight}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+q":
		return tea.KeyMsg{Type: tea.KeyCtrlQ}
	case "ctrl+n":
		return tea.KeyMsg{Type: tea.KeyCtrlN}
	case "alt+d":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}, Alt: true}
	case "alt+c":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}
	case "alt+f":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}, Alt: true}
	case "alt+r":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}, Alt: true}
	case "y":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	case "n":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	case "j":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	case "+":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}}
	case "-":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'-'}}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func line(t *testing.T, m *Model, i int) string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(m.View(), "\n"), "\n")
	if i < 0 || i >= len(lines) {
		t.Fatalf("line %d out of range (%d lines)", i, len(lines))
	}
	return lines[i]
}

// The full golden-frame suite, across terminal sizes and screens, is in
// golden_test.go. TestFrameShape below asserts the structural promises that a
// careless golden regeneration could otherwise lock in.

// TestFrameShape checks the structural promises of the layout directly, so that
// a golden update cannot silently break them.
func TestFrameShape(t *testing.T) {
	m := newModel(t)

	if got := line(t, m, 0); !strings.HasPrefix(got, "┌─ File (Alt+F) ── Edit ── Rollback (Alt+R) ── View") {
		t.Errorf("menu bar is wrong: %q", got)
	}
	if got := line(t, m, 0); !strings.Contains(got, "cellsheet: Q3_budget.cell [●]") {
		t.Errorf("title is wrong: %q", got)
	}
	if got := line(t, m, 2); !strings.Contains(got, "│  C1  │ fx: =SUM(A1:B1)") {
		t.Errorf("formula bar is wrong: %q", got)
	}
	// Decision D1: the active cell is marked in three places at once.
	if got := line(t, m, 4); !strings.Contains(got, "[C]") {
		t.Errorf("the active column header should be bracketed: %q", got)
	}
	if got := line(t, m, 6); !strings.Contains(got, "[r1]") {
		t.Errorf("the active row header should be bracketed: %q", got)
	}
	if got := line(t, m, 6); !strings.Contains(got, "║       $51,250.00 ║") {
		t.Errorf("the active cell should use heavy bars: %q", got)
	}
	// The last line is the bottom border: the frame must close.
	last := line(t, m, 31)
	if !strings.HasPrefix(last, "└") || !strings.HasSuffix(last, "┘") {
		t.Errorf("the frame does not close: %q", last)
	}
	if w := render.StringWidth(last); w != 120 {
		t.Errorf("line 32 is %d columns wide, want 120", w)
	}
	// Every line must be exactly the terminal width.
	for i := 0; i < 32; i++ {
		if w := render.StringWidth(line(t, m, i)); w != 120 {
			t.Errorf("line %d is %d columns wide, want 120", i+1, w)
		}
	}
}

func TestArrowKeysMoveTheCursor(t *testing.T) {
	m := newModel(t)
	if m.Cur != (grid.Ref{Row: 0, Col: 2}) {
		t.Fatalf("cursor starts at %v", m.Cur)
	}
	press(m, "down", "down")
	if m.Cur != (grid.Ref{Row: 2, Col: 2}) {
		t.Errorf("after Down,Down the cursor is at %v, want C3", m.Cur)
	}
	press(m, "right")
	if m.Cur != (grid.Ref{Row: 2, Col: 3}) {
		t.Errorf("after Right the cursor is at %v, want D3", m.Cur)
	}
	press(m, "up", "left", "left", "left")
	if m.Cur != (grid.Ref{Row: 1, Col: 0}) {
		t.Errorf("cursor is at %v, want A2", m.Cur)
	}
	// The edges clamp rather than wrap.
	press(m, "left", "left", "up", "up", "up")
	if m.Cur != (grid.Ref{Row: 0, Col: 0}) {
		t.Errorf("the cursor should clamp at A1, got %v", m.Cur)
	}
}

func TestCursorStaysVisibleWhenScrolling(t *testing.T) {
	m := newModel(t)
	for i := 0; i < 60; i++ {
		press(m, "down")
	}
	if int(m.Cur.Row) != 60 {
		t.Fatalf("cursor row = %d, want 60", m.Cur.Row)
	}
	if m.TopRow == 0 {
		t.Error("the viewport should have scrolled")
	}
	rows := m.visibleRows()
	if int(m.Cur.Row) < m.TopRow || int(m.Cur.Row) >= m.TopRow+rows {
		t.Errorf("the cursor is off screen: row %d, viewport %d..%d", m.Cur.Row, m.TopRow, m.TopRow+rows)
	}
}

func TestShiftArrowsSelect(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.Anchor = m.Cur
	press(m, "shift+down", "shift+right")

	r0, c0, r1, c1, ok := m.Selection()
	if !ok {
		t.Fatal("a 2x2 selection should be active")
	}
	if r0 != 0 || c0 != 0 || r1 != 1 || c1 != 1 {
		t.Errorf("selection is %d,%d..%d,%d want A1:B2", r0, c0, r1, c1)
	}
	// The active cell stays put: it is the anchor, as in Excel.
	if m.Cur != (grid.Ref{Row: 1, Col: 1}) {
		t.Errorf("the moving corner should be at B2, got %v", m.Cur)
	}
	// The status bar reports the sum and the size.
	status := line(t, m, 30)
	if !strings.Contains(status, "Sum: $65,770.50") {
		t.Errorf("status should show the selection sum: %q", status)
	}
	if !strings.Contains(status, "2R x 2C") {
		t.Errorf("status should show the selection size: %q", status)
	}
	// A plain arrow collapses the selection.
	press(m, "down")
	if _, _, _, _, ok := m.Selection(); ok {
		t.Error("moving without shift should clear the selection")
	}
}

func TestTypingEditsAndEvaluates(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 4, Col: 0} // A5, empty
	press(m, "1", "2", "3")
	if m.Mode != ModeEdit {
		t.Fatalf("typing should enter edit mode, mode = %v", m.Mode)
	}
	if m.EditBuf != "123" {
		t.Fatalf("edit buffer = %q, want 123", m.EditBuf)
	}
	press(m, "backspace")
	press(m, "enter")
	if got := m.Sheet().Get(4, 0).Value.Display(); got != "12" {
		t.Errorf("A5 = %q, want 12", got)
	}
	// Enter moves down, as in Excel.
	if m.Cur.Row != 5 {
		t.Errorf("after Enter the cursor should be on row 6, got %d", m.Cur.Row+1)
	}
}

func TestFormulaTypedIntoACellRecalculates(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 5, Col: 0}
	press(m, "=", "S", "U", "M", "(", "A", "1", ":", "B", "1", ")")
	press(m, "enter")
	if got := m.Sheet().Get(5, 0).Value.Display(); got != "$51,250.00" {
		t.Errorf("A6 = %q, want $51,250.00", got)
	}
	// And editing a precedent updates it.
	m.Cur = grid.Ref{Row: 0, Col: 1}
	m.Mode = ModeReady
	m.EditBuf = "$2,250.00"
	m.beginEdit("$2,250.00")
	press(m, "enter")
	if got := m.Sheet().Get(5, 0).Value.Display(); got != "$52,250.00" {
		t.Errorf("A6 = %q after editing B1, want $52,250.00", got)
	}
}

func TestEscapeCancelsAnEdit(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 4, Col: 0}
	press(m, "9", "9", "9", "esc")
	if m.Mode != ModeReady {
		t.Error("Esc should leave edit mode")
	}
	if m.Sheet().Has(4, 0) {
		t.Error("Esc should not commit the edit")
	}
}

func TestMenuOpensWithAltF(t *testing.T) {
	m := newModel(t)
	press(m, "alt+f")
	if !m.MenuOpen || m.MenuWhich != 0 {
		t.Fatalf("Alt+F should open the File menu, open=%v which=%d", m.MenuOpen, m.MenuWhich)
	}
	view := m.View()
	if !strings.Contains(view, "Save As…") {
		t.Error("the File menu should list Save As…")
	}
	if !strings.Contains(view, "Ctrl+Q") {
		t.Error("the File menu should show shortcuts")
	}
	press(m, "esc")
	if m.MenuOpen {
		t.Error("Esc should close the menu")
	}
}

func TestRollbackPanelShowsHistory(t *testing.T) {
	m, hist := newModelWithHistory(t)
	hist.Mark(journal.Delta{Kind: checkpoint.KindBulkPaste, Label: "Bulk Paste (+12 cells)", At: time.Now()})
	hist.Mark(journal.Delta{Kind: checkpoint.KindAuto, Label: "Auto-Checkpoint", At: time.Now()})
	press(m, "alt+r")
	if m.Mode != ModeRollback {
		t.Fatalf("Alt+R should open the rollback panel, mode = %v", m.Mode)
	}
	view := m.View()
	if !strings.Contains(view, "0: Current State") {
		t.Error("the panel should show the current state at index 0")
	}
	if !strings.Contains(view, "1: Auto-Checkpoint") {
		t.Error("the panel should show the most recent checkpoint at index 1")
	}
	if !strings.Contains(view, "Max available: 2") {
		t.Error("the panel should state the maximum available rollback index")
	}
	if !strings.Contains(view, "J: Jump to custom rollback index") {
		t.Error("the panel should offer the custom jump")
	}
	// J opens the numeric prompt, which validates against the maximum.
	press(m, "j")
	if m.Mode != ModePrompt {
		t.Fatalf("J should open the custom index prompt, mode = %v", m.Mode)
	}
	press(m, "9", "9")
	press(m, "enter")
	if m.Mode != ModePrompt {
		t.Error("an out-of-range index must be refused, leaving the prompt open")
	}
	press(m, "backspace", "backspace", "2")
	press(m, "enter")
	if m.Mode != ModeReady {
		t.Errorf("a valid index should be accepted, mode = %v", m.Mode)
	}
}

func TestResizeRequiresTheHeader(t *testing.T) {
	m := newModel(t)
	// On a cell, +/- must not resize anything. It must instead start an edit,
	// otherwise "=" could never begin a formula.
	m.Cur = grid.Ref{Row: 2, Col: 1}
	press(m, "+")
	if got := m.Sheet().RowHeight(2); got != 1 {
		t.Errorf("+ on a cell should not resize, row height = %d", got)
	}
	if m.Mode != ModeEdit {
		t.Errorf("+ on a cell should start an edit, mode = %v", m.Mode)
	}
	press(m, "esc")

	// "=" must start a formula, not trip the resize binding.
	m.Cur = grid.Ref{Row: 6, Col: 0}
	press(m, "=", "1", "+", "1")
	press(m, "enter")
	if got := m.Sheet().Get(6, 0).Value.Display(); got != "2" {
		t.Errorf("=1+1 gave %q, want 2", got)
	}
	// Left at column A moves onto the row header; now + resizes that row.
	m.Cur = grid.Ref{Row: 2, Col: 0}
	press(m, "left")
	if m.Focus != FocusRowHeader {
		t.Fatalf("Left at column A should focus the row header, focus = %v", m.Focus)
	}
	press(m, "+", "+")
	if got := m.Sheet().RowHeight(2); got != 3 {
		t.Errorf("row height = %d, want 3", got)
	}
	// The marker appears in the gutter.
	if !strings.Contains(m.View(), "(height: 3)") {
		t.Error("the height marker should be drawn")
	}
	press(m, "-")
	if got := m.Sheet().RowHeight(2); got != 2 {
		t.Errorf("row height = %d, want 2", got)
	}
	// And the bound is respected without an error.
	for i := 0; i < 40; i++ {
		press(m, "-")
	}
	if got := m.Sheet().RowHeight(2); got != 1 {
		t.Errorf("the row should clamp at 1 line, got %d", got)
	}
}

func TestDeleteRowAndColumn(t *testing.T) {
	m := newModel(t)
	// Delete row 3 ("Operations ... Audited?"). Row 4 should move up.
	m.Cur = grid.Ref{Row: 2, Col: 0}
	press(m, "alt+d")
	if m.Mode != ModeConfirm {
		t.Fatalf("a destructive edit should ask first, mode = %v", m.Mode)
	}
	press(m, "y")
	if got := m.Sheet().Get(2, 0).Value.Display(); got != "450" {
		t.Errorf("after deleting row 3, A3 = %q, want 450", got)
	}
	// Delete column A. Column B should move left.
	m.Cur = grid.Ref{Row: 0, Col: 0}
	press(m, "alt+c")
	press(m, "y")
	if got := m.Sheet().Get(0, 0).Value.Display(); got != "$1,250.00" {
		t.Errorf("after deleting column A, A1 = %q, want $1,250.00", got)
	}
}

func TestDeleteIsCancellable(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 2, Col: 0}
	press(m, "alt+d")
	press(m, "n")
	if got := m.Sheet().Get(2, 0).Value.Display(); got != "Operations" {
		t.Errorf("cancelling must change nothing, A3 = %q", got)
	}
}

func TestCtrlSAndCtrlQRaiseRequests(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+s")
	if !m.SaveRequested {
		t.Error("Ctrl+S should request a save")
	}
	press(m, "ctrl+q")
	if !m.Quit {
		t.Error("Ctrl+Q should request a quit")
	}
}

func TestSheetTabsRender(t *testing.T) {
	m := newModel(t)
	if _, err := m.Wb.AddSheet("Sheet2"); err != nil {
		t.Fatal(err)
	}
	view := m.View()
	if !strings.Contains(view, "[ Sheet1* ]") {
		t.Error("the active tab should be marked modified")
	}
	if !strings.Contains(view, "[ Sheet2 ]") {
		t.Error("the second tab should be listed")
	}
	if !strings.Contains(view, "[ + ]") {
		t.Error("the add-sheet affordance should be present")
	}
}

func TestTooSmallTerminal(t *testing.T) {
	m := newModel(t)
	m.Width, m.Height = 40, 10
	view := m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("a tiny terminal should show the warning, got:\n%s", view)
	}
}

func TestCellStylesMatchKinds(t *testing.T) {
	v := cell.Infer("450")
	if !v.AlignRight() {
		t.Error("numbers are right-aligned (D11)")
	}
	if cell.Infer("Operations").AlignRight() {
		t.Error("text is left-aligned (D11)")
	}
	if !cell.Infer("$5").AlignRight() {
		t.Error("currency is right-aligned")
	}
	if !cell.Error(cell.ErrDivZero).AlignCenter() {
		t.Error("errors are centred")
	}
}

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------

func TestCopyAndPastePreservesKindsAndFormulas(t *testing.T) {
	m := newModel(t)
	// Copy A1:B1, a currency pair, and paste at A6.
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.Anchor = grid.Ref{Row: 0, Col: 0}
	m.Cur = grid.Ref{Row: 0, Col: 1}
	m.HasSel = true
	press(m, "ctrl+c")
	if m.Clip.Empty() {
		t.Fatal("nothing was copied")
	}

	m.HasSel = false
	m.Cur = grid.Ref{Row: 5, Col: 0}
	press(m, "ctrl+v")
	if got := m.Sheet().Get(5, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("A6 = %q, want $50,000.00", got)
	}
	if got := m.Sheet().Get(5, 1).Value.Display(); got != "$1,250.00" {
		t.Errorf("B6 = %q, want $1,250.00", got)
	}
	if m.Sheet().Get(5, 0).Value.Kind != cell.KindCurrency {
		t.Error("the currency kind was not preserved")
	}
}

func TestPasteOfAFormulaRecalculates(t *testing.T) {
	m := newModel(t)
	// C1 holds =SUM(A1:B1). Copy it to C2, which should then sum A2:B2.
	m.Cur = grid.Ref{Row: 0, Col: 2}
	press(m, "ctrl+c")
	m.Cur = grid.Ref{Row: 1, Col: 2}
	press(m, "ctrl+v")
	got := m.Sheet().Get(1, 2).Value.Display()
	if got == "#NAME?" || got == "" {
		t.Fatalf("C2 = %q", got)
	}
	if !m.Sheet().Get(1, 2).IsFormula() {
		t.Error("the pasted cell should be a formula")
	}
}

func TestCutClearsTheSource(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 3, Col: 0} // A4 holds 450
	press(m, "ctrl+x")
	if !m.Clip.IsCut() {
		t.Fatal("the buffer should be marked as a cut")
	}
	m.Cur = grid.Ref{Row: 6, Col: 0}
	press(m, "ctrl+v")
	if m.Sheet().Has(3, 0) {
		t.Error("the source cell should have been cleared by the cut")
	}
	if got := m.Sheet().Get(6, 0).Value.Display(); got != "450" {
		t.Errorf("A7 = %q, want 450", got)
	}
}

// TestPasteGrowsTheSheet is the auto-growth requirement: pasting past the last
// row adds 100 more, pasting past the last column adds 26 more.
func TestPasteGrowsTheSheet(t *testing.T) {
	m := newModel(t)
	// Build a 5x3 block of text and paste it at r99, column Y.
	text := "a\tb\tc\nd\te\tf\ng\th\ti\nj\tk\tl\nm\tn\to"
	m.Cur = grid.Ref{Row: 99, Col: 24}
	m.pasteText(text)

	if got := m.Sheet().Rows(); got != 200 {
		t.Errorf("rows = %d, want 200 after pasting past the last row", got)
	}
	if got := m.Sheet().Cols(); got != 52 {
		t.Errorf("columns = %d, want 52 after pasting past the last column", got)
	}
	if got := m.Sheet().Get(99, 24).Value.Str; got != "a" {
		t.Errorf("the pasted block did not land: %q", got)
	}
	if got := m.Sheet().Get(103, 26).Value.Str; got != "o" {
		t.Errorf("the bottom-right of the block = %q, want o", got)
	}
}

func TestPasteFromApplicationParsesValues(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	m.pasteText("$1,250.00\tOperations\n12.75\tYES\n")

	if got := m.Sheet().Get(8, 0).Value.Kind; got != cell.KindCurrency {
		t.Errorf("pasted currency kind = %v", got)
	}
	if got := m.Sheet().Get(8, 0).Value.Display(); got != "$1,250.00" {
		t.Errorf("(8,0) = %q", got)
	}
	if got := m.Sheet().Get(9, 0).Value.Display(); got != "12.75" {
		t.Errorf("(9,0) = %q", got)
	}
	if got := m.Sheet().Get(9, 1).Value.Str; got != "YES" {
		t.Errorf("(9,1) = %q", got)
	}
}

func TestPasteIsOneUndoStep(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	m.pasteText("1\t2\t3\n4\t5\t6")
	depth := m.Undo.Len()
	press(m, "ctrl+z")
	if m.Undo.Len() != depth-1 {
		t.Errorf("undo depth = %d, want %d", m.Undo.Len(), depth-1)
	}
	for r := 8; r <= 9; r++ {
		for c := 0; c <= 2; c++ {
			if m.Sheet().Has(uint32(r), uint32(c)) {
				t.Errorf("cell (%d,%d) should have been cleared by one undo", r, c)
			}
		}
	}
}

func TestPasteMarksTheBlockSelected(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	m.pasteText("1\t2\n3\t4")
	r0, c0, r1, c1, ok := m.Selection()
	if !ok {
		t.Fatal("a pasted block should be selected")
	}
	if r0 != 8 || c0 != 0 || r1 != 9 || c1 != 1 {
		t.Errorf("selection is %d,%d..%d,%d", r0, c0, r1, c1)
	}
}

func TestEmptyClipboardPasteIsHarmless(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	press(m, "ctrl+v")
	if m.Sheet().Has(8, 0) {
		t.Error("pasting an empty clipboard should change nothing")
	}
}

// TestMultiRuneInputIsNotTruncated guards a bug found by driving the real
// program: the terminal coalesces fast typing into one key event, and taking
// only the first rune silently reduced "$50,000.00" to "$".
func TestMultiRuneInputIsNotTruncated(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	// One event carrying a whole string, exactly as a terminal delivers it.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("$50,000.00")})
	if m.EditBuf != "$50,000.00" {
		t.Fatalf("edit buffer = %q, want the whole string", m.EditBuf)
	}
	press(m, "enter")
	if got := m.Sheet().Get(8, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("A9 = %q, want $50,000.00", got)
	}

	// The same must hold for a formula, and for the file prompt.
	m.Cur = grid.Ref{Row: 9, Col: 0}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("=SUM(A1:A2)")})
	press(m, "enter")
	if got := m.Sheet().Get(9, 0).Value.Display(); got != "$64,200.50" {
		t.Errorf("A10 = %q, want $64,200.50", got)
	}
}

// ---------------------------------------------------------------------------
// Sheet lifecycle
// ---------------------------------------------------------------------------

func TestAddSheet(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+t")
	if m.Wb.Len() != 2 {
		t.Fatalf("sheet count = %d, want 2", m.Wb.Len())
	}
	if got := m.Wb.Active().Name; got != "Sheet2" {
		t.Errorf("the new sheet should be active, got %q", got)
	}
	// The new sheet starts empty and is independent of the first.
	if m.Sheet().Has(0, 0) {
		t.Error("a new sheet should be empty")
	}
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.Sheet().Set(0, 0, cell.Cell{Source: "on sheet two", Value: cell.Text("on sheet two")})
	m.Wb.SetActive(0)
	if got := m.Wb.Active().Get(0, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("editing Sheet2 changed Sheet1: A1 = %q", got)
	}
	// And adding is undoable.
	press(m, "ctrl+z")
	if m.Wb.Len() != 1 {
		t.Errorf("after undo, sheet count = %d, want 1", m.Wb.Len())
	}
}

func TestRenameSheet(t *testing.T) {
	m := newModel(t)
	m.renameSheet("Budget")
	if got := m.Wb.Sheets()[0].Name; got != "Budget" {
		t.Fatalf("name = %q", got)
	}
	if !strings.Contains(m.View(), "[ Budget* ]") {
		t.Error("the tab should show the new name")
	}
	// A duplicate name is refused, and the status bar says so.
	if _, err := m.Wb.AddSheet("Second"); err != nil {
		t.Fatal(err)
	}
	m.Wb.SetActive(1)
	m.renameSheet("Budget")
	if got := m.Wb.Sheets()[1].Name; got != "Second" {
		t.Errorf("a duplicate rename should be refused, name = %q", got)
	}
	if m.Status == "" {
		t.Error("the refusal should be reported in the status bar")
	}
}

func TestDeleteSheet(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+t") // Sheet2
	m.Sheet().Set(0, 0, cell.Cell{Source: "x", Value: cell.Text("x")})
	m.deleteSheet()
	if m.Wb.Len() != 1 {
		t.Fatalf("sheet count = %d, want 1", m.Wb.Len())
	}
	// Undo brings it back with its content.
	press(m, "ctrl+z")
	if m.Wb.Len() != 2 {
		t.Fatalf("after undo, sheet count = %d, want 2", m.Wb.Len())
	}
	if _, ok := m.Wb.SheetByName("Sheet2"); !ok {
		t.Error("the deleted sheet was not restored")
	}
	if got := m.Wb.Sheets()[1].Get(0, 0).Value.Display(); got != "x" {
		t.Errorf("the restored sheet lost its content: %q", got)
	}
}

func TestLastSheetCannotBeDeleted(t *testing.T) {
	m := newModel(t)
	m.deleteSheet()
	if m.Wb.Len() != 1 {
		t.Errorf("sheet count = %d, want 1", m.Wb.Len())
	}
	if m.Status == "" {
		t.Error("the refusal should be reported")
	}
}

func TestSwitchingSheetsChangesTheGrid(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+t")
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.Sheet().Set(0, 0, cell.Cell{Source: "second", Value: cell.Text("second")})
	view := m.View()
	if !strings.Contains(view, "second") {
		t.Error("the active sheet's content should be visible")
	}
	m.Wb.SetActive(0)
	view = m.View()
	if strings.Contains(view, "second") {
		t.Error("the first sheet's view should not show the second sheet's content")
	}
	if !strings.Contains(view, "$50,000.00") {
		t.Error("the first sheet's content should be visible again")
	}
}

func TestInsertRowAndColumn(t *testing.T) {
	m := newModel(t)
	// Row 3 holds "Operations"; insert a row above it.
	m.Cur = grid.Ref{Row: 2, Col: 0}
	m.insertRow()
	if got := m.Sheet().Get(2, 0).Value.Display(); got != "" {
		t.Errorf("the inserted row should be empty, A3 = %q", got)
	}
	if got := m.Sheet().Get(3, 0).Value.Display(); got != "Operations" {
		t.Errorf("the old content should have moved down, A4 = %q", got)
	}
	// Column A holds $50,000.00 and $14,200.50; insert a column before it.
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.insertColumn()
	if got := m.Sheet().Get(0, 0).Value.Display(); got != "" {
		t.Errorf("the inserted column should be empty, A1 = %q", got)
	}
	if got := m.Sheet().Get(0, 1).Value.Display(); got != "$50,000.00" {
		t.Errorf("the old content should have moved right, B1 = %q", got)
	}
	if got := m.Sheet().Get(0, 2).Value.Display(); got != "$1,250.00" {
		t.Errorf("B1 should now be at C1, got %q", got)
	}
	// Both are undoable.
	press(m, "ctrl+z")
	if got := m.Sheet().Get(0, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("after undo, A1 = %q", got)
	}
	press(m, "ctrl+z")
	if got := m.Sheet().Get(2, 0).Value.Display(); got != "Operations" {
		t.Errorf("after two undos, A3 = %q", got)
	}
}

func TestSwitchSheets(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+t") // Sheet2
	press(m, "ctrl+t") // Sheet3
	if m.Wb.ActiveIndex() != 2 {
		t.Fatalf("active index = %d, want 2", m.Wb.ActiveIndex())
	}
	press(m, "ctrl+pgup")
	if m.Wb.ActiveIndex() != 1 {
		t.Errorf("Ctrl+PgUp gave index %d, want 1", m.Wb.ActiveIndex())
	}
	press(m, "ctrl+pgup")
	press(m, "ctrl+pgup")
	// It wraps around rather than sticking.
	if m.Wb.ActiveIndex() != 2 {
		t.Errorf("after wrapping, index = %d, want 2", m.Wb.ActiveIndex())
	}
	press(m, "ctrl+pgdown")
	if m.Wb.ActiveIndex() != 0 {
		t.Errorf("Ctrl+PgDn should wrap to 0, got %d", m.Wb.ActiveIndex())
	}
	// Switching resets the cursor to A1 of the new sheet.
	if m.Cur != (grid.Ref{}) {
		t.Errorf("cursor = %v, want A1", m.Cur)
	}
}

func TestSwitchSheetsWithOnlyOne(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+pgdown")
	if m.Wb.ActiveIndex() != 0 {
		t.Errorf("active index = %d, want 0", m.Wb.ActiveIndex())
	}
	if m.Status == "" {
		t.Error("the status bar should explain that there is only one sheet")
	}
}

// TestCursorIsAlwaysVisible is the promise scrolling makes. It is checked after
// a jump, a page, and a resize, because each takes a different path through
// ensureVisible.
func TestCursorIsAlwaysVisible(t *testing.T) {
	check := func(t *testing.T, m *Model, where string) {
		t.Helper()
		plan := m.rowPlan()
		if len(plan) == 0 {
			t.Fatalf("%s: no rows were planned", where)
		}
		row := int(m.Cur.Row)
		if row < plan[0].Row || row > plan[len(plan)-1].Row {
			t.Errorf("%s: cursor is on row %d but the viewport shows rows %d..%d",
				where, row, plan[0].Row, plan[len(plan)-1].Row)
		}
		// Horizontal: the cursor's column must be in the visible range.
		if col := int(m.Cur.Col); col < m.LeftCol || col >= m.LeftCol+m.visibleColsFrom(m.LeftCol) {
			t.Errorf("%s: cursor is on column %d but the viewport starts at %d and shows %d columns",
				where, col, m.LeftCol, m.visibleColsFrom(m.LeftCol))
		}
	}

	m := newModel(t)
	for _, size := range [][2]int{{120, 32}, {80, 24}, {60, 16}, {200, 60}} {
		m.Width, m.Height = size[0], size[1]
		m.ensureVisible()
		for _, jump := range []grid.Ref{
			{Row: 0, Col: 0},
			{Row: 99, Col: 25},
			{Row: 500, Col: 100}, // past the end; setCursor clamps it
			{Row: 9000, Col: 900},
			{Row: 0, Col: 0},
		} {
			m.setCursor(jump, false)
			check(t, m, "a jump at "+itoa(size[0])+"x"+itoa(size[1]))
		}
		m.Cur = grid.Ref{Row: 50, Col: 3}
		m.ensureVisible()
		press(m, "pgdown", "pgdown")
		check(t, m, "after paging down")
		press(m, "pgup")
		check(t, m, "after paging up")
		press(m, "ctrl+end")
		check(t, m, "after Ctrl+End")
		press(m, "ctrl+home")
		check(t, m, "after Ctrl+Home")
	}
}

// TestScrollingWithTallRows checks the plan-based scroll with non-uniform row
// heights, which is where an estimate-based approach goes wrong.
func TestScrollingWithTallRows(t *testing.T) {
	m := newModel(t)
	for r := 0; r < 20; r++ {
		m.Sheet().SetRowHeight(uint32(r), 5)
	}
	for _, row := range []int{0, 3, 10, 25, 60, 99} {
		m.setCursor(grid.Ref{Row: uint32(row), Col: 0}, false)
		plan := m.rowPlan()
		if len(plan) == 0 {
			t.Fatal("no rows planned")
		}
		if row < plan[0].Row || row > plan[len(plan)-1].Row {
			t.Errorf("cursor row %d is outside the viewport %d..%d",
				row, plan[0].Row, plan[len(plan)-1].Row)
		}
	}
}
