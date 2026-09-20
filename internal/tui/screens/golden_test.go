package screens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/tui/layout"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
)

// goldenNow is the fixed instant every golden frame is rendered at. It is in
// UTC so the stored files do not depend on the machine's time zone.
var goldenNow = time.Date(2026, 3, 4, 22, 14, 2, 0, time.UTC)

// cellFor builds a cell the way the edit path would.
func cellFor(typed string) cell.Cell {
	return cell.Cell{Source: typed, Value: cell.Infer(typed)}
}

// A golden case is one complete screen: a terminal size and a state to put the
// model in. Regenerate the files with `go test ./internal/tui/screens -update`.
type goldenCase struct {
	name  string
	w, h  int
	setup func(m *Model)
}

func goldenCases() []goldenCase {
	fill := func(m *Model) {
		// A cursor part way down a populated column, so the viewport has
		// scrolled and the top row is not row 1.
		for r := uint32(0); r < 80; r++ {
			m.Sheet().Set(r, 0, cellFor("row "+itoa(int(r)+1)))
		}
		m.Cur = grid.Ref{Row: 60, Col: 3}
		m.Anchor = m.Cur
		m.ensureVisible()
	}
	return []goldenCase{
		// The approved look, at several terminal sizes.
		{name: "main_120x32", w: 120, h: 32},
		{name: "main_80x24", w: 80, h: 24},
		{name: "main_100x30", w: 100, h: 30},
		{name: "main_160x50", w: 160, h: 50},
		{name: "main_200x60", w: 200, h: 60},
		// The narrowest supported terminal.
		{name: "main_60x16", w: 60, h: 16},
		// Below the minimum: the warning panel.
		{name: "too_small_40x10", w: 40, h: 10},

		// Overlays.
		{name: "menu_file_120x32", w: 120, h: 32, setup: func(m *Model) {
			press(m, "alt+f")
		}},
		{name: "menu_edit_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.MenuOpen, m.MenuWhich, m.MenuIdx = true, 1, 0
		}},
		{name: "rollback_120x32", w: 120, h: 32, setup: func(m *Model) {
			h := m.History.(*checkpoint.History)
			// Pushed oldest first, as a real session would, so index 1 is the
			// most recent checkpoint and the oldest sits at the bottom.
			for _, e := range []struct {
				kind  checkpoint.Kind
				label string
				ago   time.Duration
			}{
				{checkpoint.KindInitial, "Initial file load", 27 * time.Minute},
				{checkpoint.KindInitial, "Sheet initialized", 24 * time.Minute},
				{checkpoint.KindAuto, "Auto-Checkpoint", 21 * time.Minute},
				{checkpoint.KindAuto, "Auto-Checkpoint", 18 * time.Minute},
				{checkpoint.KindAuto, "Auto-Checkpoint", 15 * time.Minute},
				{checkpoint.KindRowEdited, "Row 4 edited", 12 * time.Minute},
				{checkpoint.KindBulkPaste, "Bulk Paste (+12 cells)", 9 * time.Minute},
				{checkpoint.KindAuto, "Auto-Checkpoint", 6 * time.Minute},
				{checkpoint.KindAuto, "Auto-Checkpoint", 3 * time.Minute},
			} {
				h.Push(journal.Delta{Kind: e.kind, Label: e.label, At: goldenNow.Add(-e.ago)})
			}
			press(m, "alt+r")
		}},
		{name: "rollback_jump_120x32", w: 120, h: 32, setup: func(m *Model) {
			h := m.History.(*checkpoint.History)
			base := goldenNow
			for i := 0; i < 12; i++ {
				h.Push(journal.Delta{Kind: checkpoint.KindAuto, Label: "Auto-Checkpoint",
					At: base.Add(time.Duration(-i) * time.Minute)})
			}
			press(m, "alt+r")
			press(m, "j")
			press(m, "4")
		}},
		{name: "confirm_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Cur = grid.Ref{Row: 2, Col: 0}
			press(m, "alt+d")
		}},
		{name: "prompt_save_as_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.openTextPrompt(PromptSaveAs, "Save as", "budget.cell")
		}},

		// Editing and selection.
		{name: "editing_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Cur = grid.Ref{Row: 1, Col: 0}
			m.beginEdit("=SUM(A1:B1)+")
		}},
		{name: "selection_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Anchor = grid.Ref{Row: 0, Col: 0}
			m.Cur = grid.Ref{Row: 1, Col: 1}
			m.HasSel = true
		}},
		{name: "resize_row_header_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Cur = grid.Ref{Row: 2, Col: 0}
			m.Focus = FocusRowHeader
		}},
		{name: "resize_col_header_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Cur = grid.Ref{Row: 0, Col: 1}
			m.Focus = FocusColHeader
		}},

		// A scrolled viewport and a second sheet.
		{name: "scrolled_120x32", w: 120, h: 32, setup: fill},
		{name: "two_sheets_120x32", w: 120, h: 32, setup: func(m *Model) {
			if _, err := m.Wb.AddSheet("Q3 Data"); err != nil {
				panic(err)
			}
			m.Wb.SetActive(1)
			m.Sheet().Set(0, 0, cellFor("on the second sheet"))
		}},
		{name: "tall_row_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Sheet().SetRowHeight(6, 4)
			m.Sheet().SetColWidth(1, 30)
			m.Cur = grid.Ref{Row: 5, Col: 0}
		}},
		// Tall rows with text in them, at every height where the placement
		// rule changes. The earlier tall-row case was empty, which is exactly
		// how a bug that repeated a tall cell's text went unnoticed.
		{name: "tall_rows_with_text_120x32", w: 120, h: 32, setup: func(m *Model) {
			for i, h := range []int{1, 2, 3, 4, 5} {
				m.Sheet().SetRowHeight(uint32(i), h)
				m.Sheet().Set(uint32(i), 0, cellFor("height "+itoa(h)))
				m.Sheet().Set(uint32(i), 1, cellFor("row "+itoa(i+1)))
			}
			m.Eng.RebuildGraph()
			m.Eng.RecalcWorkbook(nil)
			m.Cur = grid.Ref{Row: 9, Col: 0}
		}},
		{name: "errors_120x32", w: 120, h: 32, setup: func(m *Model) {
			m.Sheet().Set(0, 3, cellFor("=1/0"))
			m.Sheet().Set(1, 3, cellFor("=NOPE()"))
			m.Sheet().Set(2, 3, cellFor("=A3+1"))
			m.Eng.RebuildGraph()
			m.Eng.RecalcWorkbook(nil)
		}},
	}
}

