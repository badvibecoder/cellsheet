package grid

import (
	"errors"
	"strings"

	"github.com/badvibecoder/cellsheet/internal/cell"
)

// Workbook errors.
var (
	ErrNoSuchSheet = errors.New("no such sheet")
	ErrNameTaken   = errors.New("a sheet with that name already exists")
	ErrNameEmpty   = errors.New("a sheet name cannot be empty")
	ErrLastSheet   = errors.New("the last sheet cannot be deleted")
)

// Workbook is one .cell file: a list of sheets plus which one is showing.
type Workbook struct {
	sheets []*Sheet
	index  map[string]*Sheet // lower-cased name -> sheet
	active int
	nextID uint32
}

// NewWorkbook creates a workbook with a single sheet named "Sheet1".
func NewWorkbook() *Workbook {
	w := &Workbook{index: make(map[string]*Sheet), nextID: 1}
	w.sheets = append(w.sheets, NewSheet(w.nextID, "Sheet1"))
	w.nextID++
	w.index["sheet1"] = w.sheets[0]
	return w
}

// Sheets returns the sheets in tab order.
func (w *Workbook) Sheets() []*Sheet { return w.sheets }

// Len is how many sheets exist.
func (w *Workbook) Len() int { return len(w.sheets) }

// ActiveIndex is the tab that is showing.
func (w *Workbook) ActiveIndex() int { return w.active }

// Active returns the sheet that is showing, or nil for an empty workbook.
func (w *Workbook) Active() *Sheet {
	if w.active < 0 || w.active >= len(w.sheets) {
		return nil
	}
	return w.sheets[w.active]
}

// SetActive switches tabs, reporting false for an out-of-range index.
func (w *Workbook) SetActive(i int) bool {
	if i < 0 || i >= len(w.sheets) {
		return false
	}
	w.active = i
	return true
}

// SheetByName finds a sheet case-insensitively, which is how formula references
// resolve (PHASE-1-SPEC.md §18.2).
func (w *Workbook) SheetByName(name string) (*Sheet, bool) {
	s, ok := w.index[strings.ToLower(strings.TrimSpace(name))]
	return s, ok
}

// SheetByID finds a sheet by its stable identifier.
func (w *Workbook) SheetByID(id uint32) (*Sheet, bool) {
	for _, s := range w.sheets {
		if s.ID == id {
			return s, true
		}
	}
	return nil, false
}

// IndexOf returns a sheet's tab position.
func (w *Workbook) IndexOf(s *Sheet) int {
	for i, x := range w.sheets {
		if x == s {
			return i
		}
	}
	return -1
}

// AddSheet appends a new sheet with the given name and returns it.
func (w *Workbook) AddSheet(name string) (*Sheet, error) {
	n, ok := NormalizeName(name)
	if !ok {
		return nil, ErrNameEmpty
	}
	if _, exists := w.index[strings.ToLower(n)]; exists {
		return nil, ErrNameTaken
	}
	s := NewSheet(w.nextID, n)
	w.nextID++
	w.sheets = append(w.sheets, s)
	w.index[strings.ToLower(n)] = s
	return s, nil
}

// NextSheetName suggests an unused default name such as "Sheet3".
func (w *Workbook) NextSheetName() string {
	for i := 1; ; i++ {
		name := "Sheet" + uitoa(i)
		if _, exists := w.index[strings.ToLower(name)]; !exists {
			return name
		}
	}
}

// RenameSheet changes a sheet's display name. Cross-sheet formula rewriting is
// deliberately NOT done here: it belongs to the layer that owns formulas, so
// that a rename is one undoable operation (PHASE-1-SPEC.md §18.2).
func (w *Workbook) RenameSheet(i int, name string) error {
	if i < 0 || i >= len(w.sheets) {
		return ErrNoSuchSheet
	}
	n, ok := NormalizeName(name)
	if !ok {
		return ErrNameEmpty
	}
	key := strings.ToLower(n)
	if cur, exists := w.index[key]; exists && cur != w.sheets[i] {
		return ErrNameTaken
	}
	delete(w.index, strings.ToLower(w.sheets[i].Name))
	w.sheets[i].Name = n
	w.index[key] = w.sheets[i]
	return nil
}

