// Package screens holds the interactive views. The grid screen is the program.
package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/clipboard"
	"github.com/badvibecoder/cellsheet/internal/formula/parser"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/tui/layout"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"
)

// PromptKind is which single-line prompt is open.
type PromptKind int

const (
	PromptRollback PromptKind = iota
	PromptSaveAs
	PromptOpen
	PromptSheetName
)

// Mode is what the keyboard currently does.
type Mode int

const (
	ModeReady Mode = iota
	ModeEdit
	ModeMenu
	ModeRollback
	ModePrompt
	ModeConfirm
)

// String is the word shown in the status bar.
func (m Mode) String() string {
	switch m {
	case ModeEdit:
		return "EDIT"
	case ModeMenu:
		return "MENU"
	case ModeRollback:
		return "ROLLBACK"
	case ModePrompt:
		return "PROMPT"
	case ModeConfirm:
		return "CONFIRM"
	default:
		return "READY"
	}
}

// Focus is which part of the frame the cursor is on. Resizing a row or column
// requires the cursor to be on that header, per the original specification.
type Focus int

const (
	FocusCell Focus = iota
	FocusRowHeader
	FocusColHeader
)

// Model is the grid screen: the whole interactive program.
type Model struct {
	Wb  *grid.Workbook
	Eng *calc.Engine
	Th  theme.Theme

	Color render.ColorMode

	Width, Height int
	Cur           grid.Ref
	Anchor        grid.Ref
	HasSel        bool
	TopRow        int
	LeftCol       int
	Focus         Focus

	Mode    Mode
	EditBuf string
	EditRef grid.Ref

	Status     string
	StatusWarn bool

	// Menus
	MenuOpen  bool
	MenuIdx   int
	MenuWhich int // 0 = File, 1 = Edit, 2 = Rollback, 3 = View

	// Rollback
	RbIdx   int
	RbTyped string

	// Path prompt (Save As, Open)
	Prompt    PromptKind
	PathBuf   string
	PromptMsg string

	// Confirmation dialog
	ConfirmMsg  string
	ConfirmFunc func(bool)

	// Checkpoints are surfaced here rather than reached for directly, so the
	// screen has no opinion about how history is stored.
	History HistoryView

	// Rec records the before-state of everything a mutation touches, so the
	// application can build a reversible delta (PHASE-1-SPEC.md §13).
	Rec *journal.Recorder

	// Undo is the session's per-edit undo stack (decision D3).
	Undo *checkpoint.UndoStack

	// Clip is the internal copy buffer (decision D6).
	Clip *clipboard.Buffer

	// OnEdit is called after a recorded mutation, so the application can build
	// the delta, push it onto the undo stack and mark the history dirty.
	OnEdit func(kind journal.Kind, label string)

	// OnDirty is called after a change that was not routed through the
	// recorder, such as undo, redo or rollback.
	OnDirty func()

	Dirty bool
	Path  string

	// Quit is set when the user asks to exit.
	Quit bool

	// Save requests are handled by the application, which owns the file.
	SaveRequested bool
	SaveAs        bool

	// Flash is a transient message with an expiry.
	flashUntil time.Time

	// FilesOpen is set when the user asks to open a workbook.
	OpenRequested bool
	NewRequested  bool

	// PendingPath is the path typed into Save As or Open.
	PendingPath string
}

// HistoryView is the slice of checkpoint state the screen needs. It is an
// interface so the grid screen can be built and tested without the storage
// layer, and so the two can evolve independently.
type HistoryView interface {
	// Len is how many rollback steps are available (0 means "current only").
	Len() int
	// Label and When describe one step; index 0 is the current state.
	Label(i int) string
	When(i int) time.Time
	// Max is the highest valid rollback index.
	Max() int
	// Uncommitted reports whether index 0 differs from what is on disk.
	Uncommitted() bool
	// NextIn is the time until the next automatic checkpoint.
	NextIn() time.Duration
	// Rollback moves the workbook to the state at index k.
	Rollback(wb *grid.Workbook, k int, now time.Time) error
}

// New creates the grid screen.
func New(wb *grid.Workbook, eng *calc.Engine, th theme.Theme, color render.ColorMode, hist HistoryView) *Model {
	if hist == nil {
		hist = emptyHistory{}
	}
	return &Model{
		Wb: wb, Eng: eng, Th: th, Color: color,
		Width: 120, Height: 32,
		History: hist,
	}
}

type emptyHistory struct{}

