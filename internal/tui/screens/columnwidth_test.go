package screens

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/tui/layout"
)

// columnInteriorWidths reads the width of every visible column straight out of
// the rendered header line.
//
// It measures what the user sees rather than what the geometry believes, which
// is the whole point: the original defect was that the geometry and the model
// disagreed, and a test that trusted either one would have missed it.
func columnInteriorWidths(t *testing.T, m *Model) []int {
	t.Helper()
	runes := []rune(line(t, m, 4)) // the column-header row
	var bars []int
	for i, r := range runes {
		if r == '│' {
			bars = append(bars, i)
		}
	}
	if len(bars) < 3 {
		t.Fatalf("the header row has no column separators: %q", string(runes))
	}
	// bars[0] is the left frame, bars[1] the gutter/column-A border, and each
	// following bar is one column's right border.
	out := make([]int, 0, len(bars)-2)
	for i := 2; i < len(bars); i++ {
		out = append(out, bars[i]-bars[i-1]-1)
	}
	return out
}

// TestDefaultColumnsMatchTheApprovedLook guards against the fix changing the
// appearance of an untouched sheet: five columns, the last one stretched.
func TestDefaultColumnsMatchTheApprovedLook(t *testing.T) {
	m := newModel(t)
	widths := columnInteriorWidths(t, m)
	if len(widths) != 5 {
		t.Fatalf("default view has %d columns, want 5: %v", len(widths), widths)
	}
	for i := 0; i < 4; i++ {
		if widths[i] != layout.DefaultColW {
			t.Errorf("column %d draws %d wide, want the default %d", i, widths[i], layout.DefaultColW)
		}
	}
	if widths[4] <= layout.DefaultColW {
		t.Errorf("the last unsized column should stretch to fill, but draws %d", widths[4])
	}
}

// TestColumnResizeIsVisible is the reported defect: widening a column must
// change what is on screen.
func TestColumnResizeIsVisible(t *testing.T) {
	m := newModel(t)

	m.Sheet().SetColWidth(0, 32)
	widths := columnInteriorWidths(t, m)
	if widths[0] != 32 {
		t.Fatalf("column A draws %d wide after being set to 32: %v", widths[0], widths)
	}

	m.Sheet().SetColWidth(0, 8)
	widths = columnInteriorWidths(t, m)
	if widths[0] != 8 {
		t.Fatalf("column A draws %d wide after being narrowed to 8: %v", widths[0], widths)
	}
}

// TestColumnResizeMovesTheFollowingColumns: widening one column must push the
// ones after it to the right, not silently reflow them.
func TestColumnResizeMovesTheFollowingColumns(t *testing.T) {
	m := newModel(t)
	beforeB := columnStartX(t, m, 'B')
	m.Sheet().SetColWidth(0, 30)
	afterB := columnStartX(t, m, 'B')
	if afterB <= beforeB {
		t.Errorf("column B starts at x=%d after widening A from x=%d; it should have moved right",
			afterB, beforeB)
	}
	// And the header for A is still drawn, centred in its new width.
	if got := columnInteriorWidths(t, m)[0]; got != 30 {
		t.Errorf("column A draws %d wide, want 30", got)
	}
}

// TestRightmostColumnResizeIsVisible covers the trap that made the first fix
// incomplete: the last column was stretched to fill, and the stretch gave back
// exactly what a resize added, so nothing appeared to happen.
func TestRightmostColumnResizeIsVisible(t *testing.T) {
	m := newModel(t)
	g := m.geometry()
	n := len(g.ColWidth)
	last := g.FirstCol + n - 1

	// Drive it through the real key path, so the scrolling that keeps the
	// resized column on screen is exercised too.
	m.Cur = grid.Ref{Row: 0, Col: uint32(last)}
	m.Focus = FocusColHeader

	for _, want := range []int{24, 30, 12, 18} {
		for m.Sheet().ColWidth(uint32(last)) < want {
			press(m, "+")
		}
		for m.Sheet().ColWidth(uint32(last)) > want {
			press(m, "-")
		}
		g2 := m.geometry()
		idx := int(m.Cur.Col) - g2.FirstCol
		if idx < 0 || idx >= len(g2.ColWidth) {
			t.Fatalf("column %d scrolled out of view after being set to %d (%d visible from %d)",
				m.Cur.Col, want, len(g2.ColWidth), g2.FirstCol)
		}
		widths := columnInteriorWidths(t, m)
		if idx >= len(widths) {
			t.Fatalf("column %d is missing from the frame: %v", m.Cur.Col, widths)
		}
		if widths[idx] != want {
			t.Errorf("the resized column draws %d wide after being set to %d: %v",
				widths[idx], want, widths)
		}
	}
}

