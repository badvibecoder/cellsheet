package screens

import (
	"strings"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/tui/layout"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
)

// geometryAt builds the frame geometry for a given leftmost column.
func (m *Model) geometryAt(leftCol int) layout.Geometry {
	cols := layout.Cols{Left: leftCol}
	if sh := m.Sheet(); sh != nil {
		cols.Width = func(c int) int { return sh.ColWidth(uint32(c)) }
		sized := sh.ColsSet()
		cols.Sized = func(c int) bool {
			_, ok := sized[uint32(c)]
			return ok
		}
	}
	return layout.Compute(m.Width, m.Height, cols)
}

// geometry is the geometry for the current scroll position.
func (m *Model) geometry() layout.Geometry { return m.geometryAt(m.LeftCol) }

// View implements tea.Model. The whole frame is assembled in memory and
// emitted in one piece, which is what keeps it free of tearing.
func (m *Model) View() string {
	c := render.NewCanvas(m.Width, m.Height)
	if layout.TooSmall(m.Width, m.Height) {
		return tooSmall(c, m.Th)
	}
	geo := m.geometry()

	m.drawTopBar(c)
	m.drawFormulaBar(c, geo)
	m.drawColumnHeader(c, geo)
	m.drawGrid(c, geo)
	m.drawTabs(c, geo)
	m.drawStatus(c, geo)
	c.HRule(geo.BottomY, 0, geo.W-1, '─',
		map[int]rune{0: '└', geo.W - 1: '┘'}, m.Th.Frame)

	switch {
	case m.Mode == ModeConfirm:
		m.drawConfirm(c)
	case m.Mode == ModePrompt && m.Prompt == PromptRollback:
		m.drawRollback(c)
		m.drawJumpPrompt(c)
	case m.Mode == ModePrompt:
		m.drawPathPrompt(c)
	case m.Mode == ModeRollback:
		m.drawRollback(c)
	case m.MenuOpen:
		m.drawMenu(c)
	}
	return c.String(m.Color)
}

func tooSmall(c *render.Canvas, th theme0) string {
	msg := "Terminal too small"
	sub := "Needs at least " + itoa(layout.MinTermW) + "x" + itoa(layout.MinTermH) + " characters"
	c.Fill(0, 0, c.W, c.H, ' ', render.Style{})
	y := c.H / 2
	c.Draw(maxInt(0, (c.W-render.StringWidth(msg))/2), y-1, render.Truncate(msg, c.W), th.Error)
	c.Draw(maxInt(0, (c.W-render.StringWidth(sub))/2), y+1, render.Truncate(sub, c.W), th.Dim)
	return c.String(render.ColorNone)
}

// theme0 aliases the theme type so the helper above stays readable.
type theme0 = themeT

// topBarOptions are the menu strips, longest first. At a narrow width the
// shortcuts are dropped before anything else, because a key hint nobody can
// read is worth less than a filename.
var topBarOptions = []string{
	"─ File (Alt+F) ── Edit ── Rollback (Alt+R) ── View ",
	"─ File ── Edit ── Rollback (Alt+R) ── View ",
	"─ File ── Edit ── Rollback ── View ",
	"─ File ── Edit ── View ",
}