func (emptyHistory) Len() int              { return 0 }
func (emptyHistory) Label(int) string      { return "" }
func (emptyHistory) When(int) time.Time    { return time.Time{} }
func (emptyHistory) Max() int              { return 0 }
func (emptyHistory) Uncommitted() bool     { return false }
func (emptyHistory) NextIn() time.Duration { return 0 }
func (emptyHistory) Rollback(*grid.Workbook, int, time.Time) error {
	return checkpoint.ErrOutOfRange
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Sheet is the active sheet.
func (m *Model) Sheet() *grid.Sheet { return m.Wb.Active() }

// SetStatus shows a message in the status bar.
func (m *Model) SetStatus(warn bool, format string, args ...any) {
	m.Status = fmt.Sprintf(format, args...)
	m.StatusWarn = warn
	m.flashUntil = time.Now().Add(6 * time.Second)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
		m.ensureVisible()
		return m, nil

	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	// Text pasted by the terminal (bracketed paste) is tab-separated data from
	// another application, not a key sequence.
	if msg.Paste {
		m.pasteText(string(msg.Runes))
		return nil
	}

	switch m.Mode {
	case ModeEdit:
		return m.handleEditKey(msg)
	case ModeMenu:
		return m.handleMenuKey(key)
	case ModeRollback:
		return m.handleRollbackKey(key)
	case ModePrompt:
		return m.handlePromptKey(msg)
	case ModeConfirm:
		return m.handleConfirmKey(key)
	}

	if m.MenuOpen {
		return m.handleMenuKey(key)
	}

	// The +/- resize gesture applies only while the cursor is on a header, and
	// it is matched by rune rather than by the key's string form. One key event
	// can carry several runes — a held key, or a coalesced read — and "++++"
	// must resize four times rather than being treated as typed text.
	if m.Focus != FocusCell {
		if n, delta := resizeRun(printableString(msg)); n > 0 {
			for i := 0; i < n; i++ {
				m.resizeFocused(delta)
			}
			return nil
		}
	}

	switch key {
	// ---------------------------------------------------------------- global
	case "ctrl+q":
		m.Quit = true
		return tea.Quit
	case "ctrl+s":
		if m.Path == "" {
			m.openPathPrompt(PromptSaveAs, "Save as")
			return nil
		}
		m.SaveRequested = true
		return nil
	case "ctrl+n":
		m.NewRequested = true
		return nil
	case "ctrl+o":
		m.openPathPrompt(PromptOpen, "Open")
		return nil
	case "ctrl+c":
		m.copySelection(false)
		return nil
	case "ctrl+x":
		m.copySelection(true)
		return nil
	case "ctrl+v":
		m.pasteClipboard()
		return nil
	case "ctrl+z":
		m.undo()
		return nil
	case "ctrl+y":
		m.redo()
		return nil
	case "ctrl+b":
		if m.OnEdit != nil {
			m.OnEdit(journal.KindManual, "Manual checkpoint")
		}
		return nil
	case "ctrl+t":
		m.addSheet()
		return nil
	case "ctrl+pgdown":
		m.switchSheet(1)
		return nil
	case "ctrl+pgup":
		m.switchSheet(-1)
		return nil
	case "alt+f":
		m.MenuOpen, m.MenuWhich, m.MenuIdx = true, 0, 0
		return nil
	case "alt+e":
		m.MenuOpen, m.MenuWhich, m.MenuIdx = true, 1, 0
		return nil
	case "alt+r":
		m.openRollback()
		return nil
	case "alt+v":
		m.MenuOpen, m.MenuWhich, m.MenuIdx = true, 3, 0
		return nil

	// ------------------------------------------------------------ navigation
	case "up":
		m.arrowUp(false)
	case "down":
		m.arrowDown(false)
	case "left":
		m.arrowLeft(false)
	case "right":
		m.arrowRight(false)
	case "shift+up":
		m.move(-1, 0, true)
	case "shift+down":
		m.move(1, 0, true)
	case "shift+left":
		m.move(0, -1, true)
	case "shift+right":
		m.move(0, 1, true)
	case "home":
		m.jumpToRowStart(false)
	case "end":
		m.jumpToRowEnd(false)
	case "ctrl+home":
		m.setCursor(grid.Ref{}, false)
	case "ctrl+end":
		m.jumpToLastUsed()
	case "pgup":
		m.move(-m.visibleRows(), 0, false)
	case "pgdown":
		m.move(m.visibleRows(), 0, false)
	case "ctrl+left":
		m.jumpNonEmpty(0, -1, false)
	case "ctrl+right":
		m.jumpNonEmpty(0, 1, false)
	case "tab":
		m.move(0, 1, false)
	case "shift+tab":
		m.move(0, -1, false)
	case "enter":
		m.enterKey()
	case "shift+enter":
		m.move(-1, 0, false)

	// --------------------------------------------------------------- editing
	case "f2":
		m.beginEdit(m.currentText())
	case "delete", "backspace":
		m.clearSelection()
	case "esc":
		m.HasSel = false
		m.Focus = FocusCell

	// -------------------------------------------------------------- resizing

	// --------------------------------------------------------- structural
	case "alt+d":
		m.askConfirm(fmt.Sprintf("Delete row %d?", m.Cur.Row+1), func(ok bool) {
			if ok {
				m.deleteRow()
			}
		})
		return nil
	case "alt+c":
		m.askConfirm(fmt.Sprintf("Delete column %s?", grid.ColName(int(m.Cur.Col))), func(ok bool) {
			if ok {
				m.deleteColumn()
			}
		})
		return nil

	default:
		// Printable text starts an edit, as in every spreadsheet. The whole
		// text is used, not just its first character. Typing while the cursor
		// is on a header takes it back into the grid rather than being
		// swallowed, because losing a keystroke is worse than a small jump.
		text := printableString(msg)
		if text == "" {
			return nil
		}
		if m.Focus != FocusCell {
			m.Focus = FocusCell
		}
		m.beginEdit(text)
	}
	return nil
}

// resizeRun reports how many resize steps a run of identical keys means, and in
// which direction. A mixed run such as "+-+" is not a resize gesture at all, so
// it is left to be treated as text.
func resizeRun(text string) (steps, delta int) {
	if text == "" {
		return 0, 0
	}
	first := text[0]
	switch first {
	case '+', '=':
		delta = +1
	case '-':
		delta = -1
	default:
		return 0, 0
	}
	for i := 1; i < len(text); i++ {
		if text[i] != first {
			return 0, 0
		}
	}
	return len(text), delta
}

// printableString returns the text carried by a key message, dropping control
// runes. A single event can carry several runes — the terminal coalesces fast
// typing, and a paste arrives as one event — so taking only the first rune
// silently truncates the input to one character.
func printableString(msg tea.KeyMsg) string {
	if msg.Type != tea.KeyRunes {
		return ""
	}
	var b strings.Builder
	for _, r := range msg.Runes {
		if r >= 32 && r != 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (m *Model) handleEditKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.commitEdit()
		m.move(1, 0, false)
		return nil
	// The global keys must keep working while a cell is being edited, or a
	// half-typed value makes Ctrl+S do nothing at all — which is the worst
	// possible moment for saving to stop responding.
	case "ctrl+s":
		m.commitEdit()
		m.SaveRequested = true
		return nil
	case "ctrl+q":
		m.commitEdit()
		m.Quit = true
		return tea.Quit
	case "ctrl+b":
		m.commitEdit()
		if m.OnEdit != nil {
			m.OnEdit(journal.KindManual, "Manual checkpoint")
		}
		return nil
	case "esc":
		m.Mode = ModeReady
		m.EditBuf = ""
		return nil
	case "backspace":
		m.EditBuf = dropLastRune(m.EditBuf)
		return nil
	case "ctrl+u":
		m.EditBuf = ""
		return nil
	case "tab":
		m.commitEdit()
		m.move(0, 1, false)
		return nil
	}
	m.EditBuf += printableString(msg)
	return nil
}

func (m *Model) handleConfirmKey(key string) tea.Cmd {
	switch key {
	case "y", "enter":
		m.Mode = ModeReady
		if m.ConfirmFunc != nil {
			m.ConfirmFunc(true)
		}
	case "n", "esc", "q":
		m.Mode = ModeReady
		if m.ConfirmFunc != nil {
			m.ConfirmFunc(false)
		}
	}
	m.ConfirmFunc = nil
	m.ConfirmMsg = ""
	return nil
}

func (m *Model) askConfirm(msg string, fn func(bool)) {
	m.Mode = ModeConfirm
	m.ConfirmMsg = msg
	m.ConfirmFunc = fn
}

// beginEdit starts editing the active cell, seeded with seed. If seed is
// non-empty the existing content is replaced, which is what happens when you
// simply start typing.
func (m *Model) beginEdit(seed string) {
	m.Mode = ModeEdit
	m.EditRef = m.Cur
	m.EditBuf = seed
}

func (m *Model) commitEdit() {
	if m.Mode != ModeEdit {
		return
	}
	m.Mode = ModeReady
	sh := m.Sheet()
	typed := m.EditBuf
	m.EditBuf = ""

	// Opening the editor and confirming without changing anything must leave
	// the cell exactly as it was. Re-inferring the displayed text would round
	// a currency amount to the two decimals the display happens to show.
	if existing := sh.GetRef(m.EditRef); existing.Source == "" && !existing.Value.IsEmpty() &&
		typed == existing.Value.Display() {
		return
	}

	// A formula that stops in the middle of a call is closed for the user: the
	// only sensible completion of "=SUM(A1:A4" is "=SUM(A1:A4)".
	if cell.IsFormula(typed) {
		if closed := parser.AutoClose(strings.TrimSpace(typed)); closed != typed {
			typed = closed
			m.SetStatus(false, "Closed the bracket for you")
		}
	}

	if m.Rec != nil {
		m.Rec.BeforeCell(sh.ID, m.EditRef, sh.GetRef(m.EditRef))
	}
	m.Eng.Set(context.Background(), sh, m.EditRef, typed)
	after := sh.GetRef(m.EditRef).Value

	m.record(journal.KindCellEdited, "Cell "+grid.RefName(m.EditRef.Row, m.EditRef.Col)+" edited")
	m.Dirty = true
	if err := m.Eng.LastError(); err != nil {
		m.SetStatus(true, "Formula error: %v", err)
		return
	}
	if after.IsError() {
		m.SetStatus(true, "%s", after.Display())
		return
	}
	// On success the cell itself shows the result, so the status bar stays
	// quiet rather than adding noise.
}

// currentText is the active cell's source, or its displayed value.
func (m *Model) currentText() string {
	cl := m.Sheet().GetRef(m.Cur)
	if cl.Source != "" {
		return cl.Source
	}
	return cl.Value.Display()
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func (m *Model) maxRow() int { return int(m.Sheet().Rows()) - 1 }
func (m *Model) maxCol() int { return int(m.Sheet().Cols()) - 1 }

func (m *Model) setCursor(ref grid.Ref, extend bool) {
	if ref.Row < 0 {
		ref.Row = 0
	}
	if ref.Col < 0 {
		ref.Col = 0
	}
	if int(ref.Row) > m.maxRow() {
		ref.Row = uint32(m.maxRow())
	}
	if int(ref.Col) > m.maxCol() {
		ref.Col = uint32(m.maxCol())
	}
	if extend {
		if !m.HasSel {
			m.Anchor = m.Cur
			m.HasSel = true
		}
	} else {
		m.HasSel = false
		m.Anchor = ref
	}
	m.Cur = ref
	m.Focus = FocusCell
	m.ensureVisible()
}

func (m *Model) move(dr, dc int, extend bool) {
	m.setCursor(grid.Ref{
		Row: uint32(clampInt(int(m.Cur.Row)+dr, 0, m.maxRow())),
		Col: uint32(clampInt(int(m.Cur.Col)+dc, 0, m.maxCol())),
	}, extend)
}

// enterKey is what Enter does on a cell that is not being edited.
//
// On a cell with content it opens the editor seeded with that content, so a
// value can be amended rather than typed again from scratch. On an empty cell
// there is nothing to amend, so it moves down as it always has.
func (m *Model) enterKey() {
	cl := m.Sheet().GetRef(m.Cur)
	if cl.Source != "" || !cl.Value.IsEmpty() {
		m.beginEdit(m.currentText())
		return
	}
	m.move(1, 0, false)
}

// The header rows and columns are where +/- resizes, so they have to be
// reachable with the keyboard. Going *past* the edge of the grid steps onto the
// header; going back steps into the grid.

// arrowUp steps onto the column header from the first row, and moves between
// rows while the row header is focused so consecutive rows can be resized.
func (m *Model) arrowUp(extend bool) {
	switch m.Focus {
	case FocusColHeader:
		m.Focus = FocusCell
		return
	case FocusRowHeader:
		m.move(-1, 0, false)
		m.Focus = FocusRowHeader
		return
	}
	if m.Cur.Row == 0 {
		m.Focus = FocusColHeader
		m.HasSel = false
		return
	}
	m.move(-1, 0, extend)
}

// arrowDown steps back into the grid from the column header, and moves between
// rows while the row header is focused.
func (m *Model) arrowDown(extend bool) {
	switch m.Focus {
	case FocusColHeader:
		m.Focus = FocusCell
		return
	case FocusRowHeader:
		m.move(1, 0, false)
		m.Focus = FocusRowHeader
		return
	}
	m.move(1, 0, extend)
}

// arrowLeft steps onto the row header from the first column, and moves between
// columns while the column header is focused.
func (m *Model) arrowLeft(extend bool) {
	switch m.Focus {
	case FocusColHeader:
		// Stay on the header, so a column can be resized without leaving it.
		m.move(0, -1, false)
		m.Focus = FocusColHeader
		return
	case FocusRowHeader:
		return
	}
	if m.Cur.Col == 0 {
		m.Focus = FocusRowHeader
		m.HasSel = false
		return
	}
	m.move(0, -1, extend)
}

// arrowRight steps back into the grid from the row header.
func (m *Model) arrowRight(extend bool) {
	switch m.Focus {
	case FocusColHeader:
		m.move(0, 1, false)
		m.Focus = FocusColHeader
		return
	case FocusRowHeader:
		m.Focus = FocusCell
		return
	}
	m.move(0, 1, extend)
}

func (m *Model) jumpToRowStart(extend bool) {
	m.setCursor(grid.Ref{Row: m.Cur.Row, Col: 0}, extend)
}

func (m *Model) jumpToRowEnd(extend bool) {
	last := 0
	m.Sheet().Each(func(ref grid.Ref, cl cell.Cell) bool {
		if ref.Row == m.Cur.Row && int(ref.Col) > last {
			last = int(ref.Col)
		}
		return true
	})
	m.setCursor(grid.Ref{Row: m.Cur.Row, Col: uint32(last)}, extend)
}

func (m *Model) jumpToLastUsed() {
	var maxR, maxC uint32
	found := false
	m.Sheet().Each(func(ref grid.Ref, cl cell.Cell) bool {
		found = true
		if ref.Row > maxR {
			maxR = ref.Row
		}
		if ref.Col > maxC {
			maxC = ref.Col
		}
		return true
	})
	if !found {
		m.setCursor(grid.Ref{}, false)
		return
	}
	m.setCursor(grid.Ref{Row: maxR, Col: maxC}, false)
}

// jumpNonEmpty moves to the next populated cell in a direction, skipping blanks.
func (m *Model) jumpNonEmpty(dr, dc int, extend bool) {
	r, c := int(m.Cur.Row), int(m.Cur.Col)
	for {
		r += dr
		c += dc
		if r < 0 || c < 0 || r > m.maxRow() || c > m.maxCol() {
			r = clampInt(r, 0, m.maxRow())
			c = clampInt(c, 0, m.maxCol())
			break
		}
		if m.Sheet().Has(uint32(r), uint32(c)) {
			break
		}
	}
	m.setCursor(grid.Ref{Row: uint32(r), Col: uint32(c)}, extend)
}

// rowPlan is the rows the renderer will actually draw, which depends on each
// row's height. Scrolling must be based on this and not on an estimate: an
// estimate that is too generous scrolls the cursor off the bottom of the grid.
func (m *Model) rowPlan() []layout.PlanRow {
	sh := m.Sheet()
	if sh == nil {
		return nil
	}
	geo := m.geometry()
	return geo.PlanRows(m.TopRow, int(sh.Rows()), func(r int) int {
		return sh.RowHeight(uint32(r))
	})
}

// visibleRows is how many rows are on screen.
func (m *Model) visibleRows() int {
	if n := len(m.rowPlan()); n > 0 {
		return n
	}
	return 1
}

// visibleColsFrom is how many columns fit starting at a given column. It uses
// the same geometry the renderer draws with, so scrolling and drawing cannot
// disagree about where a column ends.
func (m *Model) visibleColsFrom(left int) int {
	return max1(m.geometryAt(left).Visible())
}

// ensureVisible scrolls so that the cursor is actually inside the drawn area,
// vertically and horizontally.
func (m *Model) ensureVisible() {
	sh := m.Sheet()
	if sh == nil {
		return
	}
	cur := int(m.Cur.Row)

	// Vertical. Start by assuming the cursor's row is the top row, then walk
	// backwards while the row above still leaves the cursor on screen. That is
	// at most one screenful of steps, however far the cursor jumped.
	if cur < m.TopRow {
		m.TopRow = cur
	} else {
		m.TopRow = cur
		// Row heights do not depend on the scroll offset, so the geometry is
		// built once rather than inside the loop.
		geo := m.geometryAt(0)
		for m.TopRow > 0 {
			plan := geo.PlanRows(m.TopRow-1, int(sh.Rows()), func(r int) int {
				return sh.RowHeight(uint32(r))
			})
			if len(plan) == 0 || plan[len(plan)-1].Row < cur {
				break
			}
			m.TopRow--
		}
	}
	if m.TopRow < 0 {
		m.TopRow = 0
	}
	if m.TopRow > cur {
		m.TopRow = cur
	}
	// A cursor past the end of the sheet would otherwise plan an empty
	// viewport, which renders as a grid with no rows at all.
	if last := int(sh.Rows()) - 1; m.TopRow > last {
		m.TopRow = max(0, last)
	}

	// Horizontal.
	if int(m.Cur.Col) < m.LeftCol {
		m.LeftCol = int(m.Cur.Col)
	}
	for m.LeftCol < int(m.Cur.Col) && int(m.Cur.Col) >= m.LeftCol+m.visibleColsFrom(m.LeftCol) {
		m.LeftCol++
	}
	if m.LeftCol < 0 {
		m.LeftCol = 0
	}
}

// ---------------------------------------------------------------------------
// Selection, clearing, structural edits
// ---------------------------------------------------------------------------

// Selection returns the normalised selection rectangle and whether more than
// one cell is selected.
func (m *Model) Selection() (r0, c0, r1, c1 int, ok bool) {
	if !m.HasSel {
		return 0, 0, 0, 0, false
	}
	r0, r1 = int(m.Anchor.Row), int(m.Cur.Row)
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	c0, c1 = int(m.Anchor.Col), int(m.Cur.Col)
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	if r0 == r1 && c0 == c1 {
		return 0, 0, 0, 0, false
	}
	return r0, c0, r1, c1, true
}

func (m *Model) clearSelection() {
	sh := m.Sheet()
	count := 0
	clear := func(ref grid.Ref) {
		if m.Rec != nil {
			m.Rec.BeforeCell(sh.ID, ref, sh.GetRef(ref))
		}
		m.Eng.Clear(context.Background(), sh, ref)
		count++
	}
	if r0, c0, r1, c1, ok := m.Selection(); ok {
		for r := r0; r <= r1; r++ {
			for c := c0; c <= c1; c++ {
				clear(grid.Ref{Row: uint32(r), Col: uint32(c)})
			}
		}
	} else {
		clear(m.Cur)
	}
	kind, label := journal.KindCellEdited, "Cleared "+grid.RefName(m.Cur.Row, m.Cur.Col)
	if count > 1 {
		kind, label = journal.KindBulkPaste, "Cleared "+itoa(count)+" cells"
	}
	m.record(kind, label)
	m.Dirty = true
	m.SetStatus(false, "Cleared")
}

func (m *Model) selectAll() {
	m.Anchor = grid.Ref{}
	m.Cur = grid.Ref{Row: uint32(m.maxRow()), Col: uint32(m.maxCol())}
	m.HasSel = true
	m.ensureVisible()
}

func (m *Model) deleteRow() {
	sh := m.Sheet()
	row := m.Cur.Row
	if m.Rec != nil {
		m.Rec.BeforeSnapshot(sh)
	}
	// Shift every cell below up by one.
	type move struct {
		ref grid.Ref
		cl  cell.Cell
	}
	var moves []move
	var clears []grid.Ref
	sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
		switch {
		case ref.Row == row:
			clears = append(clears, ref)
		case ref.Row > row:
			clears = append(clears, ref)
			moves = append(moves, move{grid.Ref{Row: ref.Row - 1, Col: ref.Col}, cl})
		}
		return true
	})
	for _, ref := range clears {
		sh.Clear(ref.Row, ref.Col)
	}
	for _, mv := range moves {
		sh.Set(mv.ref.Row, mv.ref.Col, mv.cl)
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	m.record(journal.KindStructural, "Row "+itoa(int(row)+1)+" deleted")
	m.Dirty = true
	m.ensureVisible()
	m.SetStatus(false, "Deleted row %d", row+1)
}

func (m *Model) deleteColumn() {
	sh := m.Sheet()
	col := m.Cur.Col
	if m.Rec != nil {
		m.Rec.BeforeSnapshot(sh)
	}
	type move struct {
		ref grid.Ref
		cl  cell.Cell
	}
	var moves []move
	var clears []grid.Ref
	sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
		switch {
		case ref.Col == col:
			clears = append(clears, ref)
		case ref.Col > col:
			clears = append(clears, ref)
			moves = append(moves, move{grid.Ref{Row: ref.Row, Col: ref.Col - 1}, cl})
		}
		return true
	})
	for _, ref := range clears {
		sh.Clear(ref.Row, ref.Col)
	}
	for _, mv := range moves {
		sh.Set(mv.ref.Row, mv.ref.Col, mv.cl)
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	m.record(journal.KindStructural, "Column "+grid.ColName(int(col))+" deleted")
	m.Dirty = true
	m.ensureVisible()
	m.SetStatus(false, "Deleted column %s", grid.ColName(int(col)))
}

