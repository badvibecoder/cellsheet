package screens

import (
	"strings"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// ---------------------------------------------------------------------------
// File menu: Save, Save As and Open
// ---------------------------------------------------------------------------

// menuDown walks the open menu down n items, skipping separators.
func menuDown(m *Model, n int) {
	for i := 0; i < n; i++ {
		press(m, "down")
	}
}

// TestFileMenuSaveAsOpensThePrompt: choosing Save As from the menu used to set a
// flag with no path, so the application had nothing to save to and the item
// silently did nothing.
func TestFileMenuSaveAsOpensThePrompt(t *testing.T) {
	m := newModel(t)
	press(m, "alt+f")
	// File: New, Open…, ─, Save, Save As…
	menuDown(m, 3)
	if m.MenuIdx != 4 {
		t.Fatalf("menu cursor is on item %d, want Save As…", m.MenuIdx)
	}
	press(m, "enter")
	if m.Mode != ModePrompt || m.Prompt != PromptSaveAs {
		t.Fatalf("Save As… should open the path prompt, mode=%v prompt=%v", m.Mode, m.Prompt)
	}
	if m.PathBuf != m.Path {
		t.Errorf("the prompt should start from the current path %q, got %q", m.Path, m.PathBuf)
	}
}

// TestFileMenuSaveOnAnUntitledWorkbookAsksForAName.
func TestFileMenuSaveOnAnUntitledWorkbookAsksForAName(t *testing.T) {
	m := newModel(t)
	m.Path = ""
	press(m, "alt+f")
	menuDown(m, 2) // Save
	if m.MenuIdx != 3 {
		t.Fatalf("menu cursor is on item %d, want Save", m.MenuIdx)
	}
	press(m, "enter")
	if m.Mode != ModePrompt || m.Prompt != PromptSaveAs {
		t.Fatalf("Save on an untitled workbook should ask for a name, mode=%v prompt=%v", m.Mode, m.Prompt)
	}
}

// TestFileMenuSaveOnANamedWorkbookRequestsASave.
func TestFileMenuSaveOnANamedWorkbookRequestsASave(t *testing.T) {
	m := newModel(t)
	m.Path = "budget.cell"
	press(m, "alt+f")
	menuDown(m, 2) // Save
	press(m, "enter")
	if !m.SaveRequested {
		t.Error("Save on a named workbook should request a save")
	}
	if m.Mode == ModePrompt {
		t.Error("Save on a named workbook should not ask for a path")
	}
}

// TestFileMenuOpenOpensThePrompt: the item raised OpenRequested with no path,
// which the application ignores.
func TestFileMenuOpenOpensThePrompt(t *testing.T) {
	m := newModel(t)
	press(m, "alt+f")
	menuDown(m, 1) // Open…
	press(m, "enter")
	if m.Mode != ModePrompt || m.Prompt != PromptOpen {
		t.Fatalf("Open… should open the path prompt, mode=%v prompt=%v", m.Mode, m.Prompt)
	}
}

// TestCtrlSWhileEditingOnUntitledAsksForAName: Ctrl+S in the editor must route
// through the same code as Ctrl+S on the grid.
func TestCtrlSWhileEditingOnUntitledAsksForAName(t *testing.T) {
	m := newModel(t)
	m.Path = ""
	m.Cur = grid.Ref{Row: 8, Col: 0}
	press(m, "5")
	press(m, "ctrl+s")
	if m.Mode != ModePrompt || m.Prompt != PromptSaveAs {
		t.Fatalf("Ctrl+S on an untitled workbook should ask for a name, mode=%v prompt=%v", m.Mode, m.Prompt)
	}
	if got := m.Sheet().Get(8, 0).Value.Display(); got != "5" {
		t.Errorf("the half-typed value should still have been committed, A9 = %q", got)
	}
}

// ---------------------------------------------------------------------------
// The sheet-rename prompt
// ---------------------------------------------------------------------------

// TestSheetRenamePromptUsesTheTypedName: the handler read the buffer back after
// clearing it, so every sheet was renamed to the empty string.
func TestSheetRenamePromptUsesTheTypedName(t *testing.T) {
	m := newModel(t)
	m.openTextPrompt(PromptSheetName, "Rename sheet", m.Sheet().Name)
	press(m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	press(m, "B", "u", "d", "g", "e", "t")
	press(m, "enter")
	if got := m.Sheet().Name; got != "Budget" {
		t.Errorf("sheet name = %q, want Budget", got)
	}
}

// ---------------------------------------------------------------------------
// The menu bar renders in place
// ---------------------------------------------------------------------------

// TestOpenMenuDoesNotSmearTheBar: opening a menu redrew the bar's names at
// offsets of its own, so "View" landed on "Rollback (Alt+R)" and the old text
// stuck out after it ("Viewllback (Alt+R)").
func TestOpenMenuDoesNotSmearTheBar(t *testing.T) {
	for _, which := range []struct {
		key   string
		title string
	}{
		{"alt+f", "File"},
		{"alt+e", "Edit"},
		{"alt+r", "Rollback"},
	} {
		m := newModel(t)
		press(m, which.key)
		bar := firstLineOf(m.View())
		for _, want := range []string{"File (Alt+F)", "Edit", "Rollback", "View"} {
			if !strings.Contains(bar, want) {
				t.Errorf("after %s the bar lost %q: %q", which.key, want, bar)
			}
		}
		for _, bad := range []string{"Viewllback", "Editllback", "FileAlt"} {
			if strings.Contains(bar, bad) {
				t.Errorf("after %s the bar is smeared: %q contains %q", which.key, bar, bad)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Copying a formula translates its relative references
// ---------------------------------------------------------------------------

// TestPasteTranslatesARangeToTheNewColumn is the reported case: =sum(b1:b5)
// copied to column C must calculate column C, not column B.
func TestPasteTranslatesARangeToTheNewColumn(t *testing.T) {
	m := newModel(t)
	for r := uint32(0); r < 5; r++ {
		m.Eng.Set(t.Context(), m.Sheet(), grid.Ref{Row: r, Col: 1}, "1")
		m.Eng.Set(t.Context(), m.Sheet(), grid.Ref{Row: r, Col: 2}, "10")
	}
	// B6 holds =SUM(B1:B5); copy it to C6.
	m.Cur = grid.Ref{Row: 5, Col: 1}
	m.beginEdit("=SUM(B1:B5)")
	press(m, "enter")

	m.Cur = grid.Ref{Row: 5, Col: 1}
	press(m, "ctrl+c")
	m.Cur = grid.Ref{Row: 5, Col: 2}
	press(m, "ctrl+v")

	cl := m.Sheet().Get(5, 2)
	if cl.Source != "=SUM(C1:C5)" {
		t.Fatalf("C6 source = %q, want =SUM(C1:C5)", cl.Source)
	}
	if got := cl.Value.Display(); got != "50" {
		t.Errorf("C6 = %q, want 50 (the sum of column C)", got)
	}
}

// TestPasteTranslatesWhenCopyingDown: the same rule on the other axis.
func TestPasteTranslatesWhenCopyingDown(t *testing.T) {
	m := newModel(t)
	// C1 holds =SUM(A1:B1).
	m.Cur = grid.Ref{Row: 0, Col: 2}
	press(m, "ctrl+c")
	m.Cur = grid.Ref{Row: 1, Col: 2}
	press(m, "ctrl+v")
	cl := m.Sheet().Get(1, 2)
	if cl.Source != "=SUM(A2:B2)" {
		t.Fatalf("C2 source = %q, want =SUM(A2:B2)", cl.Source)
	}
	if got := cl.Value.Display(); got != "$14,520.50" {
		t.Errorf("C2 = %q, want $14,520.50", got)
	}
}

// TestPasteKeepsAbsoluteReferences: a '$' pins that axis.
func TestPasteKeepsAbsoluteReferences(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	m.beginEdit("=$A$1+A1")
	press(m, "enter")
	m.Cur = grid.Ref{Row: 8, Col: 0}
	press(m, "ctrl+c")
	m.Cur = grid.Ref{Row: 8, Col: 1}
	press(m, "ctrl+v")
	if got := m.Sheet().Get(8, 1).Source; got != "=$A$1+B1" {
		t.Errorf("source = %q, want =$A$1+B1", got)
	}
}

// TestTheSameBufferCanBePastedTwice: translating a formula must not mutate the
// buffer, or a second paste would be shifted twice.
func TestTheSameBufferCanBePastedTwice(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 2} // C1 holds =SUM(A1:B1)
	press(m, "ctrl+c")
	m.Cur = grid.Ref{Row: 0, Col: 3}
	press(m, "ctrl+v")
	m.Cur = grid.Ref{Row: 0, Col: 4}
	press(m, "ctrl+v")
	if got := m.Sheet().Get(0, 3).Source; got != "=SUM(B1:C1)" {
		t.Errorf("D1 = %q, want =SUM(B1:C1)", got)
	}
	if got := m.Sheet().Get(0, 4).Source; got != "=SUM(C1:D1)" {
		t.Errorf("E1 = %q, want =SUM(C1:D1)", got)
	}
}

// TestCutDoesNotTranslateReferences: moving a formula keeps its references, as
// in every spreadsheet.
func TestCutDoesNotTranslateReferences(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 2} // C1 holds =SUM(A1:B1)
	press(m, "ctrl+x")
	m.Cur = grid.Ref{Row: 0, Col: 3}
	press(m, "ctrl+v")
	if got := m.Sheet().Get(0, 3).Source; got != "=SUM(A1:B1)" {
		t.Errorf("a cut should not rewrite the formula, D1 = %q", got)
	}
}

// TestPastedTextIsTakenLiterally: formulas arriving as text from another
// application are not rewritten, because there is no source cell to offset from.
func TestPastedTextIsTakenLiterally(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 8, Col: 0}
	m.pasteText("=SUM(A1:B1)")
	if got := m.Sheet().Get(8, 0).Source; got != "=SUM(A1:B1)" {
		t.Errorf("pasted text source = %q, want it unchanged", got)
	}
}

// ---------------------------------------------------------------------------
// The Edit and Rollback menus actually do what they say
// ---------------------------------------------------------------------------

// TestEditMenuCopyCutPasteWork: the three clipboard items were listed and did
// nothing, the same defect the File menu's Save had.
func TestEditMenuCopyCutPasteWork(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 0} // A1 = $50,000.00
	press(m, "alt+e")                // Edit, cursor on Copy
	press(m, "enter")
	if m.Clip.Empty() {
		t.Fatal("Edit > Copy copied nothing")
	}
	m.Cur = grid.Ref{Row: 8, Col: 0}
	// Edit > Paste is the third item.
	press(m, "alt+e")
	menuDown(m, 2)
	if m.MenuIdx != 2 {
		t.Fatalf("menu cursor is on item %d, want Paste", m.MenuIdx)
	}
	press(m, "enter")
	if got := m.Sheet().Get(8, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("A9 = %q, want the pasted $50,000.00", got)
	}
}

// TestEditMenuCutUsesTheCutBuffer.
func TestEditMenuCutUsesTheCutBuffer(t *testing.T) {
	m := newModel(t)
	m.Cur = grid.Ref{Row: 0, Col: 0}
	press(m, "alt+e")
	menuDown(m, 1) // Cut
	press(m, "enter")
	if !m.Clip.IsCut() {
		t.Error("Edit > Cut should mark the buffer as a cut")
	}
}

// TestSelectAllIsReachableByKeyAndMenu: the Edit menu advertised Ctrl+A, the
// function existed, but nothing was bound to the key.
func TestSelectAllIsReachableByKeyAndMenu(t *testing.T) {
	m := newModel(t)
	press(m, "ctrl+a")
	r0, c0, r1, c1, ok := m.Selection()
	if !ok || r0 != 0 || c0 != 0 || r1 != m.maxRow() || c1 != m.maxCol() {
		t.Errorf("Ctrl+A selected %d,%d..%d,%d ok=%v, want the whole sheet", r0, c0, r1, c1, ok)
	}

	m = newModel(t)
	press(m, "alt+e")
	menuDown(m, 10) // Select all is the last item
	if m.MenuIdx != 13 {
		t.Fatalf("menu cursor is on item %d, want Select all", m.MenuIdx)
	}
	press(m, "enter")
	if _, _, _, _, ok := m.Selection(); !ok {
		t.Error("Edit > Select all selected nothing")
	}
}

// TestRollbackMenuTakesACheckpoint: the item claimed checkpoints "arrive with
// the file format", but Ctrl+B already took one. Alt+R opens the rollback panel
// directly, so the drop-down is addressed the way the golden tests address it.
func TestRollbackMenuTakesACheckpoint(t *testing.T) {
	m := newModel(t)
	got := ""
	m.OnEdit = func(kind journal.Kind, label string) { got = label }
	m.MenuOpen, m.MenuWhich, m.MenuIdx = true, 2, 0
	menuDown(m, 1) // Create checkpoint now
	if m.MenuIdx != 1 {
		t.Fatalf("menu cursor is on item %d, want Create checkpoint now", m.MenuIdx)
	}
	press(m, "enter")
	if got != "Manual checkpoint" {
		t.Errorf("the Rollback menu did not take a checkpoint, OnEdit got %q", got)
	}
}

func TestCtrlBTakesACheckpoint(t *testing.T) {
	m := newModel(t)
	got := ""
	m.OnEdit = func(kind journal.Kind, label string) { got = label }
	press(m, "ctrl+b")
	if got != "Manual checkpoint" {
		t.Errorf("Ctrl+B OnEdit got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Editing one cell collapses the selection
// ---------------------------------------------------------------------------

func TestEditingCollapsesTheSelection(t *testing.T) {
	m := newModel(t)
	m.Anchor = grid.Ref{Row: 0, Col: 0}
	m.Cur = grid.Ref{Row: 1, Col: 1}
	m.HasSel = true
	press(m, "f2")
	if _, _, _, _, ok := m.Selection(); ok {
		t.Error("editing a cell should collapse the block selection")
	}
	if m.EditRef != (grid.Ref{Row: 1, Col: 1}) {
		t.Errorf("editing the wrong cell: %v", m.EditRef)
	}
}
