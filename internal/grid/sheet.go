// Package grid holds the workbook model: sheets, sparse cells, row and column
// metadata, and the naming and sizing rules from PHASE-1-SPEC.md §6.
package grid

import (
	"strings"

	"github.com/badvibecoder/cellsheet/internal/cell"
)

// Limits and defaults. These match PHASE-1-SPEC.md §6.2.
const (
	MaxRows = 1_048_576
	MaxCols = 16_384

	DefaultRows = 100
	DefaultCols = 26

	DefaultRowHeight = 1
	DefaultColWidth  = 18

	MinRowHeight = 1
	MaxRowHeight = 20
	MinColWidth  = 4
	MaxColWidth  = 80

	// Growth blocks: overflowing the grid adds another block (PHASE-1-SPEC.md §6.3).
	GrowRows = 100
	GrowCols = 26
)

// Ref identifies a cell within one sheet.
type Ref struct {
	Row uint32
	Col uint32
}

// ColName converts a zero-based column index into spreadsheet letters:
// 0 -> A, 25 -> Z, 26 -> AA, 51 -> AZ, 52 -> BA, 701 -> ZZ, 702 -> AAA.
//
// This is bijective base-26, not ordinary base-26: there is no zero digit, which
// is why the Z -> AA and ZZ -> AAA boundaries need care.
func ColName(i int) string {
	if i < 0 {
		return ""
	}
	var b [8]byte
	n := len(b)
	for {
		n--
		b[n] = byte('A' + i%26)
		i = i/26 - 1
		if i < 0 {
			break
		}
	}
	return string(b[n:])
}

// ParseColName is the inverse of ColName. "A" -> 0, "Z" -> 25, "AA" -> 26.
func ParseColName(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c < 'A' || c > 'Z' {
			return 0, false
		}
		n = n*26 + int(c-'A') + 1
		if n > 1_000_000_000 {
			return 0, false // sanity bound, not the sheet limit
		}
	}
	return n - 1, true
}

// RefName formats a zero-based cell reference: (0,0) -> "A1".
func RefName(r, c uint32) string {
	return ColName(int(c)) + uitoa(int(r)+1)
}

// ParseRef reads an A1-style reference, ignoring case: "b3" -> (2, 1).
func ParseRef(s string) (Ref, bool) {
	i := 0
	for i < len(s) && (s[i] == '$' || s[i] == ' ') {
		i++
	}
	j := i
	for j < len(s) && isLetter(s[j]) {
		j++
	}
	if j == i {
		return Ref{}, false
	}
	col, ok := ParseColName(s[i:j])
	if !ok || col >= MaxCols {
		return Ref{}, false
	}
	for j < len(s) && s[j] == '$' {
		j++
	}
	k := j
	for k < len(s) && s[k] >= '0' && s[k] <= '9' {
		k++
	}
	if k == j || k != len(s) {
		return Ref{}, false
	}
	row, ok := atoi(s[j:k])
	if !ok {
		return Ref{}, false
	}
	row--
	if row < 0 || row >= MaxRows {
		return Ref{}, false
	}
	return Ref{Row: uint32(row), Col: uint32(col)}, true
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
		if n > 10_000_000 {
			return 0, false
		}
	}
	return n, true
}

func uitoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Sheet is one grid. Cells are stored sparsely: a 100x26 sheet with eight
// populated cells stores eight cells, not 2,600.
type Sheet struct {
	ID   uint32
	Name string

	rowN, colN       uint32
	defRowH, defColW int

	cells map[Ref]cell.Cell
	rows  map[uint32]int // only rows that differ from the default height
	cols  map[uint32]int // only columns that differ from the default width
}

// NewSheet creates an empty sheet at the default size.
func NewSheet(id uint32, name string) *Sheet {
	return &Sheet{
		ID:      id,
		Name:    name,
		rowN:    DefaultRows,
		colN:    DefaultCols,
		defRowH: DefaultRowHeight,
		defColW: DefaultColWidth,
		cells:   make(map[Ref]cell.Cell),
		rows:    make(map[uint32]int),
		cols:    make(map[uint32]int),
	}
}

// Rows and Cols report the current sheet size, which grows as data is pasted.
func (s *Sheet) Rows() uint32 { return s.rowN }
func (s *Sheet) Cols() uint32 { return s.colN }

// Count is how many cells hold content.
func (s *Sheet) Count() int { return len(s.cells) }

// DefaultRowHeight and DefaultColWidth report the sheet-wide defaults.
func (s *Sheet) DefaultRowHeight() int { return s.defRowH }
func (s *Sheet) DefaultColWidth() int  { return s.defColW }

// Get returns the cell at (r, c); the zero Cell means empty.
func (s *Sheet) Get(r, c uint32) cell.Cell { return s.cells[Ref{r, c}] }

// GetRef is Get by reference.
func (s *Sheet) GetRef(ref Ref) cell.Cell { return s.cells[ref] }