// insertRow shifts everything at and below the cursor down one row and leaves
// the cursor's row empty.
func (m *Model) insertRow() {
	sh := m.Sheet()
	row := m.Cur.Row
	if m.Rec != nil {
		m.Rec.BeforeSnapshot(sh)
	}
	type moved struct {
		ref grid.Ref
		cl  cell.Cell
	}
	var moves []moved
	var clears []grid.Ref
	sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
		if ref.Row >= row {
			moves = append(moves, moved{grid.Ref{Row: ref.Row + 1, Col: ref.Col}, cl})
			clears = append(clears, ref)
		}
		return true
	})
	for _, ref := range clears {
		sh.Clear(ref.Row, ref.Col)
	}
	shifted := 0
	for _, mv := range moves {
		if mv.ref.Row >= grid.MaxRows {
			continue // pushed off the bottom of the sheet
		}
		sh.GrowToFit(mv.ref.Row, mv.ref.Col)
		sh.Set(mv.ref.Row, mv.ref.Col, mv.cl)
		shifted++
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	m.record(journal.KindStructural, "Row "+itoa(int(row)+1)+" inserted")
	m.Dirty = true
	m.SetStatus(false, "Inserted a row above r%d", row+1)
}

// insertColumn shifts everything at and right of the cursor one column right.
func (m *Model) insertColumn() {
	sh := m.Sheet()
	col := m.Cur.Col
	if m.Rec != nil {
		m.Rec.BeforeSnapshot(sh)
	}
	type moved struct {
		ref grid.Ref
		cl  cell.Cell
	}
	var moves []moved
	var clears []grid.Ref
	sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
		if ref.Col >= col {
			moves = append(moves, moved{grid.Ref{Row: ref.Row, Col: ref.Col + 1}, cl})
			clears = append(clears, ref)
		}
		return true
	})
	for _, ref := range clears {
		sh.Clear(ref.Row, ref.Col)
	}
	for _, mv := range moves {
		if mv.ref.Col >= grid.MaxCols {
			continue
		}
		sh.GrowToFit(mv.ref.Row, mv.ref.Col)
		sh.Set(mv.ref.Row, mv.ref.Col, mv.cl)
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	m.record(journal.KindStructural, "Column "+grid.ColName(int(col))+" inserted")
	m.Dirty = true
	m.SetStatus(false, "Inserted a column before %s", grid.ColName(int(col)))
}

