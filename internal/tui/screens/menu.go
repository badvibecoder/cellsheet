package screens

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"
)

// themeT is the theme type, aliased so the many draw helpers read cleanly.
type themeT = theme.Theme

// menuItem is one row of a drop-down menu.
type menuItem struct {
	label string
	key   string
	sep   bool
}

// menuFor returns the items of one of the four menus.
func menuFor(which int) (string, []menuItem) {
	switch which {
	case 1: // Edit
		return "Edit", []menuItem{
			{label: "Copy", key: "Ctrl+C"},
			{label: "Cut", key: "Ctrl+X"},
			{label: "Paste", key: "Ctrl+V"},
			{sep: true},
			{label: "Insert row above"},
			{label: "Insert column left"},
			{label: "Delete row", key: "Alt+D"},
			{label: "Delete column", key: "Alt+C"},
			{sep: true},
			{label: "Add sheet", key: "Ctrl+T"},
			{label: "Rename sheet…"},
			{label: "Delete sheet"},
			{sep: true},
			{label: "Select all", key: "Ctrl+A"},
		}
	case 2: // Rollback
		return "Rollback", []menuItem{
			{label: "Show checkpoint history", key: "Alt+R"},
			{label: "Create checkpoint now", key: "Ctrl+B"},
		}
	case 3: // View
		return "View", []menuItem{
			{label: "Go to cell…", key: "Ctrl+G"},
			{label: "Recalculate"},
			{sep: true},
			{label: "Function list", key: "F1"},
		}
	default: // File
		return "File", []menuItem{
			{label: "New", key: "Ctrl+N"},
			{label: "Open…", key: "Ctrl+O"},
			{sep: true},
			{label: "Save", key: "Ctrl+S"},
			{label: "Save As…", key: "Ctrl+Shift+S"},
			{sep: true},
			{label: "Exit", key: "Ctrl+Q"},
		}
	}
}

// menuLabels are the words on the menu bar. menuCount is their number; it is a
// constant so the arrays that record where they were drawn can be sized.
var menuLabels = []string{"File", "Edit", "Rollback", "View"}

const menuCount = 4

func (m *Model) drawMenu(c *render.Canvas) {
	// Highlight the open menu's own name where the top bar actually drew it.
	// Redrawing the whole bar here was the bug: the names were re-emitted at
	// offsets that did not match the bar's spacing, so "View" landed on top of
	// "Rollback (Alt+R)" and left the old text sticking out after it.
	if m.MenuWhich >= 0 && m.MenuWhich < menuCount && m.menuTitleOK[m.MenuWhich] {
		label := menuLabels[m.MenuWhich]
		c.Draw(m.menuTitleX[m.MenuWhich], 0, label, m.Th.PanelSel)
	}

	_, items := menuFor(m.MenuWhich)
	inner := 0
	for _, it := range items {
		n := render.StringWidth("  " + it.label)
		if it.key != "" {
			n += 2 + render.StringWidth(it.key)
		}
		if n > inner {
			inner = n
		}
	}
	inner += 3
	boxW := inner + 2
	boxH := len(items) + 2

	p := render.NewCanvas(boxW, boxH)
	p.DrawBox(0, 0, boxW, boxH, m.Th.PanelBorder)

	row := 0
	for _, it := range items {
		row++
		if it.sep {
			p.HRule(row, 1, boxW-2, '─', nil, m.Th.Dim)
			continue
		}
		st := m.Th.PanelItem
		if row-1 == m.MenuIdx {
			st = m.Th.PanelSel
		}
		line := "  " + it.label
		if it.key != "" {
			pad := inner - render.StringWidth(line) - render.StringWidth(it.key) - 1
			if pad < 1 {
				pad = 1
			}
			line += repeat(" ", pad) + it.key
		}
		p.Draw(1, row, render.PadRight(line, inner), st)
	}
	c.Blit(p, 2, 1)
}

