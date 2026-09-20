package grid

import (
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
)

// TestColNameBoundaries covers the places where bijective base-26 differs from
// ordinary base-26, which is where naive implementations break.
func TestColNameBoundaries(t *testing.T) {
	cases := []struct {
		i    int
		want string
	}{
		{0, "A"},
		{1, "B"},
		{25, "Z"},
		{26, "AA"},
		{27, "AB"},
		{51, "AZ"},
		{52, "BA"},
		{53, "BB"},
		{77, "BZ"},
		{78, "CA"},
		{701, "ZZ"},
		{702, "AAA"},
		{703, "AAB"},
		{1377, "AZZ"},
		{18277, "ZZZ"},
		{18278, "AAAA"},
		{16383, "XFD"}, // Excel's last column
	}
	for _, c := range cases {
		if got := ColName(c.i); got != c.want {
			t.Errorf("ColName(%d) = %q, want %q", c.i, got, c.want)
		}
		back, ok := ParseColName(c.want)
		if !ok || back != c.i {
			t.Errorf("ParseColName(%q) = %d,%v; want %d,true", c.want, back, ok, c.i)
		}
	}
}

func TestParseRef(t *testing.T) {
	cases := []struct {
		in   string
		want Ref
		ok   bool
	}{
		{"A1", Ref{0, 0}, true},
		{"a1", Ref{0, 0}, true},
		{"B3", Ref{2, 1}, true},
		{"AA100", Ref{99, 26}, true},
		{"$C$7", Ref{6, 2}, true},
		{"XFD1048576", Ref{1048575, 16383}, true},
		{"", Ref{}, false},
		{"1A", Ref{}, false},
		{"A", Ref{}, false},
		{"A0", Ref{}, false},
		{"A1B", Ref{}, false},
		{"ZZZ1", Ref{}, false}, // past the 16,384-column limit
	}
	for _, c := range cases {
		got, ok := ParseRef(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseRef(%q) = %v,%v; want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestRefName(t *testing.T) {
	if got := RefName(0, 0); got != "A1" {
		t.Errorf("RefName(0,0) = %q", got)
	}
	if got := RefName(99, 26); got != "AA100" {
		t.Errorf("RefName(99,26) = %q", got)
	}
}

func TestSparseStorage(t *testing.T) {
	s := NewSheet(1, "Sheet1")
	if s.Rows() != 100 || s.Cols() != 26 {
		t.Fatalf("new sheet should be 100x26, got %dx%d", s.Rows(), s.Cols())
	}
	if s.Count() != 0 {
		t.Fatalf("new sheet should be empty, got %d cells", s.Count())
	}
	s.Set(0, 0, cell.Cell{Value: cell.Currency(cell.NewDec(50000, 0))})
	s.Set(3, 2, cell.Cell{Value: cell.Number(cell.FromInt64(450))})
	if s.Count() != 2 {
		t.Fatalf("count = %d, want 2", s.Count())
	}
	if !s.Has(0, 0) || s.Has(1, 1) {
		t.Error("Has is wrong")
	}
	if got := s.Get(3, 2).Value.Display(); got != "450" {
		t.Errorf("Get(3,2) = %q", got)
	}
	// Setting an empty cell removes it rather than leaving a tombstone.
	s.Set(0, 0, cell.Cell{})
	if s.Has(0, 0) || s.Count() != 1 {
		t.Errorf("clearing left a tombstone: count=%d", s.Count())
	}
	// Clear is explicit.
	s.Clear(3, 2)
	if s.Count() != 0 {
		t.Errorf("Clear did not remove the cell")
	}
	// Writing 2,600 cells is fine and cheap; the map only holds what exists.
	for r := uint32(0); r < 100; r++ {
		for c := uint32(0); c < 26; c++ {
			s.Set(r, c, cell.Cell{Value: cell.Number(cell.FromInt64(int64(r * c)))})
		}
	}
	if s.Count() != 2600 {
		t.Errorf("count = %d, want 2600", s.Count())
	}
}

func TestGrowToFit(t *testing.T) {
	cases := []struct {
		r, c         uint32
		wantR, wantC uint32
		why          string
	}{
		{99, 25, 100, 26, "inside the default grid: no growth"},
		{100, 25, 200, 26, "one past the last row adds 100 rows"},
		{199, 25, 200, 26, "still inside the new block"},
		{200, 25, 300, 26, "one past again adds another block"},
		{5, 26, 100, 52, "one past the last column adds 26 columns"},
		{100, 26, 200, 52, "both dimensions grow together"},
		{0, 51, 100, 52, "index 51 is the last cell of the second block"},
	}
	for _, c := range cases {
		s := NewSheet(1, "S")
		s.GrowToFit(c.r, c.c)
		if s.Rows() != c.wantR || s.Cols() != c.wantC {
			t.Errorf("GrowToFit(%d,%d) -> %dx%d, want %dx%d (%s)",
				c.r, c.c, s.Rows(), s.Cols(), c.wantR, c.wantC, c.why)
		}
	}
}

func TestResizeBounds(t *testing.T) {
	s := NewSheet(1, "S")
	if s.RowHeight(5) != DefaultRowHeight || s.ColWidth(5) != DefaultColWidth {
		t.Fatal("defaults are wrong")
	}
	if !s.SetRowHeight(5, 3) || s.RowHeight(5) != 3 {
		t.Error("setting a valid row height failed")
	}
	if s.SetRowHeight(5, MaxRowHeight+1) || s.RowHeight(5) != 3 {
		t.Error("an out-of-range row height must be refused without changing anything")
	}
	if s.SetRowHeight(5, MinRowHeight-1) {
		t.Error("zero-height rows must be refused")
	}
	if !s.SetColWidth(2, 40) || s.ColWidth(2) != 40 {
		t.Error("setting a valid column width failed")
	}
	if s.SetColWidth(2, MaxColWidth+1) || s.ColWidth(2) != 40 {
		t.Error("an out-of-range column width must be refused")
	}
	// Returning to the default clears the override rather than storing it.
	s.SetRowHeight(5, DefaultRowHeight)
	if _, ok := s.RowsSet()[5]; ok {
		t.Error("a default height should not be stored as an override")
	}
	// Other rows and columns are untouched.
	if s.RowHeight(6) != DefaultRowHeight || s.ColWidth(3) != DefaultColWidth {
		t.Error("resizing leaked to a neighbouring row or column")
	}
}

func TestWorkbookLifecycle(t *testing.T) {
	w := NewWorkbook()
	if w.Len() != 1 || w.Active().Name != "Sheet1" {
		t.Fatal("a new workbook should have one sheet named Sheet1")
	}
	s2, err := w.AddSheet("Sheet2")
	if err != nil {
		t.Fatalf("AddSheet: %v", err)
	}
	if w.Len() != 2 || s2.ID == w.Sheets()[0].ID {
		t.Error("sheet IDs must be distinct")
	}
	if _, err := w.AddSheet("sheet2"); err != ErrNameTaken {
		t.Errorf("duplicate names must be refused case-insensitively, got %v", err)
	}
	if _, err := w.AddSheet("   "); err != ErrNameEmpty {
		t.Errorf("empty names must be refused, got %v", err)
	}
	if got := w.NextSheetName(); got != "Sheet3" {
		t.Errorf("NextSheetName = %q, want Sheet3", got)
	}
	// Lookup is case-insensitive.
	if s, ok := w.SheetByName("SHEET2"); !ok || s != s2 {
		t.Error("SheetByName should be case-insensitive")
	}
	if !w.SetActive(1) || w.Active() != s2 {
		t.Error("SetActive failed")
	}
	if w.SetActive(9) {
		t.Error("SetActive must reject an out-of-range index")
	}
	if err := w.RenameSheet(1, "Q3 Data"); err != nil {
		t.Fatalf("RenameSheet: %v", err)
	}
	if w.Sheets()[1].Name != "Q3 Data" {
		t.Error("rename did not take effect")
	}
	if _, ok := w.SheetByName("Sheet2"); ok {
		t.Error("the old name should no longer resolve")
	}
	if _, ok := w.SheetByName("q3 data"); !ok {
		t.Error("the new name should resolve case-insensitively")
	}
	// Deleting down to one sheet is allowed; deleting the last is not.
	if err := w.DeleteSheet(1); err != nil {
		t.Fatalf("DeleteSheet: %v", err)
	}
	if err := w.DeleteSheet(0); err != ErrLastSheet {
		t.Errorf("deleting the last sheet must be refused, got %v", err)
	}
}

func TestWorkbookSetCellGrows(t *testing.T) {
	w := NewWorkbook()
	s := w.Active()
	if !w.SetCell(s, 250, 30, cell.Cell{Value: cell.Text("far")}) {
		t.Fatal("SetCell failed")
	}
	if s.Rows() != 300 || s.Cols() != 52 {
		t.Errorf("SetCell should have grown the sheet, got %dx%d", s.Rows(), s.Cols())
	}
	if w.SetCell(s, MaxRows, 0, cell.Cell{Value: cell.Text("past the end")}) {
		t.Error("SetCell should refuse a row past the limit")
	}
}