// resizeFocused implements the +/- gesture on a row or column header.
func (m *Model) resizeFocused(delta int) {
	sh := m.Sheet()
	switch m.Focus {
	case FocusRowHeader:
		cur := sh.RowHeight(m.Cur.Row)
		if m.Rec != nil {
			m.Rec.BeforeRow(sh.ID, m.Cur.Row, cur)
		}
		if !sh.SetRowHeight(m.Cur.Row, cur+delta) {
			m.SetStatus(true, "Row height is limited to %d-%d lines", grid.MinRowHeight, grid.MaxRowHeight)
			return
		}
		m.ensureVisible()
		m.record(journal.KindRowEdited, "Row "+itoa(int(m.Cur.Row)+1)+" height")
		m.SetStatus(false, "Row %d height %d", m.Cur.Row+1, sh.RowHeight(m.Cur.Row))
	case FocusColHeader:
		cur := sh.ColWidth(m.Cur.Col)
		if m.Rec != nil {
			m.Rec.BeforeCol(sh.ID, m.Cur.Col, cur)
		}
		if !sh.SetColWidth(m.Cur.Col, cur+delta) {
			m.SetStatus(true, "Column width is limited to %d-%d characters", grid.MinColWidth, grid.MaxColWidth)
			return
		}
		// Enlarging a column can push it past the right-hand edge, so the view
		// is scrolled to keep the column being resized on screen. Without
		// this, widening a column made it disappear.
		m.ensureVisible()
		m.record(journal.KindStructural, "Column "+grid.ColName(int(m.Cur.Col))+" width")
		m.SetStatus(false, "Column %s width %d", grid.ColName(int(m.Cur.Col)), sh.ColWidth(m.Cur.Col))
	default:
		// Not on a header: nothing happens, which is better than surprising
		// the user by resizing the row they are editing.
		m.SetStatus(false, "Press Left at column A for the row header, or Up at row 1 for the column header")
	}
}

