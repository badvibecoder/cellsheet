// Package journal records what changed and why.
//
// It is the single source of both undo (per-edit, in memory) and checkpoints
// (persisted, coarse). Nothing else in the program needs to know how either
// works: a mutation site calls Before* on a Recorder before it changes
// something, and Build diffs the recorded state against the workbook afterwards.
//
// Only *user-visible* content is recorded: cell sources, literal values, row and
// column sizes, and sheet lifecycle. Formula results are deliberately not
// recorded, because formulas are stored as source and recomputed on load, so a
// recomputed value can never be stale.
package journal

import (
	"sort"
	"sync"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

// Kind is why a checkpoint or undo step exists, which is what gives it a label.
type Kind uint8

const (
	KindInitial Kind = iota
	KindAuto
	KindCellEdited
	KindRowEdited
	KindBulkPaste
	KindStructural
	KindManual
	KindRollback
)

func (k Kind) String() string {
	switch k {
	case KindAuto:
		return "Auto-Checkpoint"
	case KindCellEdited:
		return "Cell edited"
	case KindRowEdited:
		return "Row edited"
	case KindBulkPaste:
		return "Bulk Paste"
	case KindStructural:
		return "Structural change"
	case KindManual:
		return "Manual checkpoint"
	case KindRollback:
		return "Rollback"
	default:
		return "Sheet initialized"
	}
}

// CellChange is one cell's content before and after.
type CellChange struct {
	Sheet  uint32
	Ref    grid.Ref
	Before cell.Cell
	After  cell.Cell
}

// RowChange is one row's height before and after.
type RowChange struct {
	Sheet         uint32
	Row           uint32
	Before, After int
}

// ColChange is one column's width before and after.
type ColChange struct {
	Sheet         uint32
	Col           uint32
	Before, After int
}

// SheetState is the complete content of one sheet, used when cells shift and a
// per-reference diff cannot describe the result.
type SheetState struct {
	ID       uint32
	Name     string
	Rows     uint32
	Cols     uint32
	DefRowH  int
	DefColW  int
	Cells    []CellEntry
	RowSizes map[uint32]int
	ColSizes map[uint32]int
}

// CellEntry is one populated cell, in a deterministic order.
type CellEntry struct {
	Ref  grid.Ref
	Cell cell.Cell
}

// CaptureSheet snapshots a sheet's entire content.
func CaptureSheet(sh *grid.Sheet) SheetState {
	st := SheetState{
		ID:       sh.ID,
		Name:     sh.Name,
		Rows:     sh.Rows(),
		Cols:     sh.Cols(),
		DefRowH:  sh.DefaultRowHeight(),
		DefColW:  sh.DefaultColWidth(),
		RowSizes: map[uint32]int{},
		ColSizes: map[uint32]int{},
	}
	sh.Each(func(ref grid.Ref, cl cell.Cell) bool {
		st.Cells = append(st.Cells, CellEntry{Ref: ref, Cell: cl})
		return true
	})
	// Sorted so that two identical sheets serialise identically, which keeps
	// deltas and file contents reproducible.
	sort.Slice(st.Cells, func(i, j int) bool {
		if st.Cells[i].Ref.Row != st.Cells[j].Ref.Row {
			return st.Cells[i].Ref.Row < st.Cells[j].Ref.Row
		}
		return st.Cells[i].Ref.Col < st.Cells[j].Ref.Col
	})
	for r, h := range sh.RowsSet() {
		st.RowSizes[r] = h
	}
	for c, w := range sh.ColsSet() {
		st.ColSizes[c] = w
	}
	return st
}

// Restore replaces (or creates) the sheet at the given index with this state.
func (st SheetState) Restore(wb *grid.Workbook, index int) {
	sh := grid.NewSheet(st.ID, st.Name)
	sh.SetSize(st.Rows, st.Cols)
	sh.SetDefaults(st.DefRowH, st.DefColW)
	for _, e := range st.Cells {
		sh.Set(e.Ref.Row, e.Ref.Col, e.Cell)
	}
	for r, h := range st.RowSizes {
		sh.SetRowHeightRaw(r, h)
	}
	for c, w := range st.ColSizes {
		sh.SetColWidthRaw(c, w)
	}
	wb.PutSheet(index, sh)
}

// SheetAdd is a sheet that appeared.
type SheetAdd struct {
	Index int
	State SheetState
}

// SheetRemove is a sheet that disappeared, with enough content to bring it back.
type SheetRemove struct {
	Index int
	State SheetState
}

// SheetRename is a name change.
type SheetRename struct {
	ID            uint32
	Before, After string
}

// SnapshotChange is a whole-sheet before/after, used for structural edits where
// many cells move at once.
type SnapshotChange struct {
	Index  int
	Sheet  uint32
	Before SheetState
	After  SheetState
}

// Delta is one reversible step.
type Delta struct {
	Kind  Kind
	Label string
	At    time.Time

	Cells []CellChange
	Rows  []RowChange
	Cols  []ColChange

	Added   []SheetAdd
	Removed []SheetRemove
	Renamed []SheetRename

	Snapshots []SnapshotChange

	// BeforeWorkbook and AfterWorkbook, when set, replace the entire workbook.
	// They are used by rollback, which composes many steps into one reversible
	// action.
	BeforeWorkbook *WorkbookState
	AfterWorkbook  *WorkbookState
}

// Empty reports whether the delta changes nothing, so it can be discarded
// rather than filling the history with no-ops.
func (d Delta) Empty() bool {
	return len(d.Cells) == 0 && len(d.Rows) == 0 && len(d.Cols) == 0 &&
		len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Renamed) == 0 &&
		len(d.Snapshots) == 0 && d.BeforeWorkbook == nil && d.AfterWorkbook == nil
}

