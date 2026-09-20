// Package render is the frame compositor.
//
// A frame is built as a two-dimensional grid of styled runes and only encoded
// to ANSI at the very end. That is what allows a menu or dialog to be drawn
// *over* the grid, hiding what is behind it, and it is why the program does not
// simply diff lines of text.
//
// This package descends directly from the Phase 1 look prototype
// (prototype/look/canvas.go), which is why the approved appearance and the
// shipped appearance are the same by construction rather than by effort.
package render

import "strings"

// Color is a terminal colour. The zero value is ColDefault, meaning "use the
// terminal's own colour". 0-7 are the basic ANSI colours, 8-15 the bright
// variants, 16-255 the 256-colour cube, and anything at or above 0x01000000 is
// a 24-bit RGB value.
type Color int32

const (
	ColDefault Color = -1

	ColBlack   Color = 0
	ColRed     Color = 1
	ColGreen   Color = 2
	ColYellow  Color = 3
	ColBlue    Color = 4
	ColMagenta Color = 5
	ColCyan    Color = 6
	ColWhite   Color = 7

	ColBrBlack   Color = 8
	ColBrRed     Color = 9
	ColBrGreen   Color = 10
	ColBrYellow  Color = 11
	ColBrBlue    Color = 12
	ColBrMagenta Color = 13
	ColBrCyan    Color = 14
	ColBrWhite   Color = 15
)

// RGB builds a 24-bit colour, downgraded automatically on lesser terminals.
func RGB(r, g, b uint8) Color {
	return Color(0x01000000 | int32(r)<<16 | int32(g)<<8 | int32(b))
}

// Attr is a bit set of text attributes.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrFaint
	AttrItalic
	AttrUnderline
	AttrReverse
)

// Style is everything needed to paint one cell of the screen.
type Style struct {
	Fg   Color
	Bg   Color
	Attr Attr
}

// S is a short constructor: S(ColRed, AttrBold).
func S(fg Color, attr ...Attr) Style {
	var a Attr
	for _, x := range attr {
		a |= x
	}
	return Style{Fg: fg, Bg: ColDefault, Attr: a}
}

// With returns a copy with extra attributes.
func (s Style) With(attr ...Attr) Style {
	for _, x := range attr {
		s.Attr |= x
	}
	return s
}

// On returns a copy with a background colour.
func (s Style) On(bg Color) Style { s.Bg = bg; return s }

// ColorMode is how much colour the terminal can take.
type ColorMode int

const (
	ColorNone ColorMode = iota
	Color16
	Color256
	ColorTrue
)

