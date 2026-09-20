package screens

import (
	"strings"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/tui/layout"
)

// gutterName extracts the row name from a rendered line, tolerating the
// brackets the active row is drawn with.
func gutterName(l string) string {
	r := []rune(l)
	if len(r) < 14 {
		return ""
	}
	s := strings.TrimSpace(string(r[1:14]))
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	return s
}

// rowBlock returns the rendered content lines of one row.
func rowBlock(t *testing.T, m *Model, row, height int) []string {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	want := "r" + itoa(row+1)
	for i, l := range lines {
		if gutterName(l) != want {
			continue
		}
		end := i + height
		if end > len(lines) {
			end = len(lines)
		}
		return lines[i:end]
	}
	t.Fatalf("row %d is not on screen", row+1)
	return nil
}

// TestTallRowDoesNotRepeatItsText is the reported defect: a row four lines tall
// drew its contents on all four lines.
func TestTallRowDoesNotRepeatItsText(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4, 5, 8} {
		m := newModel(t)
		m.Sheet().SetRowHeight(6, height)
		m.Sheet().Set(6, 0, cellFor("hello"))
		m.Eng.RebuildGraph()
		m.Eng.RecalcWorkbook(nil)

		block := rowBlock(t, m, 6, height)
		if len(block) != height {
			t.Fatalf("a row of height %d rendered %d lines", height, len(block))
		}
		seen := 0
		for _, l := range block {
			if strings.Contains(l, "hello") {
				seen++
			}
		}
		if seen != 1 {
			t.Errorf("a row of height %d shows its text %d times, want exactly once", height, seen)
		}
	}
}

// TestTallRowTextPlacement pins the requested vertical alignment: the middle
// line, or the lower of the two middle lines when the height is even.
func TestTallRowTextPlacement(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
		m := newModel(t)
		m.Sheet().SetRowHeight(6, height)
		m.Sheet().Set(6, 0, cellFor("marker"))
		m.Eng.RebuildGraph()
		m.Eng.RecalcWorkbook(nil)

		block := rowBlock(t, m, 6, height)
		want := layout.TextLine(height)
		for i, l := range block {
			has := strings.Contains(l, "marker")
			if i == want && !has {
				t.Errorf("height %d: text is missing from line %d of %d", height, want+1, height)
			}
			if i != want && has {
				t.Errorf("height %d: text appears on line %d of %d, want line %d",
					height, i+1, height, want+1)
			}
		}
	}
}

// TestTallRowTextPlacementIsStableWhenScrolled: the placement is a property of
// the row, not of where it happens to be on screen.
func TestTallRowTextPlacementIsScrolled(t *testing.T) {
	m := newModel(t)
	for r := uint32(0); r < 20; r++ {
		m.Sheet().SetRowHeight(r, 3)
		m.Sheet().Set(r, 0, cellFor("row "+itoa(int(r)+1)))
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(nil)
	m.Cur = grid.Ref{Row: 12, Col: 0}
	m.ensureVisible()

	plan := m.rowPlan()
	for _, p := range plan {
		if p.Lines < 2 {
			continue
		}
		block := rowBlock(t, m, p.Row, p.Lines)
		want := layout.TextLine(p.Lines)
		for i, l := range block {
			has := strings.Contains(l, "row "+itoa(p.Row+1))
			if i == want && !has {
				t.Errorf("row %d: text missing from line %d", p.Row+1, want+1)
			}
			if i != want && has {
				t.Errorf("row %d: text on line %d, want %d", p.Row+1, i+1, want+1)
			}
		}
	}
}

// TestTallActiveCellIsOutlinedDownItsHeight: the cursor's heavy bars must run
// the full height of a tall row, even though the contents sit on one line.
func TestTallActiveCellIsOutlinedDownItsHeight(t *testing.T) {
	m := newModel(t)
	m.Sheet().SetRowHeight(2, 4)
	m.Sheet().Set(2, 0, cellFor("tall"))
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(nil)
	m.Cur = grid.Ref{Row: 2, Col: 0}
	m.Focus = FocusCell

	block := rowBlock(t, m, 2, 4)
	for i, l := range block {
		if !strings.Contains(l, "║") {
			t.Errorf("line %d of the active tall row has no cursor bar: %q", i+1, l)
		}
	}
	if !strings.Contains(block[layout.TextLine(4)], "tall") {
		t.Errorf("the active cell's text is not on the placement line: %v", block)
	}
}

// TestTallRowWithNoContentStaysBlank: the other lines of a tall row must be
// empty, not filled with repeated padding.
func TestTallRowWithNoContentStaysBlank(t *testing.T) {
	m := newModel(t)
	// Row 7 is empty in the sample data and near enough the top to be on
	// screen; rows 1 to 4 hold the mock's content.
	m.Sheet().SetRowHeight(6, 3)
	block := rowBlock(t, m, 6, 3)
	for i, l := range block {
		// Everything right of the gutter border is the cell area; the gutter
		// itself legitimately carries the row name and the height marker.
		r := []rune(l)
		if len(r) < 15 {
			continue
		}
		cells := strings.ReplaceAll(string(r[15:len(r)-1]), "│", "")
		if strings.TrimSpace(cells) != "" {
			t.Errorf("line %d of an empty tall row has content in the cells: %q", i+1, l)
		}
	}
}