// Summary counts what changed, for the label the Rollback menu shows.
func (d Delta) Summary() (cells, sheets int) {
	return len(d.Cells), len(d.Added) + len(d.Removed) + len(d.Renamed) + len(d.Snapshots)
}

// Reverse returns the delta that undoes this one.
func (d Delta) Reverse() Delta {
	out := Delta{Kind: KindRollback, Label: d.Label, At: d.At}
	for _, c := range d.Cells {
		out.Cells = append(out.Cells, CellChange{c.Sheet, c.Ref, c.After, c.Before})
	}
	for _, r := range d.Rows {
		out.Rows = append(out.Rows, RowChange{r.Sheet, r.Row, r.After, r.Before})
	}
	for _, c := range d.Cols {
		out.Cols = append(out.Cols, ColChange{c.Sheet, c.Col, c.After, c.Before})
	}
	for _, s := range d.Snapshots {
		out.Snapshots = append(out.Snapshots, SnapshotChange{
			Index: s.Index, Sheet: s.Sheet, Before: s.After, After: s.Before,
		})
	}
	// Lifecycle operations swap roles.
	for _, a := range d.Added {
		out.Removed = append(out.Removed, SheetRemove(a))
	}
	for _, r := range d.Removed {
		out.Added = append(out.Added, SheetAdd(r))
	}
	for _, rn := range d.Renamed {
		out.Renamed = append(out.Renamed, SheetRename{rn.ID, rn.After, rn.Before})
	}
	if d.BeforeWorkbook != nil || d.AfterWorkbook != nil {
		out.BeforeWorkbook = d.AfterWorkbook
		out.AfterWorkbook = d.BeforeWorkbook
	}
	return out
}

// ApplyForward restores the "after" state.
func (d Delta) ApplyForward(wb *grid.Workbook) { d.apply(wb, true) }

// ApplyReverse restores the "before" state.
func (d Delta) ApplyReverse(wb *grid.Workbook) { d.apply(wb, false) }