// ParseColorMode converts a setting name; "auto" picks 256 colours when stdout
// looks like a terminal and no colour otherwise.
func ParseColorMode(s string, isTerminal bool) ColorMode {
	switch strings.ToLower(s) {
	case "none", "off", "no":
		return ColorNone
	case "16":
		return Color16
	case "256":
		return Color256
	case "true", "truecolor", "24":
		return ColorTrue
	default:
		if isTerminal {
			return Color256
		}
		return ColorNone
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func fgCode(c Color, mode ColorMode) string {
	switch {
	case c < 0:
		return ""
	case c < 8:
		return "3" + itoa(int(c))
	case c < 16:
		return "9" + itoa(int(c)-8)
	case c < 256:
		return "38;5;" + itoa(int(c))
	default:
		r, g, b := byte(c>>16), byte(c>>8), byte(c)
		if mode == ColorTrue {
			return "38;2;" + itoa(int(r)) + ";" + itoa(int(g)) + ";" + itoa(int(b))
		}
		return "38;5;" + itoa(int(rgbTo256(r, g, b)))
	}
}

func bgCode(c Color, mode ColorMode) string {
	switch {
	case c < 0:
		return ""
	case c < 8:
		return "4" + itoa(int(c))
	case c < 16:
		return "10" + itoa(int(c)-8)
	case c < 256:
		return "48;5;" + itoa(int(c))
	default:
		r, g, b := byte(c>>16), byte(c>>8), byte(c)
		if mode == ColorTrue {
			return "48;2;" + itoa(int(r)) + ";" + itoa(int(g)) + ";" + itoa(int(b))
		}
		return "48;5;" + itoa(int(rgbTo256(r, g, b)))
	}
}

func rgbTo256(r, g, b uint8) uint8 {
	toCube := func(v uint8) int {
		switch {
		case v < 48:
			return 0
		case v < 115:
			return 1
		default:
			return (int(v) - 35) / 40
		}
	}
	return uint8(16 + 36*toCube(r) + 6*toCube(g) + toCube(b))
}

func sgr(s Style, mode ColorMode) string {
	if mode == ColorNone {
		return ""
	}
	var p []string
	if s.Attr&AttrBold != 0 {
		p = append(p, "1")
	}
	if s.Attr&AttrFaint != 0 {
		p = append(p, "2")
	}
	if s.Attr&AttrItalic != 0 {
		p = append(p, "3")
	}
	if s.Attr&AttrUnderline != 0 {
		p = append(p, "4")
	}
	if s.Attr&AttrReverse != 0 {
		p = append(p, "7")
	}
	if c := fgCode(s.Fg, mode); c != "" {
		p = append(p, c)
	}
	if c := bgCode(s.Bg, mode); c != "" {
		p = append(p, c)
	}
	if len(p) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(p, ";") + "m"
}

// ---------------------------------------------------------------------------
// Character widths
// ---------------------------------------------------------------------------

// RuneWidth reports how many terminal columns a rune occupies. Wide characters
// matter: ignoring them breaks every column to the right of a CJK cell.
func RuneWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r < 32 || (r >= 0x7f && r < 0xa0):
		return 0
	case r == 0x200b || r == 0x200c || r == 0x200d:
		return 0
	case r >= 0x0300 && r <= 0x036f,
		r >= 0x1ab0 && r <= 0x1aff,
		r >= 0x1dc0 && r <= 0x1dff,
		r >= 0x20d0 && r <= 0x20ff,
		r >= 0xfe00 && r <= 0xfe0f,
		r >= 0xfe20 && r <= 0xfe2f:
		return 0
	case isWide(r):
		return 2
	}
	return 1
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe6f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}

// StringWidth is the printed width of a string.
func StringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// Truncate cuts s to width columns, appending an ellipsis when it had to drop
// characters.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if StringWidth(s) <= width {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := RuneWidth(r)
		if w+rw > width-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteRune('…')
	return b.String()
}

// PadRight left-aligns s in width columns.
func PadRight(s string, width int) string { return Fit(s, width, 0) }

// PadLeft right-aligns s in width columns.
func PadLeft(s string, width int) string { return Fit(s, width, 2) }

// PadCenter centres s in width columns.
func PadCenter(s string, width int) string { return Fit(s, width, 1) }

// Fit aligns s in width columns. mode: 0 left, 1 centre, 2 right. Text that is
// too wide is truncated with an ellipsis.
func Fit(s string, width, mode int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	gap := width - StringWidth(s)
	if gap <= 0 {
		return s
	}
	switch mode {
	case 1:
		l := gap / 2
		return strings.Repeat(" ", l) + s + strings.Repeat(" ", gap-l)
	case 2:
		return strings.Repeat(" ", gap) + s
	default:
		return s + strings.Repeat(" ", gap)
	}
}

// ---------------------------------------------------------------------------
// Canvas
// ---------------------------------------------------------------------------

type canvasCell struct {
	r    rune
	s    Style
	skip bool // right half of a wide rune
}

// Canvas is an off-screen frame buffer.
type Canvas struct {
	W, H  int
	cells []canvasCell
}

// NewCanvas allocates a frame buffer filled with spaces.
func NewCanvas(w, h int) *Canvas {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	c := &Canvas{W: w, H: h, cells: make([]canvasCell, w*h)}
	for i := range c.cells {
		c.cells[i] = canvasCell{r: ' ', s: Style{Fg: ColDefault, Bg: ColDefault}}
	}
	return c
}

func (c *Canvas) in(x, y int) bool {
	return x >= 0 && y >= 0 && x < c.W && y < c.H
}

// Set writes one rune. Later writes win, which is what makes overlay
// compositing work.
func (c *Canvas) Set(x, y int, r rune, s Style) {
	if !c.in(x, y) {
		return
	}
	w := RuneWidth(r)
	if w == 0 {
		return
	}
	c.cells[y*c.W+x] = canvasCell{r: r, s: s}
	if w == 2 && c.in(x+1, y) {
		c.cells[y*c.W+x+1] = canvasCell{r: 0, s: s, skip: true}
	}
}