// TestGoldenFrames locks every screen to a stored rendering.
//
// The universal invariants are checked for every case as well, because a golden
// file regenerated carelessly would happily record a broken frame: every line
// must be exactly the terminal width, and there must be exactly as many lines
// as the terminal is tall.
func TestGoldenFrames(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			m := newModel(t)
			m.Width, m.Height = c.w, c.h
			m.ensureVisible()
			if c.setup != nil {
				c.setup(m)
			}
			got := m.View()

			lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
			if len(lines) != c.h {
				t.Errorf("rendered %d lines, want %d", len(lines), c.h)
			}
			for i, ln := range lines {
				if w := render.StringWidth(ln); w != c.w {
					t.Errorf("line %d is %d columns wide, want %d: %q", i+1, w, c.w, ln)
				}
			}
			if !layout.TooSmall(c.w, c.h) {
				if first, last := lines[0], lines[len(lines)-1]; !strings.HasPrefix(first, "┌") || !strings.HasPrefix(last, "└") {
					t.Errorf("the frame does not close: first=%q last=%q", first, last)
				}
			}

			path := filepath.Join("testdata", c.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file (run with -update): %v", err)
			}
			if got != string(want) {
				// The length must be checked explicitly. The line-by-line
				// diff below cannot see a difference that consists only of
				// trailing newlines, because an out-of-range line and a
				// trailing empty line both compare as "".
				if len(got) != len(want) {
					t.Errorf("the frame differs in length: got %d bytes, want %d",
						len(got), len(want))
				}
				if a, b := strings.Count(got, "\n"), strings.Count(string(want), "\n"); a != b {
					t.Errorf("the frame has %d lines, want %d", a+1, b+1)
				}
				gl := strings.Split(got, "\n")
				wl := strings.Split(string(want), "\n")
				shown := 0
				for i := 0; i < len(gl) || i < len(wl); i++ {
					var a, b string
					if i < len(gl) {
						a = gl[i]
					}
					if i < len(wl) {
						b = wl[i]
					}
					if a != b && shown < 6 {
						shown++
						t.Errorf("line %d differs:\n got %q\nwant %q", i+1, a, b)
					}
				}
			}
		})
	}
}