func (m *Model) openRollback() {
	m.Mode = ModeRollback
	m.RbIdx = 0
	m.RbTyped = ""
}

func (m *Model) handleMenuKey(key string) tea.Cmd {
	if !m.MenuOpen {
		return nil
	}
	_, items := menuFor(m.MenuWhich)
	switch key {
	case "esc":
		m.MenuOpen = false
	case "up":
		m.menuStep(items, -1)
	case "down":
		m.menuStep(items, +1)
	case "left":
		m.MenuWhich = (m.MenuWhich + len(menuLabels) - 1) % len(menuLabels)
		m.MenuIdx = 0
	case "right":
		m.MenuWhich = (m.MenuWhich + 1) % len(menuLabels)
		m.MenuIdx = 0
	case "enter":
		m.MenuOpen = false
		m.runMenuItem(m.MenuWhich, m.MenuIdx)
	}
	return nil
}

func (m *Model) menuStep(items []menuItem, dir int) {
	n := len(items)
	for i := 0; i < n; i++ {
		m.MenuIdx = (m.MenuIdx + dir + n) % n
		if !items[m.MenuIdx].sep {
			return
		}
	}
}

func (m *Model) runMenuItem(which, idx int) {
	_, items := menuFor(which)
	if idx < 0 || idx >= len(items) {
		return
	}
	item := items[idx]
	switch which {
	case 0: // File
		switch item.label {
		case "New":
			m.NewRequested = true
		case "Open…":
			// The prompt has to be opened here; raising OpenRequested with no
			// path is a request the application cannot act on, which is why
			// the menu item used to do nothing.
			m.openPathPrompt(PromptOpen, "Open")
		case "Save":
			m.requestSave()
		case "Save As…":
			m.openTextPrompt(PromptSaveAs, "Save as", m.Path)
		case "Exit":
			m.Quit = true
		}
	case 1: // Edit
		switch item.label {
		case "Copy":
			m.copySelection(false)
		case "Cut":
			m.copySelection(true)
		case "Paste":
			m.pasteClipboard()
		case "Insert row above":
			m.insertRow()
		case "Insert column left":
			m.insertColumn()
		case "Delete row":
			m.deleteRow()
		case "Delete column":
			m.deleteColumn()
		case "Select all":
			m.selectAll()
		case "Add sheet":
			m.addSheet()
		case "Rename sheet…":
			m.openTextPrompt(PromptSheetName, "Rename sheet", m.Sheet().Name)
		case "Delete sheet":
			m.askConfirm("Delete sheet "+m.Sheet().Name+"?", func(ok bool) {
				if ok {
					m.deleteSheet()
				}
			})
		}
	case 2: // Rollback
		if item.label == "Show checkpoint history" {
			m.openRollback()
		} else {
			m.manualCheckpoint()
		}
	case 3: // View
		if item.label == "Recalculate" {
			m.Eng.RecalcWorkbook(nil)
			m.SetStatus(false, "Recalculated")
		} else {
			m.SetStatus(false, "%s is not available yet", item.label)
		}
	}
}

// ---------------------------------------------------------------------------
// Rollback panel
// ---------------------------------------------------------------------------