// At reads a cell back.
func (c *Canvas) At(x, y int) (rune, Style) {
	if !c.in(x, y) {
		return ' ', Style{Fg: ColDefault, Bg: ColDefault}
	}
	cl := c.cells[y*c.W+x]
	return cl.r, cl.s
}

// Draw writes a string, honouring double-width runes.
func (c *Canvas) Draw(x, y int, s string, st Style) {
	for _, r := range s {
		c.Set(x, y, r, st)
		x += RuneWidth(r)
	}
}

// Fill paints a rectangle with one rune.
func (c *Canvas) Fill(x, y, w, h int, r rune, st Style) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.Set(xx, yy, r, st)
		}
	}
}

// HRule draws a horizontal rule, substituting runes at the given x positions.
// Explicit junctions, rather than a generic box algorithm, are how the grid
// gets its exact tee and cross characters.
func (c *Canvas) HRule(y, x0, x1 int, h rune, junctions map[int]rune, st Style) {
	for x := x0; x <= x1; x++ {
		r := h
		if j, ok := junctions[x]; ok {
			r = j
		}
		c.Set(x, y, r, st)
	}
}

// VRule draws a vertical rule with optional junctions.
func (c *Canvas) VRule(x, y0, y1 int, v rune, junctions map[int]rune, st Style) {
	for y := y0; y <= y1; y++ {
		r := v
		if j, ok := junctions[y]; ok {
			r = j
		}
		c.Set(x, y, r, st)
	}
}

// DrawBox draws a single-line rectangular frame.
func (c *Canvas) DrawBox(x, y, w, h int, st Style) {
	if w < 2 || h < 2 {
		return
	}
	c.HRule(y, x, x+w-1, '─', nil, st)
	c.HRule(y+h-1, x, x+w-1, '─', nil, st)
	c.VRule(x, y, y+h-1, '│', nil, st)
	c.VRule(x+w-1, y, y+h-1, '│', nil, st)
	c.Set(x, y, '┌', st)
	c.Set(x+w-1, y, '┐', st)
	c.Set(x, y+h-1, '└', st)
	c.Set(x+w-1, y+h-1, '┘', st)
}

// Blit copies src onto c at (ox, oy). The copy is opaque: blank cells in src
// erase whatever was underneath, which is exactly what a floating menu needs so
// that the grid does not show through it.
func (c *Canvas) Blit(src *Canvas, ox, oy int) {
	for y := 0; y < src.H; y++ {
		for x := 0; x < src.W; x++ {
			cl := src.cells[y*src.W+x]
			if cl.skip {
				continue
			}
			c.Set(ox+x, oy+y, cl.r, cl.s)
		}
	}
}

// Lines renders the frame as one string per row, with no escape sequences.
// Used by the golden-file render tests.
func (c *Canvas) Lines() []string {
	out := make([]string, 0, c.H)
	for y := 0; y < c.H; y++ {
		var b strings.Builder
		for x := 0; x < c.W; x++ {
			cl := c.cells[y*c.W+x]
			if cl.skip {
				continue
			}
			b.WriteRune(cl.r)
		}
		out = append(out, b.String())
	}
	return out
}

// String renders the frame, emitting escape sequences only when the style
// changes and always resetting at the end of a line.
//
// There is deliberately NO newline after the final line. A trailing newline
// advances the cursor past the last row of the terminal, which scrolls the
// whole screen up by one and pushes the menu bar off the top — the frame looked
// like it was missing its first line.
func (c *Canvas) String(mode ColorMode) string {
	var b strings.Builder
	b.Grow(c.W*c.H + c.H*8)
	for y := 0; y < c.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		if mode != ColorNone {
			b.WriteString("\x1b[0m")
		}
		cur := Style{Fg: ColDefault, Bg: ColDefault}
		fresh := true
		for x := 0; x < c.W; x++ {
			cl := c.cells[y*c.W+x]
			if cl.skip {
				continue
			}
			if mode != ColorNone && (fresh || cl.s != cur) {
				if seq := sgr(cl.s, mode); seq != "" {
					b.WriteString(seq)
				} else {
					b.WriteString("\x1b[0m")
				}
				cur = cl.s
				fresh = false
			}
			b.WriteRune(cl.r)
		}
		if mode != ColorNone {
			b.WriteString("\x1b[0m")
		}
	}
	return b.String()
}