func (d Delta) apply(wb *grid.Workbook, forward bool) {
	if d.BeforeWorkbook != nil || d.AfterWorkbook != nil {
		st := d.BeforeWorkbook
		if forward {
			st = d.AfterWorkbook
		}
		if st != nil {
			st.Restore(wb)
			return
		}
	}
	for _, s := range d.Snapshots {
		st := s.Before
		if forward {
			st = s.After
		}
		st.Restore(wb, s.Index)
	}
	for _, c := range d.Cells {
		cl := c.Before
		if forward {
			cl = c.After
		}
		if sh, ok := wb.SheetByID(c.Sheet); ok {
			sh.Set(c.Ref.Row, c.Ref.Col, cl)
		}
	}
	for _, r := range d.Rows {
		h := r.Before
		if forward {
			h = r.After
		}
		if sh, ok := wb.SheetByID(r.Sheet); ok {
			sh.SetRowHeightRaw(r.Row, h)
		}
	}
	for _, c := range d.Cols {
		w := c.Before
		if forward {
			w = c.After
		}
		if sh, ok := wb.SheetByID(c.Sheet); ok {
			sh.SetColWidthRaw(c.Col, w)
		}
	}
	if forward {
		for _, a := range d.Added {
			a.State.Restore(wb, a.Index)
		}
		for _, r := range d.Removed {
			wb.RemoveSheetAt(r.Index)
		}
		for _, rn := range d.Renamed {
			wb.RenameSheetByID(rn.ID, rn.After)
		}
		return
	}
	for _, a := range d.Added {
		wb.RemoveSheetAt(a.Index)
	}
	for _, r := range d.Removed {
		r.State.Restore(wb, r.Index)
	}
	for _, rn := range d.Renamed {
		wb.RenameSheetByID(rn.ID, rn.Before)
	}
}

// ---------------------------------------------------------------------------
// Recorder
// ---------------------------------------------------------------------------

type cellKey struct {
	sheet uint32
	ref   grid.Ref
}

// Recorder captures the before-state of everything a mutation touches, then
// diffs it against the workbook to produce a Delta.
type Recorder struct {
	mu      sync.Mutex
	cells   map[cellKey]cell.Cell
	rows    map[cellKey]int
	cols    map[cellKey]int
	added   []SheetAdd
	removed []SheetRemove
	renamed []SheetRename
	sheets  []SheetState
}

// NewRecorder creates an empty recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) reset() {
	r.cells = nil
	r.rows = nil
	r.cols = nil
	r.added = nil
	r.removed = nil
	r.renamed = nil
	r.sheets = nil
}

// BeforeCell records a cell's current content.
func (r *Recorder) BeforeCell(sheet uint32, ref grid.Ref, before cell.Cell) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cells == nil {
		r.cells = map[cellKey]cell.Cell{}
	}
	k := cellKey{sheet, ref}
	if _, seen := r.cells[k]; !seen {
		r.cells[k] = before
	}
}

// BeforeRow records a row's current height.
func (r *Recorder) BeforeRow(sheet uint32, row uint32, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rows == nil {
		r.rows = map[cellKey]int{}
	}
	k := cellKey{sheet, grid.Ref{Row: row}}
	if _, seen := r.rows[k]; !seen {
		r.rows[k] = h
	}
}

// BeforeCol records a column's current width.
func (r *Recorder) BeforeCol(sheet uint32, col uint32, w int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cols == nil {
		r.cols = map[cellKey]int{}
	}
	k := cellKey{sheet, grid.Ref{Col: col}}
	if _, seen := r.cols[k]; !seen {
		r.cols[k] = w
	}
}

// BeforeSheetAdd records that a sheet is about to be inserted. The sheet's
// content and final position are captured when the delta is built, because at
// this point the sheet does not exist yet.
func (r *Recorder) BeforeSheetAdd(index int, state SheetState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.added = append(r.added, SheetAdd{Index: index, State: state})
}

// BeforeSheetRemove records that a sheet is about to be removed.
func (r *Recorder) BeforeSheetRemove(index int, state SheetState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, SheetRemove{Index: index, State: state})
}

// BeforeSheetRename records a sheet's current name.
func (r *Recorder) BeforeSheetRename(id uint32, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renamed = append(r.renamed, SheetRename{ID: id, Before: name})
}

// BeforeSnapshot records an entire sheet, for structural edits where cells move
// and a per-reference diff cannot describe the outcome.
func (r *Recorder) BeforeSnapshot(sh *grid.Sheet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sheets = append(r.sheets, CaptureSheet(sh))
}

