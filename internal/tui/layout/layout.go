// Package layout computes where every line of the frame goes.
//
// The measurements here are the approved ones from PHASE-1-SPEC.md §10. The
// renderer never hard-codes an x or y coordinate; it asks the geometry.
package layout

// Fixed region measurements.
const (
	// MinTermW and MinTermH are the smallest terminal the interface will draw
	// in; below this the program shows a "terminal too small" panel instead of
	// a garbled screen.
	MinTermW = 60
	MinTermH = 16

	// MinGutterW must fit "r1000000" and the "(height: 99)" marker.
	MinGutterW = 13

	// MinRefW must fit "C1" and "AA100".
	MinRefW = 6

	DefaultColW = 18

	// MaxVisibleCols is a safety rail for the column loop; no terminal shows
	// anything close to this many.
	MaxVisibleCols = 512

	// minClippedCol is the narrowest sliver of a following column worth
	// drawing: a border plus one character. It exists only so the border is
	// never drawn with nothing after it.
	minClippedCol = 1

	// RowOverhead is the number of screen rows taken by everything that is not
	// the grid: menu bar, rules, formula bar, headers, tabs and status bar.
	RowOverhead = 10
)

// Geometry is the single source of truth for every vertical line and horizontal
// band in one frame.
type Geometry struct {
	W, H int

	GutterW int // interior width of the row-number gutter
	RefW    int // width of the reference box
	GridTop int // first y of the grid area

	TabsY   int // the sheet tab strip
	StatusY int // the status bar
	BottomY int // the closing frame

	ColLeft   []int // left border x of each visible column
	ColWidth  []int // interior width of each visible column
	XSepFirst int   // border between the gutter and column A

	// FirstCol is the sheet column index that ColLeft[0] refers to. The
	// renderer reads it rather than repeating the scroll offset, so the two
	// cannot drift apart.
	FirstCol int
}

// Visible reports how many columns are on screen.
func (g Geometry) Visible() int { return len(g.ColLeft) }

// RightEdge is the x of the final border of a visible column.
func (g Geometry) RightEdge(i int) int {
	return g.ColLeft[i] + g.ColWidth[i] + 1
}

// Cols describes the columns a geometry should lay out.
type Cols struct {
	// Left is the leftmost sheet column on screen.
	Left int

	// Width gives a column's interior width. nil means the default for all.
	Width func(col int) int

	// Sized reports whether a column's width was set explicitly. An explicitly
	// sized column keeps its width even when it is the last one on screen; an
	// unsized one stretches to meet the frame, as the approved mock shows.
	//
	// Without this distinction, widening the rightmost visible column had no
	// visible effect at all: the stretch simply gave back exactly what the
	// resize added.
	Sized func(col int) bool
}

// Compute builds the geometry for a terminal size.
//
// Both the renderer and the scroller derive from this one function, which is
// the point: when the geometry assumed a fixed width and the model did not, a
// resized column kept its old width on screen even though the value had been
// stored correctly.
func Compute(w, h int, c Cols) Geometry {
	g := Geometry{
		W: w, H: h,
		GutterW: MinGutterW,
		RefW:    MinRefW,
		GridTop: 6,
		TabsY:   h - 4,
		StatusY: h - 2,
		BottomY: h - 1,
	}
	g.XSepFirst = 1 + g.GutterW
	leftCol := c.Left
	if leftCol < 0 {
		leftCol = 0
	}
	g.FirstCol = leftCol

	width := func(col int) int {
		if c.Width == nil {
			return DefaultColW
		}
		if v := c.Width(col); v > 0 {
			return v
		}
		return DefaultColW
	}

	avail := w - 1 - g.XSepFirst
	if avail < 2 {
		avail = 2
	}

	// Fit as many whole columns as the space allows, using each column's real
	// width rather than assuming they are all the same.
	n, used := 0, 0
	for n < MaxVisibleCols {
		need := width(leftCol+n) + 1
		if used+need > avail {
			break
		}
		used += need
		n++
	}
	if n == 0 {
		// Not even one column fits: show a single clipped one rather than an
		// empty grid.
		g.ColWidth = []int{max(1, avail-1)}
	} else {
		g.ColWidth = make([]int, n)
		for i := range g.ColWidth {
			g.ColWidth[i] = width(leftCol + i)
		}
		rem := avail - used
		last := leftCol + n - 1
		explicit := c.Sized != nil && c.Sized(last)
		switch {
		case rem <= 0:
			// Everything is used exactly.
		case explicit && rem-1 >= minClippedCol:
			// The user chose this column's width, so honour it and start the
			// next column, clipped, instead of quietly widening theirs.
			g.ColWidth = append(g.ColWidth, rem-1)
		default:
			// The last, partly filled column stretches to meet the right-hand
			// frame, exactly as the approved mock does.
			g.ColWidth[n-1] += rem
		}
	}

	g.ColLeft = make([]int, len(g.ColWidth))
	x := g.XSepFirst
	for i := range g.ColLeft {
		g.ColLeft[i] = x
		x += g.ColWidth[i] + 1
	}
	return g
}

// TooSmall reports whether the terminal cannot hold the interface.
func TooSmall(w, h int) bool { return w < MinTermW || h < MinTermH }

// GridLines is how many screen rows are available to the grid.
func (g Geometry) GridLines() int {
	n := g.TabsY - g.GridTop
	if n < 0 {
		return 0
	}
	return n
}

// PlanRow is one grid row scheduled for drawing.
type PlanRow struct {
	Row     int
	Lines   int // screen lines actually drawn
	RealHgt int // the row's true height, for the "(height: n)" marker
}

// TextLine is the screen line inside a row of the given height where the cell's
// text is drawn: the middle line, or the lower of the two middle lines when the
// height is even.
//
//	height 1 -> line 0
//	height 2 -> line 1
//	height 3 -> line 1
//	height 4 -> line 2
//
// This is also what stops a tall row from repeating its text once per line.
func TextLine(height int) int {
	if height <= 1 {
		return 0
	}
	return height / 2
}

// PlanRows chooses which rows are visible and how many lines each gets.
//
// Every row consumes its height in lines plus one separator, and the very last
// separator drawn is the closing rule. Choosing the set greedily and then
// absorbing a single spare line into the final row guarantees the closing rule
// lands exactly on the bottom line of the grid area: no doubled rules, no
// ragged edge. This is the bug the Phase 1 prototype had, fixed here.
func (g Geometry) PlanRows(topRow, rowCount int, height func(int) int) []PlanRow {
	area := g.GridLines()
	if area < 2 || rowCount <= 0 {
		return nil
	}
	var plan []PlanRow
	consumed := 0
	for r := topRow; r < rowCount; r++ {
		h := height(r)
		if h < 1 {
			h = 1
		}
		if consumed+h+1 > area {
			break
		}
		plan = append(plan, PlanRow{Row: r, Lines: h, RealHgt: h})
		consumed += h + 1
	}
	if len(plan) == 0 {
		return nil
	}
	if area-consumed == 1 {
		plan[len(plan)-1].Lines++
	}
	return plan
}

// CursorScreenY returns the screen y of the top line of a row, given the plan.
// ok is false when the row is not visible.
func CursorScreenY(plan []PlanRow, row int) (int, bool) {
	y := 0
	for _, p := range plan {
		if p.Row == row {
			return y, true
		}
		y += p.Lines + 1
	}
	return 0, false
}
