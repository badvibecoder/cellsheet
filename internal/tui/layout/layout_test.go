package layout

import "testing"

func TestComputeUsesSuppliedWidths(t *testing.T) {
	// Uniform default.
	g := Compute(120, 32, Cols{})
	if len(g.ColWidth) != 5 {
		t.Fatalf("default layout has %d columns, want 5: %v", len(g.ColWidth), g.ColWidth)
	}
	for i := 0; i < 4; i++ {
		if g.ColWidth[i] != DefaultColW {
			t.Errorf("column %d width = %d, want %d", i, g.ColWidth[i], DefaultColW)
		}
	}

	// A wide first column pushes the others right and reduces how many fit.
	wide := map[int]int{0: 40}
	g2 := Compute(120, 32, Cols{Width: func(c int) int {
		if w, ok := wide[c]; ok {
			return w
		}
		return DefaultColW
	}})
	if g2.ColWidth[0] != 40 {
		t.Errorf("column 0 width = %d, want 40: %v", g2.ColWidth[0], g2.ColWidth)
	}
	if len(g2.ColWidth) >= len(g.ColWidth) {
		t.Errorf("a wider first column should fit fewer columns: %d then %d",
			len(g.ColWidth), len(g2.ColWidth))
	}
	// Borders must tile without gaps or overlaps.
	for i := 1; i < len(g2.ColLeft); i++ {
		if want := g2.ColLeft[i-1] + g2.ColWidth[i-1] + 1; g2.ColLeft[i] != want {
			t.Errorf("column %d starts at %d, want %d", i, g2.ColLeft[i], want)
		}
	}
	if right := g2.ColLeft[len(g2.ColLeft)-1] + g2.ColWidth[len(g2.ColWidth)-1] + 1; right != 120-1 {
		t.Errorf("the last right border is at x=%d, want %d (the frame)", right, 120-1)
	}
}

// TestExplicitlySizedLastColumnKeepsItsWidth is the trap: the last column
// stretches to fill, and the stretch used to give back exactly what a resize
// added, so widening it had no visible effect at all.
func TestExplicitlySizedLastColumnKeepsItsWidth(t *testing.T) {
	sized := map[int]bool{4: true}
	widths := map[int]int{4: 24}
	cols := Cols{
		Width: func(c int) int {
			if w, ok := widths[c]; ok {
				return w
			}
			return DefaultColW
		},
		Sized: func(c int) bool { return sized[c] },
	}
	g := Compute(120, 32, cols)
	if len(g.ColWidth) < 5 {
		t.Fatalf("expected at least five columns, got %v", g.ColWidth)
	}
	if g.ColWidth[4] != 24 {
		t.Errorf("the explicitly sized column draws %d wide, want 24: %v", g.ColWidth[4], g.ColWidth)
	}
	// The space that could not fit another whole column is offered to the next
	// one, clipped.
	if len(g.ColWidth) == 5 {
		t.Error("the leftover space vanished instead of starting the next column")
	}
	right := g.ColLeft[len(g.ColLeft)-1] + g.ColWidth[len(g.ColWidth)-1] + 1
	if right != 120-1 {
		t.Errorf("the last right border is at x=%d, want %d", right, 120-1)
	}

	// An unsized last column still stretches, so the approved look is kept.
	g2 := Compute(120, 32, Cols{})
	if g2.ColWidth[len(g2.ColWidth)-1] <= DefaultColW {
		t.Errorf("an unsized last column should stretch, got %v", g2.ColWidth)
	}
}

func TestComputeHandlesAHugeColumn(t *testing.T) {
	g := Compute(120, 32, Cols{Width: func(int) int { return 400 }})
	if len(g.ColWidth) != 1 {
		t.Fatalf("expected a single clipped column, got %v", g.ColWidth)
	}
	// The clipped column fills everything between the gutter and the frame.
	if g.ColWidth[0] != g.W-1-g.XSepFirst-1 {
		t.Errorf("the clipped column draws %d, want %d", g.ColWidth[0], g.W-1-g.XSepFirst-1)
	}
	if right := g.ColLeft[0] + g.ColWidth[0] + 1; right != 120-1 {
		t.Errorf("the clipped column's right border is at x=%d, want %d", right, 120-1)
	}
}

func TestComputeScrollsFromTheGivenColumn(t *testing.T) {
	widths := map[int]int{7: 30}
	cols := Cols{
		Left: 5,
		Width: func(c int) int {
			if w, ok := widths[c]; ok {
				return w
			}
			return DefaultColW
		},
	}
	g := Compute(120, 32, cols)
	if g.FirstCol != 5 {
		t.Errorf("FirstCol = %d, want 5", g.FirstCol)
	}
	// Column 7 is the third visible one and must be the wide one.
	idx := 7 - g.FirstCol
	if idx < 0 || idx >= len(g.ColWidth) {
		t.Fatalf("column 7 is not visible: %v", g.ColWidth)
	}
	if g.ColWidth[idx] != 30 {
		t.Errorf("column 7 draws %d wide, want 30: %v", g.ColWidth[idx], g.ColWidth)
	}
}

func TestComputeRejectsNegativeScroll(t *testing.T) {
	g := Compute(120, 32, Cols{Left: -5})
	if g.FirstCol != 0 {
		t.Errorf("FirstCol = %d, want 0", g.FirstCol)
	}
}

func TestPlanRowsAbsorbsASpareLine(t *testing.T) {
	g := Compute(120, 32, Cols{})
	height := func(int) int { return 1 }
	plan := g.PlanRows(0, 1000, height)
	if len(plan) == 0 {
		t.Fatal("no rows planned")
	}
	consumed := 0
	for _, p := range plan {
		consumed += p.Lines + 1
	}
	if consumed != g.GridLines() {
		t.Errorf("the plan consumes %d lines, want exactly %d", consumed, g.GridLines())
	}
}

func TestTooSmall(t *testing.T) {
	if !TooSmall(40, 10) || !TooSmall(120, 10) || !TooSmall(40, 32) {
		t.Error("small terminals should be reported as too small")
	}
	if TooSmall(MinTermW, MinTermH) || TooSmall(300, 100) {
		t.Error("the minimum size and anything larger are usable")
	}
}

// TestTextLine is the vertical placement rule for cell text: the middle line,
// or the lower of the two middle lines when the height is even.
func TestTextLine(t *testing.T) {
	cases := []struct{ height, want int }{
		{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 2}, {5, 2}, {6, 3}, {7, 3}, {20, 10},
	}
	for _, c := range cases {
		if got := TextLine(c.height); got != c.want {
			t.Errorf("TextLine(%d) = %d, want %d", c.height, got, c.want)
		}
	}
	// The placement must always be inside the row.
	for h := 1; h <= 40; h++ {
		if got := TextLine(h); got < 0 || got >= h {
			t.Errorf("TextLine(%d) = %d, which is outside the row", h, got)
		}
	}
}