// openPathPrompt starts the single-line file prompt.
func (m *Model) openPathPrompt(kind PromptKind, title string) {
	m.openTextPrompt(kind, title, "")
}

// openTextPrompt starts a single-line prompt with optional initial text.
func (m *Model) openTextPrompt(kind PromptKind, title, initial string) {
	m.Mode = ModePrompt
	m.Prompt = kind
	m.PromptMsg = title
	m.PathBuf = initial
}

// switchSheet moves between tabs, wrapping around, as Ctrl+PgDn/PgUp does in
// Excel.
func (m *Model) switchSheet(delta int) {
	n := m.Wb.Len()
	if n < 2 {
		m.SetStatus(false, "There is only one sheet")
		return
	}
	idx := (m.Wb.ActiveIndex() + delta + n) % n
	m.Wb.SetActive(idx)
	m.HasSel = false
	m.Cur = grid.Ref{}
	m.Anchor = grid.Ref{}
	m.TopRow, m.LeftCol = 0, 0
	m.SetStatus(false, "Switched to %s", m.Sheet().Name)
}

// addSheet appends a new sheet and switches to it.
func (m *Model) addSheet() {
	name := m.Wb.NextSheetName()
	sh, err := m.Wb.AddSheet(name)
	if err != nil {
		m.SetStatus(true, "Could not add a sheet: %v", err)
		return
	}
	if m.Rec != nil {
		m.Rec.BeforeSheetAdd(m.Wb.IndexOf(sh), journal.SheetState{ID: sh.ID, Name: name})
	}
	m.Wb.SetActive(m.Wb.IndexOf(sh))
	m.record(journal.KindStructural, "Sheet "+name+" added")
	m.Dirty = true
	m.SetStatus(false, "Added %s", name)
}