// Build diffs the recorded state against the workbook and returns the delta.
// It always resets the recorder, so a checkpoint cannot be produced twice.
func (r *Recorder) Build(wb *grid.Workbook, kind Kind, label string, at time.Time) Delta {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := Delta{Kind: kind, Label: label, At: at}

	for k, before := range r.cells {
		sh, ok := wb.SheetByID(k.sheet)
		if !ok {
			continue
		}
		after := sh.GetRef(k.ref)
		if sameCell(before, after) {
			continue
		}
		d.Cells = append(d.Cells, CellChange{Sheet: k.sheet, Ref: k.ref, Before: before, After: after})
	}
	sort.Slice(d.Cells, func(i, j int) bool {
		a, b := d.Cells[i], d.Cells[j]
		if a.Sheet != b.Sheet {
			return a.Sheet < b.Sheet
		}
		if a.Ref.Row != b.Ref.Row {
			return a.Ref.Row < b.Ref.Row
		}
		return a.Ref.Col < b.Ref.Col
	})

	for k, before := range r.rows {
		sh, ok := wb.SheetByID(k.sheet)
		if !ok {
			continue
		}
		after := sh.RowHeight(k.ref.Row)
		if before == after {
			continue
		}
		d.Rows = append(d.Rows, RowChange{Sheet: k.sheet, Row: k.ref.Row, Before: before, After: after})
	}
	for k, before := range r.cols {
		sh, ok := wb.SheetByID(k.sheet)
		if !ok {
			continue
		}
		after := sh.ColWidth(k.ref.Col)
		if before == after {
			continue
		}
		d.Cols = append(d.Cols, ColChange{Sheet: k.sheet, Col: k.ref.Col, Before: before, After: after})
	}

	for _, st := range r.sheets {
		sh, ok := wb.SheetByID(st.ID)
		if !ok {
			continue
		}
		idx := wb.IndexOf(sh)
		after := CaptureSheet(sh)
		if sameSheetState(st, after) {
			continue
		}
		d.Snapshots = append(d.Snapshots, SnapshotChange{Index: idx, Sheet: st.ID, Before: st, After: after})
	}

	// A sheet that was just added must be captured *now*, so that reapplying
	// the delta recreates it with the content it has.
	for _, a := range r.added {
		if sh, ok := wb.SheetByID(a.State.ID); ok {
			a.State = CaptureSheet(sh)
			a.Index = wb.IndexOf(sh)
		}
		d.Added = append(d.Added, a)
	}
	d.Removed = append(d.Removed, r.removed...)
	for i := range r.renamed {
		rn := &r.renamed[i]
		sh, ok := wb.SheetByID(rn.ID)
		if !ok || sh.Name == rn.Before {
			continue
		}
		d.Renamed = append(d.Renamed, SheetRename{ID: rn.ID, Before: rn.Before, After: sh.Name})
	}

	r.reset()
	return d
}

func sameCell(a, b cell.Cell) bool {
	if a.Source != b.Source {
		return false
	}
	if a.Value.Kind != b.Value.Kind {
		return false
	}
	switch a.Value.Kind {
	case cell.KindNumber, cell.KindCurrency:
		return a.Value.Num.Equal(b.Value.Num)
	case cell.KindText:
		return a.Value.Str == b.Value.Str
	case cell.KindError:
		return a.Value.Code == b.Value.Code
	default:
		return true
	}
}

func sameSheetState(a, b SheetState) bool {
	if a.Name != b.Name || a.Rows != b.Rows || a.Cols != b.Cols ||
		a.DefRowH != b.DefRowH || a.DefColW != b.DefColW || len(a.Cells) != len(b.Cells) {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i].Ref != b.Cells[i].Ref || !sameCell(a.Cells[i].Cell, b.Cells[i].Cell) {
			return false
		}
	}
	return true
}

// WorkbookState is every sheet plus which one is showing. It is the blunt
// instrument used for rollbacks: composing a chain of per-cell deltas into one
// reversible delta is easy to get subtly wrong, whereas capturing the whole
// workbook before and after is obviously correct. Rollbacks are rare enough
// that the size does not matter.
type WorkbookState struct {
	Sheets []SheetState
	Active int
}