// TestResizingAColumnKeepsItOnScreen: widening a column past the right edge must
// scroll rather than making it vanish.
func TestResizingAColumnKeepsItOnScreen(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 4} // the rightmost visible column
	m.Focus = FocusColHeader
	for i := 0; i < 30; i++ {
		press(m, "+")
	}
	if got := m.Sheet().ColWidth(4); got != 48 {
		t.Fatalf("column E width = %d, want 48", got)
	}
	g := m.geometry()
	idx := int(m.Cur.Col) - g.FirstCol
	if idx < 0 || idx >= len(g.ColWidth) {
		t.Fatalf("column E scrolled out of view: showing %d columns from %d", len(g.ColWidth), g.FirstCol)
	}
	widths := columnInteriorWidths(t, m)
	if idx >= len(widths) || widths[idx] != 48 {
		t.Errorf("column E draws %v, want 48 at slot %d", widths, idx)
	}
}

// TestColumnResizeSurvivesScrolling: the geometry indexes columns from the
// scroll offset, so a width set on one column must not be applied to another
// when the view scrolls.
func TestColumnResizeSurvivesScrolling(t *testing.T) {
	m := newModel(t)
	m.Sheet().SetColWidth(1, 40) // column B, which is not the stretched one

	// Scroll right so the visible window starts past column A.
	m.Cur = grid.Ref{Row: 0, Col: 4}
	m.ensureVisible()
	g := m.geometry()
	if g.FirstCol == 0 {
		t.Fatalf("expected the view to have scrolled, FirstCol = %d", g.FirstCol)
	}
	widths := columnInteriorWidths(t, m)
	for i := range g.ColWidth {
		col := g.FirstCol + i
		if col == 1 {
			continue // scrolled past
		}
		if i < len(widths) && widths[i] != g.ColWidth[i] {
			t.Errorf("column %s draws %d wide but the geometry says %d",
				grid.ColName(col), widths[i], g.ColWidth[i])
		}
	}
	// Every drawn column must be the width the sheet says it is.
	for i := range widths {
		col := g.FirstCol + i
		if col == g.FirstCol+len(g.ColWidth)-1 {
			continue // the stretched or clipped final column
		}
		if want := m.Sheet().ColWidth(uint32(col)); widths[i] != want {
			t.Errorf("column %s draws %d wide, sheet says %d", grid.ColName(col), widths[i], want)
		}
	}
}

// TestManyWideColumnsStillShowSomething: a column wider than the terminal must
// not leave an empty grid.
func TestManyWideColumnsStillShowSomething(t *testing.T) {
	m := newModel(t)
	for c := 0; c < 6; c++ {
		m.Sheet().SetColWidth(uint32(c), grid.MaxColWidth)
	}
	view := m.View()
	if !strings.Contains(view, "┌") {
		t.Fatal("the frame disappeared")
	}
	widths := columnInteriorWidths(t, m)
	if len(widths) == 0 {
		t.Fatal("no columns were drawn")
	}
	// The first column is far wider than the screen, so it is clipped.
	if widths[0] != m.geometry().ColWidth[0] {
		t.Errorf("the clipped column draws %d but the geometry says %d", widths[0], m.geometry().ColWidth[0])
	}
}

// columnStartX is the x of the left border of the column whose header reads
// name, found by scanning the header row for its label.
func columnStartX(t *testing.T, m *Model, name rune) int {
	t.Helper()
	runes := []rune(line(t, m, 4))
	for i, r := range runes {
		if r == name {
			// Walk back to the border before it.
			for j := i; j >= 0; j-- {
				if runes[j] == '│' {
					return j
				}
			}
		}
	}
	t.Fatalf("column %q is not on screen: %q", string(name), string(runes))
	return -1
}

// TestHeldResizeKeyIsNotSwallowed is the bug the terminal session exposed: a
// run of "+" can arrive as one key event, and matching on the whole key string
// meant "++++" matched nothing and started an edit instead of resizing.
func TestHeldResizeKeyIsNotSwallowed(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 2, Col: 1}
	m.Focus = FocusRowHeader
	before := m.Sheet().RowHeight(2)

	// One event carrying four characters, as a held key can produce.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("++++")})
	if m.Mode == ModeEdit {
		t.Fatal("a run of resize keys must not start an edit")
	}
	if got := m.Sheet().RowHeight(2); got != before+4 {
		t.Errorf("row height = %d, want %d", got, before+4)
	}

	// The same on a column header.
	m.Cur = grid.Ref{Row: 0, Col: 2}
	m.Focus = FocusColHeader
	beforeCol := m.Sheet().ColWidth(2)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("---")})
	if got := m.Sheet().ColWidth(2); got != beforeCol-3 {
		t.Errorf("column width = %d, want %d", got, beforeCol-3)
	}

	// A mixed run is not a gesture and falls through to normal handling.
	m.Focus = FocusRowHeader
	m.Mode = ModeReady
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("+-")})
	if m.Mode != ModeEdit {
		t.Error("a mixed run should be treated as text")
	}
	if m.Focus != FocusCell {
		t.Error("typing on a header should return the cursor to the grid")
	}
}

// TestSinglePressStillResizes is the ordinary case, which must not regress.
func TestSinglePressStillResizes(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 1}
	m.Focus = FocusColHeader
	before := m.Sheet().ColWidth(1)
	press(m, "+")
	press(m, "=") // the unshifted key on many layouts
	press(m, "-")
	if got := m.Sheet().ColWidth(1); got != before+1 {
		t.Errorf("column width = %d, want %d", got, before+1)
	}
}