// renameSheet renames the active sheet.
func (m *Model) renameSheet(name string) {
	idx := m.Wb.ActiveIndex()
	old := m.Sheets()[idx].Name
	if name == old {
		return
	}
	if m.Rec != nil {
		m.Rec.BeforeSheetRename(m.Wb.Sheets()[idx].ID, old)
	}
	if err := m.Wb.RenameSheet(idx, name); err != nil {
		m.SetStatus(true, "%v", err)
		return
	}
	m.record(journal.KindStructural, "Sheet renamed to "+name)
	m.Dirty = true
	m.SetStatus(false, "Renamed %s to %s", old, name)
}

// deleteSheet removes the active sheet, keeping at least one.
func (m *Model) deleteSheet() {
	idx := m.Wb.ActiveIndex()
	if m.Wb.Len() <= 1 {
		m.SetStatus(true, "The last sheet cannot be deleted")
		return
	}
	sh := m.Wb.Sheets()[idx]
	if m.Rec != nil {
		m.Rec.BeforeSheetRemove(idx, journal.CaptureSheet(sh))
	}
	name := sh.Name
	if err := m.Wb.DeleteSheet(idx); err != nil {
		m.SetStatus(true, "%v", err)
		return
	}
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	m.record(journal.KindStructural, "Sheet "+name+" deleted")
	m.Dirty = true
	m.ensureVisible()
	m.SetStatus(false, "Deleted sheet %s", name)
}

