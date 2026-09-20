package screens

import (
	"strings"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

// ---------------------------------------------------------------------------
// Enter opens the editor on a cell that has content
// ---------------------------------------------------------------------------

// TestEnterEditsAPopulatedCell is the fix for the first report: pressing Enter
// on a cell that already holds something must let it be amended, not silently
// start replacing it.
func TestEnterEditsAPopulatedCell(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 0} // A1 holds $50,000.00

	press(m, "enter")
	if m.Mode != ModeEdit {
		t.Fatalf("Enter on a populated cell should open the editor, mode = %v", m.Mode)
	}
	if m.EditBuf != "$50,000.00" {
		t.Errorf("the editor was seeded with %q, want the existing content", m.EditBuf)
	}
	// Typing appends rather than replacing, which is the point.
	press(m, "0")
	if m.EditBuf != "$50,000.000" {
		t.Errorf("edit buffer = %q, want the existing content plus the new character", m.EditBuf)
	}
	press(m, "esc")

	// From an existing cell, amend and commit.
	m.Cur = grid.Ref{Row: 2, Col: 0} // A3 holds "Operations"
	press(m, "enter")
	press(m, "!")
	press(m, "enter")
	if got := m.Sheet().Get(2, 0).Value.Display(); got != "Operations!" {
		t.Errorf("A3 = %q, want Operations!", got)
	}
}

// TestEnterMovesDownOnAnEmptyCell keeps the old behaviour where it still makes
// sense: there is nothing to amend in an empty cell.
func TestEnterMovesDownOnAnEmptyCell(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}
	press(m, "enter")
	if m.Mode == ModeEdit {
		t.Error("Enter on an empty cell should not open the editor")
	}
	if m.Cur.Row != 11 {
		t.Errorf("cursor row = %d, want 11", m.Cur.Row)
	}
}

// TestEnterAfterCommittingStillMovesDown: the edit path is unchanged, so typing
// a value and pressing Enter still advances.
func TestEnterAfterCommittingStillMovesDown(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}
	press(m, "7", "7")
	press(m, "enter")
	if got := m.Sheet().Get(10, 0).Value.Display(); got != "77" {
		t.Errorf("A11 = %q, want 77", got)
	}
	if m.Cur.Row != 11 {
		t.Errorf("after committing, the cursor should be on row 12, got %d", m.Cur.Row+1)
	}
}

// ---------------------------------------------------------------------------
// A missing closing bracket is added
// ---------------------------------------------------------------------------

// TestMissingClosingBracketIsAdded is the fix for the second report. "=SUM(A1:A4"
// has exactly one sensible completion.
func TestMissingClosingBracketIsAdded(t *testing.T) {
	cases := []struct {
		typed string
		want  string
	}{
		{"=SUM(A1:A4", "=SUM(A1:A4)"},
		{"=SUM(A1:B1, A2", "=SUM(A1:B1, A2)"},
		{"=((1+2", "=((1+2))"},
		{"=SUM(A1:A4)", "=SUM(A1:A4)"},
		{"=1+2", "=1+2"},
		{"=SUM(A1:A4))", "=SUM(A1:A4))"}, // too many: left for the user to fix
	}
	for _, c := range cases {
		m := newModel(t)
		m.Cur = grid.Ref{Row: 10, Col: 0}
		m.beginEdit(c.typed)
		press(m, "enter")
		if got := m.Sheet().Get(10, 0).Source; got != c.want {
			t.Errorf("typing %q stored %q, want %q", c.typed, got, c.want)
		}
	}
}

// TestAutoClosedFormulaCalculates: the repair is only useful if the result is a
// number, not an error.
func TestAutoClosedFormulaCalculates(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}
	m.beginEdit("=SUM(A1:B1")
	press(m, "enter")
	if got := m.Sheet().Get(10, 0).Value.Display(); got != "$51,250.00" {
		t.Errorf("A11 = %q, want $51,250.00", got)
	}
}