func (m *Model) drawRollback(c *render.Canvas) {
	const shown = 10 // 0 (current) plus the last nine
	max := m.History.Max()

	type entry struct {
		idx   int
		label string
		when  string
	}
	var entries []entry
	for i := 0; i <= max && i < shown; i++ {
		when := ""
		if t := m.History.When(i); !t.IsZero() {
			when = t.Format("3:04:05 PM")
		}
		entries = append(entries, entry{i, m.History.Label(i), when})
	}
	if len(entries) == 0 {
		entries = append(entries, entry{0, "Current State", ""})
	}

	nameW := 0
	for _, e := range entries {
		if n := render.StringWidth(itoa(e.idx) + ": " + e.label); n > nameW {
			nameW = n
		}
	}
	timeW := 0
	for _, e := range entries {
		if n := render.StringWidth("[" + e.when + "]"); n > timeW {
			timeW = n
		}
	}
	note := ""
	if m.History.Uncommitted() {
		note = "(Uncommitted)"
	}
	inner := 2 + nameW + 1 + timeW + 1 + render.StringWidth(note) + 1
	hint := "J: Jump to custom rollback index... (Max available: " + itoa(max) + ")"
	if n := render.StringWidth(hint) + 3; n > inner {
		inner = n
	}

	boxW := inner + 2
	boxH := len(entries) + 4
	p := render.NewCanvas(boxW, boxH)
	p.DrawBox(0, 0, boxW, boxH, m.Th.PanelBorder)

	y := 1
	for _, e := range entries {
		line := itoa(e.idx) + ": " + e.label
		left := " " + render.PadRight(line, nameW) + " " + render.PadLeft("["+e.when+"]", timeW)
		if e.idx == 0 && note != "" {
			left += " " + note
		}
		st := m.Th.PanelItem
		if e.idx == m.RbIdx {
			st = m.Th.PanelSel
		}
		p.Draw(1, y, render.PadRight(left, inner), st)
		y++
	}
	p.HRule(y, 1, boxW-2, '─', nil, m.Th.Dim)
	y++
	p.Draw(1, y, render.PadRight(" "+hint, inner), m.Th.PanelHint)

	x := (c.W - boxW) / 2
	if x < 1 {
		x = 1
	}
	c.Blit(p, x, 2)
}

func (m *Model) drawJumpPrompt(c *render.Canvas) {
	body := "Rollback to index (1-" + itoa(m.History.Max()) + "): " + m.RbTyped + "█"
	inner := render.StringWidth(body) + 2
	boxW := inner + 2
	p := render.NewCanvas(boxW, 5)
	p.DrawBox(0, 0, boxW, 5, m.Th.PanelBorder)
	p.Draw(1, 1, render.PadRight(" Rollback", inner), m.Th.PanelSel)
	p.Draw(1, 2, render.PadRight(" "+body, inner), m.Th.PanelItem)
	p.Draw(1, 3, render.PadRight(" Enter = go   Esc = cancel", inner), m.Th.PanelHint)
	x := (c.W - boxW) / 2
	if x < 1 {
		x = 1
	}
	y := (c.H - 5) / 2
	if y < 1 {
		y = 1
	}
	c.Blit(p, x, y)
}

func (m *Model) handleRollbackKey(key string) tea.Cmd {
	max := m.History.Max()
	switch key {
	case "esc":
		m.Mode = ModeReady
	case "j":
		m.Mode = ModePrompt
		m.Prompt = PromptRollback
		m.RbTyped = ""
	case "up":
		if m.RbIdx > 0 {
			m.RbIdx--
		}
	case "down":
		if m.RbIdx < max && m.RbIdx < 9 {
			m.RbIdx++
		}
	case "enter":
		m.Mode = ModeReady
		if m.RbIdx == 0 {
			m.SetStatus(false, "Already at the current state")
			return nil
		}
		m.rollbackTo(m.RbIdx)
	}
	return nil
}

// rollbackTo moves the workbook to a rollback index.
func (m *Model) rollbackTo(k int) {
	if err := m.History.Rollback(m.Wb, k, time.Now()); err != nil {
		m.SetStatus(true, "Rollback failed: %v", err)
		return
	}
	// The workbook changed wholesale, so the per-edit undo stack no longer
	// describes it.
	if m.Undo != nil {
		m.Undo.Reset()
	}
	m.afterHistoryChange("Rolled back to index " + itoa(k))
}

func (m *Model) handlePromptKey(msg tea.KeyMsg) tea.Cmd {
	if m.Prompt == PromptRollback {
		return m.handleRollbackPrompt(msg)
	}
	return m.handlePathPrompt(msg)
}