// DeleteSheet removes a sheet, keeping at least one.
func (w *Workbook) DeleteSheet(i int) error {
	if i < 0 || i >= len(w.sheets) {
		return ErrNoSuchSheet
	}
	if len(w.sheets) == 1 {
		return ErrLastSheet
	}
	delete(w.index, strings.ToLower(w.sheets[i].Name))
	w.sheets = append(w.sheets[:i], w.sheets[i+1:]...)
	if w.active >= len(w.sheets) {
		w.active = len(w.sheets) - 1
	}
	return nil
}

// MoveSheet reorders tabs.
func (w *Workbook) MoveSheet(from, to int) bool {
	if from < 0 || from >= len(w.sheets) || to < 0 || to >= len(w.sheets) || from == to {
		return false
	}
	s := w.sheets[from]
	w.sheets = append(w.sheets[:from], w.sheets[from+1:]...)
	rest := append([]*Sheet{s}, w.sheets[to:]...)
	w.sheets = append(w.sheets[:to], rest...)
	w.active = to
	return true
}

// CellCount totals populated cells across the workbook.
func (w *Workbook) CellCount() int {
	n := 0
	for _, s := range w.sheets {
		n += s.Count()
	}
	return n
}

// SetCell is a convenience for tests and paste paths: it writes a value and
// grows the sheet if needed.
func (w *Workbook) SetCell(s *Sheet, r, c uint32, cl cell.Cell) bool {
	if r >= MaxRows || c >= MaxCols {
		return false
	}
	s.GrowToFit(r, c)
	s.Set(r, c, cl)
	return true
}

// PutSheet inserts a sheet at the given index, or replaces the sheet with the
// same ID wherever it currently is. It is used when replaying history, where
// both "a sheet appeared" and "a sheet's whole content changed" must be
// expressible.
func (w *Workbook) PutSheet(index int, sh *Sheet) {
	if existing, ok := w.SheetByID(sh.ID); ok {
		i := w.IndexOf(existing)
		delete(w.index, strings.ToLower(existing.Name))
		w.sheets[i] = sh
		w.index[strings.ToLower(sh.Name)] = sh
		return
	}
	if index < 0 {
		index = 0
	}
	if index > len(w.sheets) {
		index = len(w.sheets)
	}
	w.sheets = append(w.sheets, nil)
	copy(w.sheets[index+1:], w.sheets[index:])
	w.sheets[index] = sh
	w.index[strings.ToLower(sh.Name)] = sh
	if sh.ID >= w.nextID {
		w.nextID = sh.ID + 1
	}
}

// RemoveSheetAt removes the sheet at an index and returns it.
func (w *Workbook) RemoveSheetAt(index int) *Sheet {
	if index < 0 || index >= len(w.sheets) {
		return nil
	}
	sh := w.sheets[index]
	delete(w.index, strings.ToLower(sh.Name))
	w.sheets = append(w.sheets[:index], w.sheets[index+1:]...)
	if w.active >= len(w.sheets) {
		w.active = len(w.sheets) - 1
	}
	if w.active < 0 {
		w.active = 0
	}
	return sh
}

// RenameSheetByID renames without the duplicate-name check. Replaying history
// can pass through transient states that a user could never type, so the check
// would be wrong there.
func (w *Workbook) RenameSheetByID(id uint32, name string) bool {
	sh, ok := w.SheetByID(id)
	if !ok {
		return false
	}
	delete(w.index, strings.ToLower(sh.Name))
	sh.Name = name
	w.index[strings.ToLower(name)] = sh
	return true
}