// CaptureWorkbook snapshots every sheet in tab order.
func CaptureWorkbook(wb *grid.Workbook) WorkbookState {
	st := WorkbookState{Active: wb.ActiveIndex()}
	for _, sh := range wb.Sheets() {
		st.Sheets = append(st.Sheets, CaptureSheet(sh))
	}
	return st
}

// Restore replaces the workbook's entire contents with this state.
func (st WorkbookState) Restore(wb *grid.Workbook) {
	for wb.Len() > 0 {
		wb.RemoveSheetAt(0)
	}
	for i, s := range st.Sheets {
		s.Restore(wb, i)
	}
	wb.SetActive(st.Active)
}

// Same reports whether two workbook snapshots hold the same content.
func (st WorkbookState) Same(other WorkbookState) bool {
	if len(st.Sheets) != len(other.Sheets) || st.Active != other.Active {
		return false
	}
	for i := range st.Sheets {
		if !sameSheetState(st.Sheets[i], other.Sheets[i]) {
			return false
		}
	}
	return true
}

// Combine folds a run of per-edit deltas into the single delta that describes
// them all, which is what an automatic checkpoint stores.
//
// Structural changes are deliberately not combined: the design takes a
// checkpoint immediately when one happens, so a structural delta is always a
// checkpoint of its own. If one reaches here anyway it is carried through
// verbatim, which is correct if less compact.
func Combine(deltas []Delta) Delta {
	switch len(deltas) {
	case 0:
		return Delta{}
	case 1:
		return deltas[0]
	}
	out := Delta{Kind: KindAuto, Label: "Auto-Checkpoint", At: deltas[len(deltas)-1].At}

	type ck struct {
		sheet uint32
		ref   grid.Ref
	}
	cellIdx := map[ck]int{}
	rowIdx := map[ck]int{}
	colIdx := map[ck]int{}

	for _, d := range deltas {
		for _, c := range d.Cells {
			k := ck{c.Sheet, c.Ref}
			if i, ok := cellIdx[k]; ok {
				out.Cells[i].After = c.After
				continue
			}
			cellIdx[k] = len(out.Cells)
			out.Cells = append(out.Cells, c)
		}
		for _, r := range d.Rows {
			k := ck{r.Sheet, grid.Ref{Row: r.Row}}
			if i, ok := rowIdx[k]; ok {
				out.Rows[i].After = r.After
				continue
			}
			rowIdx[k] = len(out.Rows)
			out.Rows = append(out.Rows, r)
		}
		for _, c := range d.Cols {
			k := ck{c.Sheet, grid.Ref{Col: c.Col}}
			if i, ok := colIdx[k]; ok {
				out.Cols[i].After = c.After
				continue
			}
			colIdx[k] = len(out.Cols)
			out.Cols = append(out.Cols, c)
		}
		out.Added = append(out.Added, d.Added...)
		out.Removed = append(out.Removed, d.Removed...)
		out.Renamed = append(out.Renamed, d.Renamed...)
		out.Snapshots = append(out.Snapshots, d.Snapshots...)
		if d.BeforeWorkbook != nil || d.AfterWorkbook != nil {
			out.BeforeWorkbook = d.BeforeWorkbook
			out.AfterWorkbook = d.AfterWorkbook
		}
	}

	// A change that ended where it started is not worth remembering.
	cells := out.Cells[:0]
	for _, c := range out.Cells {
		if sameCell(c.Before, c.After) {
			continue
		}
		cells = append(cells, c)
	}
	out.Cells = cells
	rows := out.Rows[:0]
	for _, r := range out.Rows {
		if r.Before == r.After {
			continue
		}
		rows = append(rows, r)
	}
	out.Rows = rows
	cols := out.Cols[:0]
	for _, c := range out.Cols {
		if c.Before == c.After {
			continue
		}
		cols = append(cols, c)
	}
	out.Cols = cols

	sort.Slice(out.Cells, func(i, j int) bool {
		a, b := out.Cells[i], out.Cells[j]
		if a.Sheet != b.Sheet {
			return a.Sheet < b.Sheet
		}
		if a.Ref.Row != b.Ref.Row {
			return a.Ref.Row < b.Ref.Row
		}
		return a.Ref.Col < b.Ref.Col
	})
	return out
}