// TestAutoCloseIgnoresQuotedBrackets: a bracket inside a string literal must not
// be counted, or the count would be wrong and a valid formula would be damaged.
func TestAutoCloseIgnoresQuotedBrackets(t *testing.T) {
	cases := []struct{ in, want string }{
		{`=CONCAT("(", A1`, `=CONCAT("(", A1)`},
		{`=CONCAT(")", A1`, `=CONCAT(")", A1)`},
		{`=CONCAT("((", A1`, `=CONCAT("((", A1)`},
		{`='Q3 (final)'!A1`, `='Q3 (final)'!A1`},
		{`=CONCAT("a""(b", A1`, `=CONCAT("a""(b", A1)`},
	}
	for _, c := range cases {
		m := newModel(t)
		m.Cur = grid.Ref{Row: 10, Col: 0}
		m.beginEdit(c.in)
		press(m, "enter")
		if got := m.Sheet().Get(10, 0).Source; got != c.want {
			t.Errorf("typing %q stored %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// -= is a formula
// ---------------------------------------------------------------------------

// TestLeadingMinusEqualsIsAFormula is the fix for the third report.
// "-=SUM(D4:D5)" must calculate the negative of the sum, not sit there as text.
func TestLeadingMinusEqualsIsAFormula(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}

	m.beginEdit("-=SUM(A1:B1)")
	press(m, "enter")
	cl := m.Sheet().Get(10, 0)
	if !cl.IsFormula() {
		t.Fatalf("the cell is not a formula: kind=%v source=%q", cl.Value.Kind, cl.Source)
	}
	if got := cl.Value.Display(); got != "-$51,250.00" {
		t.Errorf("A11 = %q, want -$51,250.00", got)
	}
}

// TestLeadingMinusEqualsMatchesItsLongForm: both spellings must agree, because
// they are the same intent.
func TestLeadingMinusEqualsMatchesItsLongForm(t *testing.T) {
	m := newModel(t)
	for i, src := range []string{"-=SUM(A1:B1)", "=-SUM(A1:B1)", "=-sum(a1:b1)"} {
		row := uint32(10 + i)
		m.Cur = grid.Ref{Row: row, Col: 0}
		m.Mode = ModeReady
		m.beginEdit(src)
		press(m, "enter")
		if got := m.Sheet().Get(row, 0).Value.Display(); got != "-$51,250.00" {
			t.Errorf("%q gave %q, want -$51,250.00", src, got)
		}
	}
}

// TestLeadingMinusEqualsOnPlainNumbers: the sign must survive for plain numbers
// too, which is what the report asked for in general terms.
func TestLeadingMinusEqualsOnPlainNumbers(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}
	m.beginEdit("-=100")
	press(m, "enter")
	if got := m.Sheet().Get(10, 0).Value.Display(); got != "-100" {
		t.Errorf("A11 = %q, want -100", got)
	}
}

// TestCellIsFormulaAcceptsBothForms pins the rule in the lowest layer.
func TestCellIsFormulaAcceptsBothForms(t *testing.T) {
	for _, s := range []string{"=1", "-=1", "-=SUM(A1)", "=-SUM(A1)"} {
		if !cell.IsFormula(s) {
			t.Errorf("cell.IsFormula(%q) = false, want true", s)
		}
	}
	// IsFormula is a prefix test, not a validity test: "=  " and "=x" are
	// formulas that the parser will reject. Only a missing marker is not a
	// formula.
	for _, s := range []string{"", "1", "-1", "SUM(A1)", "-", "-x"} {
		if cell.IsFormula(s) {
			t.Errorf("cell.IsFormula(%q) = true, want false", s)
		}
	}
}

// ---------------------------------------------------------------------------
// The column header is reachable
// ---------------------------------------------------------------------------

// TestColumnHeaderIsReachable is the fix for the fourth report: there was no way
// to get the cursor onto a column name, so columns could not be widened.
func TestColumnHeaderIsReachable(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 1}
	press(m, "up")
	if m.Focus != FocusColHeader {
		t.Fatalf("Up at row 1 should focus the column header, focus = %v", m.Focus)
	}
	// And now the resize gesture works on the column.
	before := m.Sheet().ColWidth(1)
	press(m, "+", "+")
	if got := m.Sheet().ColWidth(1); got != before+2 {
		t.Errorf("column B width = %d, want %d", got, before+2)
	}
	press(m, "-")
	if got := m.Sheet().ColWidth(1); got != before+1 {
		t.Errorf("column B width = %d, want %d", got, before+1)
	}
}

// TestColumnHeaderNavigation: once on the header, moving between columns must
// stay on the header, so several columns can be widened in a row.
func TestColumnHeaderNavigation(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 1}
	press(m, "up")
	press(m, "right")
	if m.Focus != FocusColHeader || m.Cur.Col != 2 {
		t.Errorf("right on the header gave focus=%v col=%d, want the header and column 2", m.Focus, m.Cur.Col)
	}
	press(m, "left", "left")
	if m.Focus != FocusColHeader || m.Cur.Col != 0 {
		t.Errorf("left on the header gave focus=%v col=%d", m.Focus, m.Cur.Col)
	}
	// Left at the first column must stay put rather than wrapping.
	press(m, "left")
	if m.Cur.Col != 0 || m.Focus != FocusColHeader {
		t.Errorf("left at column A should stay there, got focus=%v col=%d", m.Focus, m.Cur.Col)
	}
	// Down returns to the grid.
	press(m, "down")
	if m.Focus != FocusCell {
		t.Errorf("down should return to the grid, focus = %v", m.Focus)
	}
}