func (m *Model) handleRollbackPrompt(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.Mode = ModeRollback
		m.RbTyped = ""
	case "backspace":
		if m.RbTyped != "" {
			m.RbTyped = m.RbTyped[:len(m.RbTyped)-1]
		}
	case "enter":
		n := 0
		ok := m.RbTyped != ""
		for _, r := range m.RbTyped {
			if r < '0' || r > '9' {
				ok = false
				break
			}
			n = n*10 + int(r-'0')
		}
		if !ok || n < 1 || n > m.History.Max() {
			m.SetStatus(true, "Enter a number between 1 and %d", m.History.Max())
			return nil
		}
		m.RbTyped = ""
		m.Mode = ModeReady
		m.rollbackTo(n)
	default:
		for _, r := range printableString(msg) {
			if r >= '0' && r <= '9' {
				m.RbTyped += string(r)
			}
		}
	}
	return nil
}

func (m *Model) handlePathPrompt(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.Mode = ModeReady
		m.PathBuf = ""
	case "backspace":
		if m.PathBuf != "" {
			m.PathBuf = m.PathBuf[:len(m.PathBuf)-1]
		}
	case "enter":
		value := trimSpace(m.PathBuf)
		if value == "" {
			m.SetStatus(true, "Enter a file name")
			return nil
		}
		kind := m.Prompt
		m.Mode = ModeReady
		m.PathBuf = ""
		if kind == PromptSheetName {
			// The name typed is the value just read; reading it back out of the
			// buffer after clearing it renamed every sheet to "".
			m.renameSheet(value)
			return nil
		}
		path := value
		if !strings.HasSuffix(strings.ToLower(path), ".cell") {
			path += ".cell"
		}
		switch kind {
		case PromptSaveAs:
			m.PendingPath = path
			m.SaveRequested = true
			m.SaveAs = true
		case PromptOpen:
			m.PendingPath = path
			m.OpenRequested = true
		}
	default:
		m.PathBuf += printableString(msg)
	}
	return nil
}

// drawPathPrompt renders the single-line file prompt.
func (m *Model) drawPathPrompt(c *render.Canvas) {
	body := m.PathBuf + "█"
	inner := maxInt(render.StringWidth(m.PromptMsg)+2, render.StringWidth(body)+2)
	inner = maxInt(inner, render.StringWidth(" Enter = confirm   Esc = cancel")+2)
	boxW := inner + 2
	p := render.NewCanvas(boxW, 5)
	p.DrawBox(0, 0, boxW, 5, m.Th.PanelBorder)
	p.Draw(1, 1, render.PadRight(" "+m.PromptMsg, inner), m.Th.PanelSel)
	p.Draw(1, 2, render.PadRight(" "+body, inner), m.Th.PanelItem)
	p.Draw(1, 3, render.PadRight(" Enter = confirm   Esc = cancel", inner), m.Th.PanelHint)
	x := (c.W - boxW) / 2
	if x < 1 {
		x = 1
	}
	y := (c.H - 5) / 2
	if y < 1 {
		y = 1
	}
	c.Blit(p, x, y)
}

func (m *Model) drawConfirm(c *render.Canvas) {
	body := m.ConfirmMsg
	hint := "y = yes    n = no"
	inner := render.StringWidth(body)
	if n := render.StringWidth(hint); n > inner {
		inner = n
	}
	inner += 4
	boxW := inner + 2
	p := render.NewCanvas(boxW, 5)
	p.DrawBox(0, 0, boxW, 5, m.Th.PanelBorder)
	p.Draw(1, 1, render.PadRight(" Confirm", inner), m.Th.PanelSel)
	p.Draw(1, 2, render.PadRight(" "+body, inner), m.Th.PanelItem)
	p.Draw(1, 3, render.PadRight(" "+hint, inner), m.Th.PanelHint)
	x := (c.W - boxW) / 2
	if x < 1 {
		x = 1
	}
	y := (c.H - 5) / 2
	if y < 1 {
		y = 1
	}
	c.Blit(p, x, y)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// Track the time so the status bar countdown refreshes.
var _ = time.Second