func (m *Model) drawTopBar(c *render.Canvas) {
	w := c.W
	mark := "○"
	if m.Dirty {
		mark = "●"
	}
	title := m.Path
	if title == "" {
		title = "untitled.cell"
	}
	inner := w - 2

	// Degrade gracefully. The frame must always close with a clean rule, so a
	// long title loses its prefix, then the menu loses its shortcuts, and only
	// then is the name itself truncated.
	titles := []string{
		" cellsheet: " + title + " [" + mark + "] ─",
		" " + title + " [" + mark + "] ─",
	}
	left, right := "", ""
	found := false
	for _, l := range topBarOptions {
		for _, t := range titles {
			if render.StringWidth(l)+render.StringWidth(t) <= inner {
				left, right, found = l, t, true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		left = topBarOptions[len(topBarOptions)-1]
		right = " ─"
		room := inner - render.StringWidth(left) - render.StringWidth(" ["+mark+"] ─")
		if room >= 4 {
			right = " " + render.Truncate(title, room) + " [" + mark + "] ─"
		}
		if render.StringWidth(left)+render.StringWidth(right) > inner {
			right = " ─"
		}
	}

	mid := ""
	if n := inner - render.StringWidth(left) - render.StringWidth(right); n > 0 {
		mid = strings.Repeat("─", n)
	}
	// Remember where each menu name landed; drawMenu highlights the open one in
	// place rather than re-emitting the bar at offsets of its own.
	m.recordMenuTitles(left)
	c.Draw(1, 0, left+mid+right, m.Th.MenuTitle)
	c.Set(0, 0, '┌', m.Th.Frame)
	c.Set(w-1, 0, '┐', m.Th.Frame)
}

// recordMenuTitles finds each menu name inside the menu bar that was drawn, so
// that an open menu's title can be highlighted exactly where it already is. A
// name that did not fit at this width is simply not highlightable.
func (m *Model) recordMenuTitles(left string) {
	for i, label := range menuLabels {
		if i >= menuCount {
			break
		}
		if idx := strings.Index(left, label); idx >= 0 {
			// strings.Index counts bytes and the bar is drawn in columns, so
			// the offset has to be measured in display width — the rule
			// characters are three bytes each.
			m.menuTitleX[i], m.menuTitleOK[i] = 1+render.StringWidth(left[:idx]), true
		} else {
			m.menuTitleOK[i] = false
		}
	}
}

func (m *Model) drawFormulaBar(c *render.Canvas, geo layout.Geometry) {
	w := c.W
	c.HRule(1, 0, w-1, '─', map[int]rune{0: '├', geo.RefW + 1: '┬', w - 1: '┤'}, m.Th.Frame)

	c.Set(0, 2, '│', m.Th.Frame)
	c.Set(geo.RefW+1, 2, '│', m.Th.Frame)
	c.Set(w-1, 2, '│', m.Th.Frame)

	refRow, refCol := m.Cur.Row, m.Cur.Col
	ref := grid.ColName(int(refCol)) + itoa(int(refRow)+1)
	c.Draw(1, 2, render.PadCenter(ref, geo.RefW), m.Th.RefBox)

	var body string
	if m.Mode == ModeEdit {
		body = " fx: " + m.EditBuf + "█"
	} else {
		body = " fx: " + m.currentText()
	}
	c.Draw(geo.RefW+2, 2, render.Truncate(body, w-2-(geo.RefW+2)), m.Th.FormulaBar)

	j := map[int]rune{0: '├', geo.RefW + 1: '┴', w - 1: '┤'}
	for _, x := range geo.ColLeft {
		j[x] = '┬'
	}
	c.HRule(3, 0, w-1, '─', j, m.Th.Frame)
}

func (m *Model) drawColumnHeader(c *render.Canvas, geo layout.Geometry) {
	y := 4
	c.Set(0, y, '│', m.Th.Frame)
	c.Set(geo.W-1, y, '│', m.Th.Frame)
	for i, x := range geo.ColLeft {
		c.Set(x, y, '│', m.Th.Frame)
		col := m.LeftCol + i
		st := m.Th.ColHeader
		label := grid.ColName(col)
		if col == int(m.Cur.Col) {
			st = m.Th.ColHeaderHi
			label = "[" + label + "]"
			if m.Focus == FocusColHeader {
				st = m.Th.CursorEdge.With(render.AttrUnderline)
			}
		}
		c.Draw(x+1, y, render.PadCenter(label, geo.ColWidth[i]), st)
	}
	j := map[int]rune{0: '├', geo.W - 1: '┤'}
	for _, x := range geo.ColLeft {
		j[x] = '┼'
	}
	c.HRule(5, 0, geo.W-1, '─', j, m.Th.Frame)

	if r0, _, _, _, ok := m.Selection(); ok && r0 == m.TopRow {
		if x0, x1, ok := m.selXRange(geo); ok {
			for x := x0; x <= x1; x++ {
				c.Set(x, 5, '┄', m.Th.SelEdge)
			}
		}
	}
}

func (m *Model) drawGrid(c *render.Canvas, geo layout.Geometry) {
	sh := m.Sheet()
	plan := geo.PlanRows(m.TopRow, int(sh.Rows()), func(r int) int { return sh.RowHeight(uint32(r)) })
	y := geo.GridTop
	for i, p := range plan {
		for sub := 0; sub < p.Lines; sub++ {
			m.drawGridLine(c, geo, y, p.Row, sub, p.Lines, p.RealHgt)
			y++
		}
		m.drawRowSeparator(c, geo, y, p.Row, i == len(plan)-1)
		y++
	}
}

func (m *Model) drawGridLine(c *render.Canvas, geo layout.Geometry, y, row, sub, lines, realHgt int) {
	sh := m.Sheet()
	w := geo.W
	c.Set(0, y, '│', m.Th.Frame)
	for _, x := range geo.ColLeft {
		c.Set(x, y, '│', m.Th.Frame)
	}
	c.Set(w-1, y, '│', m.Th.Frame)

	if sub == 0 {
		st := m.Th.RowHeader
		name := "r" + itoa(row+1)
		if row == int(m.Cur.Row) {
			st = m.Th.RowHeaderHi
			name = "[" + name + "]"
			if m.Focus == FocusRowHeader {
				st = m.Th.CursorEdge.With(render.AttrUnderline)
			}
		}
		c.Draw(1, y, render.PadRight(" "+name, geo.GutterW), st)
	} else if realHgt > 1 && sub == lines-1 {
		c.Draw(1, y, render.PadCenter("(height: "+itoa(realHgt)+")", geo.GutterW), m.Th.Dim)
	}

	// A row of height N occupies N lines, and its text sits on exactly one of
	// them. Drawing it on every line made a tall cell repeat its contents once
	// per line.
	//
	// Placement uses the row's declared height rather than the lines actually
	// drawn: the layout may absorb one spare line into the last visible row to
	// keep the closing rule tidy, and that padding must not shift the text.
	textLine := layout.TextLine(realHgt)

	r0, c0, r1, c1, hasSel := m.Selection()
	for i := range geo.ColLeft {
		col := m.LeftCol + i
		x := geo.ColLeft[i] + 1
		cw := geo.ColWidth[i]
		isActive := row == int(m.Cur.Row) && col == int(m.Cur.Col)
		selected := hasSel && inRect(row, col, r0, c0, r1, c1) && !isActive

		// A selection tints the whole cell, however tall it is.
		if selected {
			c.Fill(x, y, cw, 1, ' ', m.Th.SelText)
		}
		if sub == textLine {
			v := sh.Get(uint32(row), uint32(col)).Value
			st := styleFor(v, m.Th)
			if selected {
				st = m.Th.SelText
			}
			c.Draw(x, y, formatValue(v, cw), st)
		}
		if selected {
			if !inRect(row, col-1, r0, c0, r1, c1) {
				c.Set(geo.ColLeft[i], y, '┆', m.Th.SelEdge)
			}
			if !inRect(row, col+1, r0, c0, r1, c1) {
				c.Set(geo.ColLeft[i]+cw+1, y, '┆', m.Th.SelEdge)
			}
		}
	}

	// The active cell: heavy bars down the whole height so a tall row is still
	// clearly outlined, with the contents on the text line.
	if row == int(m.Cur.Row) && m.Focus == FocusCell {
		vi := int(m.Cur.Col) - m.LeftCol
		if vi >= 0 && vi < len(geo.ColLeft) {
			left := geo.ColLeft[vi]
			right := left + geo.ColWidth[vi] + 1
			c.Set(left, y, '║', m.Th.CursorEdge)
			c.Set(right, y, '║', m.Th.CursorEdge)
			if sub == textLine {
				c.Fill(left+1, y, geo.ColWidth[vi], 1, ' ', m.Th.Cursor)
				if m.Mode == ModeEdit && sameRef(m.EditRef, m.Cur) {
					c.Draw(left+1, y, render.Truncate(m.EditBuf+"█", geo.ColWidth[vi]), m.Th.Cursor)
				} else {
					v := sh.GetRef(m.Cur).Value
					c.Draw(left+1, y, formatValue(v, geo.ColWidth[vi]), m.Th.Cursor)
				}
			}
		}
	}
}

func (m *Model) drawRowSeparator(c *render.Canvas, geo layout.Geometry, y, row int, closing bool) {
	j := map[int]rune{0: '├'}
	for _, x := range geo.ColLeft {
		if closing {
			j[x] = '┴'
		} else {
			j[x] = '┼'
		}
	}
	j[geo.W-1] = '┤'
	c.HRule(y, 0, geo.W-1, '─', j, m.Th.Frame)

	if r0, _, r1, _, ok := m.Selection(); ok && (row == r1 || row+1 == r0) {
		if x0, x1, ok := m.selXRange(geo); ok {
			for x := x0; x <= x1; x++ {
				c.Set(x, y, '┄', m.Th.SelEdge)
			}
		}
	}
}

func (m *Model) selXRange(geo layout.Geometry) (int, int, bool) {
	_, c0, _, c1, ok := m.Selection()
	if !ok {
		return 0, 0, false
	}
	x0, x1 := -1, -1
	for i := range geo.ColLeft {
		col := m.LeftCol + i
		if col < c0 || col > c1 {
			continue
		}
		l := geo.ColLeft[i]
		r := l + geo.ColWidth[i] + 1
		if x0 < 0 || l < x0 {
			x0 = l
		}
		if r > x1 {
			x1 = r
		}
	}
	if x0 < 0 {
		return 0, 0, false
	}
	return x0, x1, true
}

func (m *Model) drawTabs(c *render.Canvas, geo layout.Geometry) {
	y := geo.TabsY
	c.Set(0, y, '│', m.Th.Frame)
	c.Set(geo.W-1, y, '│', m.Th.Frame)
	x := 2
	for i, sh := range m.Wb.Sheets() {
		name := sh.Name
		if i == m.Wb.ActiveIndex() && m.Dirty {
			name += "*"
		}
		label := "[ " + name + " ]"
		st := m.Th.TabIdle
		if i == m.Wb.ActiveIndex() {
			st = m.Th.TabActive
		}
		c.Draw(x, y, label, st)
		x += render.StringWidth(label) + 2
	}
	c.Draw(x, y, "[ + ]", m.Th.TabAdd)
}

func (m *Model) drawStatus(c *render.Canvas, geo layout.Geometry) {
	w := geo.W
	c.HRule(geo.StatusY-1, 0, w-1, '─', map[int]rune{0: '├', w - 1: '┤'}, m.Th.Frame)
	c.Set(0, geo.StatusY, '│', m.Th.Frame)
	c.Set(w-1, geo.StatusY, '│', m.Th.Frame)

	modeSt := m.Th.StatusMode
	if m.Mode != ModeReady || m.Focus != FocusCell {
		modeSt = m.Th.StatusWarn
	}
	segs := []statusSeg{{m.statusWord(), modeSt, -1}}

	sh := m.Sheet()
	info := itoa(int(sh.Rows())) + "R x " + itoa(int(sh.Cols())) + "C"
	if r0, c0, r1, c1, ok := m.Selection(); ok {
		info = itoa(r1-r0+1) + "R x " + itoa(c1-c0+1) + "C"
		if sum, ok := m.selectionSum(r0, c0, r1, c1); ok {
			info = "Sum: " + sum + " · " + info
		}
	}
	segs = append(segs, statusSeg{info, m.Th.StatusVal, -1})

	kind := sh.GetRef(m.Cur).Value.Kind.String()
	switch m.Focus {
	case FocusRowHeader:
		kind = "Row header"
	case FocusColHeader:
		kind = "Column header"
	}
	segs = append(segs, statusSeg{"Active: " + grid.ColName(int(m.Cur.Col)) + itoa(int(m.Cur.Row)+1) + " (" + kind + ")", m.Th.StatusVal, -1})
	if m.Status != "" && time.Now().Before(m.flashUntil) {
		segs = append(segs, statusSeg{"  " + m.Status, m.statusStyle(), -1})
	}
	segs = append(segs, statusSeg{"Checkpoint: " + shortDuration(m.History.NextIn()) + " [Rev " + itoa(m.History.Len()) + "/99]", m.Th.StatusVal, -1})
	// Key hints are two visually distinct pieces ("^S" and " Save") that must
	// never be split by a separator, so they share a group id.
	for gi, hint := range m.keyHints() {
		parts := strings.SplitN(hint, " ", 2)
		g := gi + 1
		segs = append(segs, statusSeg{parts[0], m.Th.StatusKey, g})
		if len(parts) > 1 {
			segs = append(segs, statusSeg{" " + parts[1], m.Th.StatusVal, g})
		}
	}

	// Lay the segments out group by group. A group is placed only if the whole
	// group fits, so nothing is ever chopped mid-word or left with a separator
	// dangling after it.
	const sepText = " │ "
	x := 2
	limit := w - 1
	first := true
	for i := 0; i < len(segs); {
		j := i + 1
		if segs[i].group >= 0 {
			for j < len(segs) && segs[j].group == segs[i].group {
				j++
			}
		}
		width := 0
		for k := i; k < j; k++ {
			width += render.StringWidth(segs[k].text)
		}
		need := width
		if !first {
			need += render.StringWidth(sepText)
		}
		if x+need > limit {
			return
		}
		if !first {
			c.Draw(x, geo.StatusY, sepText, m.Th.Frame)
			x += render.StringWidth(sepText)
		}
		for k := i; k < j; k++ {
			c.Draw(x, geo.StatusY, segs[k].text, segs[k].style)
			x += render.StringWidth(segs[k].text)
		}
		first = false
		i = j
	}
}

type statusSeg struct {
	text  string
	style render.Style
	// group ties segments that must be placed together; -1 means standalone.
	group int
}

func (m *Model) statusStyle() render.Style {
	if m.StatusWarn {
		return m.Th.StatusWarn
	}
	return m.Th.StatusVal
}

func (m *Model) statusWord() string {
	switch m.Focus {
	case FocusRowHeader:
		return "RESIZE ROW r" + itoa(int(m.Cur.Row)+1)
	case FocusColHeader:
		return "RESIZE COL " + grid.ColName(int(m.Cur.Col))
	}
	return m.Mode.String()
}

func (m *Model) keyHints() []string {
	switch m.Focus {
	case FocusRowHeader, FocusColHeader:
		return []string{"+/- Resize", "Esc Back", "^S Save"}
	}
	switch m.Mode {
	case ModeEdit:
		return []string{"Enter Commit", "Esc Cancel"}
	case ModeConfirm:
		return []string{"y Confirm", "n Cancel"}
	case ModeRollback:
		return []string{"Enter Roll back", "J Custom", "Esc Close"}
	case ModePrompt:
		if m.Prompt == PromptRollback {
			return []string{"Enter Go", "Esc Cancel"}
		}
		return []string{"Enter Confirm", "Esc Cancel"}
	case ModeMenu:
		return []string{"Enter Choose", "Esc Close"}
	}
	return []string{"^S Save", "^Q Quit", "+/- Resize Header"}
}

func (m *Model) selectionSum(r0, c0, r1, c1 int) (string, bool) {
	sh := m.Sheet()
	total := cell.ZeroNumber()
	found := false
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			v := sh.Get(uint32(r), uint32(c)).Value
			if !v.IsNumeric() {
				continue
			}
			found = true
			total = cell.Apply(cell.OpAdd, total, v)
			if total.IsError() {
				return total.Display(), true
			}
		}
	}
	if !found {
		return "", false
	}
	return total.Display(), true
}