// TestRowHeaderNavigation: the row header was reachable, and must stay so.
func TestRowHeaderNavigation(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 3, Col: 0}
	press(m, "left")
	if m.Focus != FocusRowHeader {
		t.Fatalf("Left at column A should focus the row header, focus = %v", m.Focus)
	}
	before := m.Sheet().RowHeight(3)
	press(m, "+")
	if got := m.Sheet().RowHeight(3); got != before+1 {
		t.Errorf("row 4 height = %d, want %d", got, before+1)
	}
	// Right returns to the grid.
	press(m, "right")
	if m.Focus != FocusCell {
		t.Errorf("right should return to the grid, focus = %v", m.Focus)
	}
	// Esc also returns.
	press(m, "left")
	press(m, "esc")
	if m.Focus != FocusCell {
		t.Errorf("Esc should return to the grid, focus = %v", m.Focus)
	}
}

// TestArrowKeysNeverLeaveTheGridStuck: whatever the focus, an arrow key must
// produce a sensible position and never a panic.
func TestArrowKeysNeverLeaveTheGridStuck(t *testing.T) {
	m := newModel(t)
	for _, start := range []grid.Ref{{Row: 0, Col: 0}, {Row: 5, Col: 5}, {Row: 99, Col: 25}} {
		for _, focus := range []Focus{FocusCell, FocusRowHeader, FocusColHeader} {
			for _, key := range []string{"up", "down", "left", "right"} {
				m.Cur = start
				m.Focus = focus
				press(m, key)
				if m.Cur.Row >= m.Sheet().Rows() || m.Cur.Col >= m.Sheet().Cols() {
					t.Fatalf("%s from %v at focus %v gave an out-of-range cursor %v",
						key, start, focus, m.Cur)
				}
				if m.Focus < FocusCell || m.Focus > FocusColHeader {
					t.Fatalf("focus became invalid: %v", m.Focus)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// The frame is exactly the height of the terminal
// ---------------------------------------------------------------------------

// TestFrameHasNoTrailingNewline is the fix for the fifth report. A trailing
// newline advances the cursor past the last row, the terminal scrolls by one,
// and the menu bar disappears off the top — which is exactly what was seen.
func TestFrameHasNoTrailingNewline(t *testing.T) {
	for _, size := range [][2]int{{120, 32}, {80, 24}, {60, 16}, {200, 60}} {
		m := newModel(t)
		m.Width, m.Height = size[0], size[1]
		m.ensureVisible()
		view := m.View()

		lines := strings.Count(view, "\n") + 1
		if lines != size[1] {
			t.Errorf("at %dx%d the frame has %d lines, want %d",
				size[0], size[1], lines, size[1])
		}
		if strings.HasSuffix(view, "\n") {
			t.Errorf("at %dx%d the frame ends with a newline, which scrolls the terminal",
				size[0], size[1])
		}
		if !strings.HasPrefix(view, "┌") {
			t.Errorf("at %dx%d the frame does not start with the menu bar: %q",
				size[0], size[1], firstLineOf(view))
		}
	}
}

// TestTooSmallFrameAlsoHasNoTrailingNewline covers the other rendering path.
func TestTooSmallFrameAlsoHasNoTrailingNewline(t *testing.T) {
	m := newModel(t)
	m.Width, m.Height = 40, 10
	view := m.View()
	if strings.HasSuffix(view, "\n") {
		t.Error("the too-small panel ends with a newline")
	}
	if got := strings.Count(view, "\n") + 1; got != 10 {
		t.Errorf("the too-small panel has %d lines, want 10", got)
	}
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// TestGlobalKeysWorkWhileEditing: a half-typed value must not make Ctrl+S or
// Ctrl+Q stop responding. This was found while driving the real program.
func TestGlobalKeysWorkWhileEditing(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 10, Col: 0}
	press(m, "5", "5")
	if m.Mode != ModeEdit {
		t.Fatal("precondition: should be editing")
	}
	press(m, "ctrl+s")
	if m.Mode != ModeReady {
		t.Error("Ctrl+S should commit the edit")
	}
	if got := m.Sheet().Get(10, 0).Value.Display(); got != "55" {
		t.Errorf("the edit was lost: A11 = %q", got)
	}
	if !m.SaveRequested {
		t.Error("Ctrl+S while editing should still request a save")
	}

	m.Cur = grid.Ref{Row: 12, Col: 0}
	press(m, "9")
	press(m, "ctrl+q")
	if !m.Quit {
		t.Error("Ctrl+Q while editing should still quit")
	}
	if got := m.Sheet().Get(12, 0).Value.Display(); got != "9" {
		t.Errorf("Ctrl+Q should commit before quitting: A13 = %q", got)
	}
}

// TestReportedScenario replays exactly what the user described, end to end, so
// that the whole flow is covered by one test rather than five.
func TestReportedScenario(t *testing.T) {
	m := newModel(t)

	// Two values, in A1 and A2.
	m.Cur = grid.Ref{Row: 0, Col: 0}
	m.beginEdit("500")
	press(m, "enter")
	m.beginEdit("600")
	press(m, "enter")

	// D1: the "-=" form, which used to be stored as text.
	m.Cur = grid.Ref{Row: 0, Col: 3}
	m.beginEdit("-=SUM(A1:A2)")
	press(m, "enter")
	if got := m.Sheet().Get(0, 3).Value.Display(); got != "-1100" {
		t.Fatalf("D1 = %q, want -1100", got)
	}

	// D2: a missing closing bracket.
	m.beginEdit("=SUM(A1:A2")
	press(m, "enter")
	if got := m.Sheet().Get(1, 3).Value.Display(); got != "1100" {
		t.Fatalf("D2 = %q, want 1100", got)
	}

	// Resize column D by going up past row 1.
	m.Cur = grid.Ref{Row: 0, Col: 3}
	press(m, "up")
	if m.Focus != FocusColHeader {
		t.Fatalf("the column header is not reachable, focus = %v", m.Focus)
	}
	width := m.Sheet().ColWidth(3)
	press(m, "+", "+")
	if got := m.Sheet().ColWidth(3); got != width+2 {
		t.Errorf("column D width = %d, want %d", got, width+2)
	}

	// And Enter on a populated cell opens it for amendment.
	m.Focus = FocusCell
	m.Cur = grid.Ref{Row: 0, Col: 0}
	press(m, "enter")
	if m.Mode != ModeEdit || m.EditBuf != "500" {
		t.Errorf("Enter on A1 gave mode=%v buffer=%q, want an edit of 500", m.Mode, m.EditBuf)
	}
}

// TestHeadersAllowResizingConsecutiveRowsAndColumns: while a header is focused,
// moving along it must stay on the header. Otherwise you cannot size two rows
// in a row without stepping back out to the grid each time.
func TestHeadersAllowResizingConsecutiveRowsAndColumns(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 1, Col: 0}
	press(m, "left")
	if m.Focus != FocusRowHeader {
		t.Fatal("expected the row header")
	}
	press(m, "down")
	if m.Focus != FocusRowHeader || m.Cur.Row != 2 {
		t.Fatalf("down on the row header gave focus=%v row=%d", m.Focus, m.Cur.Row)
	}
	press(m, "+")
	if got := m.Sheet().RowHeight(2); got != 2 {
		t.Errorf("row 3 height = %d, want 2", got)
	}
	press(m, "down")
	if m.Focus != FocusRowHeader || m.Cur.Row != 3 {
		t.Fatalf("second down gave focus=%v row=%d", m.Focus, m.Cur.Row)
	}
	press(m, "+", "+")
	if got := m.Sheet().RowHeight(3); got != 3 {
		t.Errorf("row 4 height = %d, want 3", got)
	}
	// Right leaves the row header.
	press(m, "right")
	if m.Focus != FocusCell {
		t.Errorf("right should leave the row header, focus = %v", m.Focus)
	}

	// The column header behaves the same way.
	m.Cur = grid.Ref{Row: 0, Col: 1}
	press(m, "up")
	press(m, "right")
	if m.Focus != FocusColHeader || m.Cur.Col != 2 {
		t.Fatalf("right on the column header gave focus=%v col=%d", m.Focus, m.Cur.Col)
	}
	press(m, "+")
	if got := m.Sheet().ColWidth(2); got != 19 {
		t.Errorf("column C width = %d, want 19", got)
	}
	// Down leaves the column header.
	press(m, "down")
	if m.Focus != FocusCell {
		t.Errorf("down should leave the column header, focus = %v", m.Focus)
	}
}