// Sheets is the workbook's sheet list.
func (m *Model) Sheets() []*grid.Sheet { return m.Wb.Sheets() }

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------

// selectionRect is the block to act on: the selection, or the single cell.
func (m *Model) selectionRect() (r0, c0, r1, c1 int) {
	if a, b, c, d, ok := m.Selection(); ok {
		return a, b, c, d
	}
	return int(m.Cur.Row), int(m.Cur.Col), int(m.Cur.Row), int(m.Cur.Col)
}

func (m *Model) copySelection(cut bool) {
	sh := m.Sheet()
	r0, c0, r1, c1 := m.selectionRect()
	if cut {
		m.Clip = clipboard.Cut(sh, r0, c0, r1, c1)
		m.SetStatus(false, "Cut %d cell(s)", (r1-r0+1)*(c1-c0+1))
		return
	}
	m.Clip = clipboard.Copy(sh, r0, c0, r1, c1)
	m.SetStatus(false, "Copied %d cell(s)", (r1-r0+1)*(c1-c0+1))
}

func (m *Model) pasteClipboard() {
	if m.Clip.Empty() {
		m.SetStatus(true, "Clipboard is empty")
		return
	}
	m.pasteBuffer(m.Clip)
}

// pasteText handles a paste from another application.
func (m *Model) pasteText(text string) {
	b := clipboard.FromTSV(text)
	if b.Empty() {
		return
	}
	m.pasteBuffer(b)
}