// ---------------------------------------------------------------------------
// Shared cell rendering
// ---------------------------------------------------------------------------

func styleFor(v cell.Value, th themeT) render.Style {
	switch v.Kind {
	case cell.KindCurrency:
		return th.Currency
	case cell.KindNumber:
		return th.Number
	case cell.KindError:
		return th.Error
	default:
		return th.Text
	}
}

// formatValue renders a value into exactly the column's interior width.
//
// Values occupy width-1 columns with one trailing blank, which is the steady
// one-character right margin the approved mock shows. Decision D11: text is
// left-aligned and numbers right-aligned.
func formatValue(v cell.Value, width int) string {
	if width <= 0 {
		return ""
	}
	s := v.Display()
	if s == "" {
		return strings.Repeat(" ", width)
	}
	inner := width - 1
	switch {
	case v.IsError():
		return render.PadCenter(s, inner) + " "
	case v.AlignRight():
		if render.StringWidth(s) > inner {
			// Excel shows a row of hashes when a number cannot fit.
			return strings.Repeat("#", width)
		}
		return render.PadLeft(s, inner) + " "
	default:
		return render.PadRight(s, inner) + " "
	}
}

func inRect(row, col, r0, c0, r1, c1 int) bool {
	return row >= r0 && row <= r1 && col >= c0 && col <= c1
}

func sameRef(a, b grid.Ref) bool { return a == b }

func shortDuration(d time.Duration) string {
	if d <= 0 {
		return "due"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return itoa(int(d.Seconds())) + "s"
	}
	return itoa(int(d.Minutes())) + "m " + itoa(int(d.Seconds())%60) + "s"
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