// Has reports whether a cell holds anything.
func (s *Sheet) Has(r, c uint32) bool {
	_, ok := s.cells[Ref{r, c}]
	return ok
}

// Set writes a cell. Writing an empty cell removes it, so clearing a cell never
// leaves a tombstone behind.
func (s *Sheet) Set(r, c uint32, cl cell.Cell) {
	if cl.Value.IsEmpty() && cl.Source == "" {
		delete(s.cells, Ref{r, c})
		return
	}
	if s.cells == nil {
		s.cells = make(map[Ref]cell.Cell)
	}
	s.cells[Ref{r, c}] = cl
}

// Clear removes a cell.
func (s *Sheet) Clear(r, c uint32) { delete(s.cells, Ref{r, c}) }

// Each visits every populated cell, in an unspecified order. Returning false
// from fn stops the walk.
func (s *Sheet) Each(fn func(Ref, cell.Cell) bool) {
	for ref, cl := range s.cells {
		if !fn(ref, cl) {
			return
		}
	}
}

// RowHeight returns the height of a row in lines.
func (s *Sheet) RowHeight(r uint32) int {
	if h, ok := s.rows[r]; ok {
		return h
	}
	return s.defRowH
}

// SetRowHeight changes one row's height. Out-of-range values are refused
// without error, so a held-down key cannot destroy the layout.
func (s *Sheet) SetRowHeight(r uint32, h int) bool {
	if h < MinRowHeight || h > MaxRowHeight {
		return false
	}
	if h == s.defRowH {
		delete(s.rows, r)
		return true
	}
	if s.rows == nil {
		s.rows = make(map[uint32]int)
	}
	s.rows[r] = h
	return true
}

// ColWidth returns the width of a column in characters.
func (s *Sheet) ColWidth(c uint32) int {
	if w, ok := s.cols[c]; ok {
		return w
	}
	return s.defColW
}

// SetColWidth changes one column's width. Out-of-range values are refused.
func (s *Sheet) SetColWidth(c uint32, w int) bool {
	if w < MinColWidth || w > MaxColWidth {
		return false
	}
	if w == s.defColW {
		delete(s.cols, c)
		return true
	}
	if s.cols == nil {
		s.cols = make(map[uint32]int)
	}
	s.cols[c] = w
	return true
}

// GrowToFit enlarges the sheet so that (r, c) is inside it, adding whole blocks
// as described in PHASE-1-SPEC.md §6.3. Growth happens once per paste, from the
// shape of the paste, not once per cell.
func (s *Sheet) GrowToFit(r, c uint32) {
	if r >= s.rowN {
		s.rowN = growBlock(s.rowN, r, GrowRows, MaxRows)
	}
	if c >= s.colN {
		s.colN = growBlock(s.colN, c, GrowCols, MaxCols)
	}
}

func growBlock(current, need, block, max uint32) uint32 {
	if need < current {
		return current
	}
	want := current
	for want <= need {
		if want > max-block {
			return max
		}
		want += block
	}
	return want
}

// RowsSet returns the rows with a non-default height. The caller must not
// modify the returned map.
func (s *Sheet) RowsSet() map[uint32]int { return s.rows }

// ColsSet returns the columns with a non-default width.
func (s *Sheet) ColsSet() map[uint32]int { return s.cols }

// SetSize forces the sheet to an exact size, used when loading a file.
func (s *Sheet) SetSize(rows, cols uint32) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	if rows > MaxRows {
		rows = MaxRows
	}
	if cols > MaxCols {
		cols = MaxCols
	}
	s.rowN, s.colN = rows, cols
}

// SetDefaults overrides the sheet-wide default height and width, used when
// loading a file.
func (s *Sheet) SetDefaults(h, w int) {
	if h >= MinRowHeight && h <= MaxRowHeight {
		s.defRowH = h
	}
	if w >= MinColWidth && w <= MaxColWidth {
		s.defColW = w
	}
}

// SetRowHeightRaw and SetColWidthRaw restore metadata verbatim during load,
// bypassing the interactive range checks.
func (s *Sheet) SetRowHeightRaw(r uint32, h int) {
	if s.rows == nil {
		s.rows = make(map[uint32]int)
	}
	s.rows[r] = h
}

// SetColWidthRaw restores a column width verbatim during load.
func (s *Sheet) SetColWidthRaw(c uint32, w int) {
	if s.cols == nil {
		s.cols = make(map[uint32]int)
	}
	s.cols[c] = w
}

// SheetNames is a small helper for error messages and menus.
func SheetNames(sheets []*Sheet) []string {
	out := make([]string, 0, len(sheets))
	for _, s := range sheets {
		out = append(out, s.Name)
	}
	return out
}

// NormalizeName trims a sheet name and rejects the empty result.
func NormalizeName(name string) (string, bool) {
	t := strings.TrimSpace(name)
	if t == "" {
		return "", false
	}
	return t, true
}