// pasteBuffer writes a block with its top-left corner at the cursor.
//
// The whole paste is one transaction: one delta, one recalculation, one undo
// step. A paste that overflows the sheet grows it by whole blocks first, so a
// 500-cell paste resizes once rather than 500 times.
func (m *Model) pasteBuffer(b *clipboard.Buffer) {
	sh := m.Sheet()
	origin := m.Cur

	// A cut clears its source once the paste has landed.
	var (
		cutSheet  *grid.Sheet
		cutOrigin grid.Ref
		cutW      = b.W
		cutH      = b.H
	)
	if b.IsCut() {
		if s, ok := m.Wb.SheetByID(b.SourceSheet()); ok {
			cutSheet, cutOrigin = s, b.Origin()
		}
	}
	if m.Rec != nil {
		// A paste can move a whole block and grow the sheet, so a snapshot of
		// the sheet is the only description that stays obviously correct.
		m.Rec.BeforeSnapshot(sh)
	}

	ctx := context.Background()
	pasted, skipped := 0, 0
	for r := 0; r < b.H; r++ {
		for c := 0; c < b.W; c++ {
			ref := grid.Ref{Row: origin.Row + uint32(r), Col: origin.Col + uint32(c)}
			if ref.Row >= grid.MaxRows || ref.Col >= grid.MaxCols {
				skipped++
				continue
			}
			sh.GrowToFit(ref.Row, ref.Col)
			m.Eng.PutQuiet(sh, ref, b.At(r, c))
			pasted++
		}
	}
	if cutSheet != nil {
		for r := 0; r < cutH; r++ {
			for c := 0; c < cutW; c++ {
				ref := grid.Ref{Row: cutOrigin.Row + uint32(r), Col: cutOrigin.Col + uint32(c)}
				m.Eng.PutQuiet(cutSheet, ref, cell.Cell{})
			}
		}
	}

	// One pass for the whole paste.
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(ctx)

	// Select what was pasted, so the result is visible and can be acted on.
	m.Anchor = origin
	m.Cur = grid.Ref{
		Row: min(origin.Row+uint32(b.H)-1, uint32(grid.MaxRows-1)),
		Col: min(origin.Col+uint32(b.W)-1, uint32(grid.MaxCols-1)),
	}
	m.HasSel = b.W > 1 || b.H > 1

	kind := journal.KindCellEdited
	if pasted > 1 {
		kind = journal.KindBulkPaste
	}
	label := "Pasted " + itoa(pasted) + " cells"
	if b.IsCut() {
		label = "Moved " + itoa(pasted) + " cells"
	}
	m.record(kind, label)
	m.Dirty = true
	m.ensureVisible()
	if skipped > 0 {
		m.SetStatus(true, "Pasted %d cells; %d fell outside the sheet", pasted, skipped)
		return
	}
	m.SetStatus(false, "%s", label)
}

// record tells the application that a recorded mutation just finished.
func (m *Model) record(kind journal.Kind, label string) {
	if m.OnEdit != nil {
		m.OnEdit(kind, label)
	}
}

// undo reverses the most recent recorded change.
func (m *Model) undo() {
	if m.Undo == nil {
		return
	}
	if _, ok := m.Undo.Undo(m.Wb); !ok {
		m.SetStatus(true, "Nothing to undo")
		return
	}
	m.afterHistoryChange("Undone")
}

// redo reapplies the most recently undone change.
func (m *Model) redo() {
	if m.Undo == nil {
		return
	}
	if _, ok := m.Undo.Redo(m.Wb); !ok {
		m.SetStatus(true, "Nothing to redo")
		return
	}
	m.afterHistoryChange("Redone")
}

// afterHistoryChange re-synchronises the engine after undo, redo or rollback:
// formulas must be re-indexed because sources may have changed wholesale, and
// the affected cells recomputed.
func (m *Model) afterHistoryChange(word string) {
	m.Eng.RebuildGraph()
	m.Eng.RecalcWorkbook(context.Background())
	if m.History != nil && m.History.Len() < m.Wb.Len() {
		// nothing; kept for clarity
	}
	m.Dirty = true
	m.ensureVisible()
	if m.OnDirty != nil {
		m.OnDirty()
	}
	m.SetStatus(false, "%s", word)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// dropLastRune removes the final rune, which for UTF-8 is one or more bytes.
func dropLastRune(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i]&0xC0 != 0x80 {
			return s[:i]
		}
	}
	return ""
}

func trimSpace(s string) string { return strings.TrimSpace(s) }
