package sheetfile

import (
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// Record tags. Unknown tags are skipped by the decoder so that a newer writer
// can add records an older reader does not understand.
const (
	tagSheetBegin = 1
	tagRowMeta    = 2
	tagColMeta    = 3
	tagCell       = 4
	tagSheetEnd   = 5

	tagDeltaMeta     = 20
	tagCellChange    = 21
	tagRowChange     = 22
	tagColChange     = 23
	tagSheetAdd      = 24
	tagSheetRemove   = 25
	tagSheetRename   = 26
	tagSnapshot      = 27
	tagWorkbookState = 28
	tagEntryEnd      = 29
)

// Cell kinds in the file. These numbers are part of the format and must not be
// renumbered.
const (
	cellEmpty    = 0
	cellNumber   = 1
	cellCurrency = 2
	cellText     = 3
	cellFormula  = 4
	cellError    = 5
)

// hasSourceBit marks a literal cell that carries its original typed text.
//
// The display form is lossy: currency always shows two decimals, so $0.0001
// displays as "$0.00" and cannot be recovered from it. The source is therefore
// always stored for literal cells, and never reconstructed from the display.
const hasSourceBit = 0x80

func writeCell(w *writer, cl cell.Cell) {
	if cl.IsFormula() {
		// A formula stores only its source; the value is recomputed on load.
		w.byte(cellFormula)
		w.str(cl.Source)
		return
	}
	var kind byte
	switch cl.Value.Kind {
	case cell.KindNumber:
		kind = cellNumber
	case cell.KindCurrency:
		kind = cellCurrency
	case cell.KindText:
		kind = cellText
	case cell.KindError:
		kind = cellError
	default:
		// A cell can have a source and no value: typing a single space does
		// exactly that. The kind byte says "empty" and the source still
		// follows, which readCell mirrors.
		kind = cellEmpty
	}
	if cl.Source != "" {
		kind |= hasSourceBit
	}
	w.byte(kind)

	switch cl.Value.Kind {
	case cell.KindNumber, cell.KindCurrency:
		w.varint(cl.Value.Num.Mant)
		w.uvarint(uint64(cl.Value.Num.Scale))
	case cell.KindText:
		w.str(cl.Value.Str)
	case cell.KindError:
		w.byte(byte(cl.Value.Code))
	}
	if kind&hasSourceBit != 0 {
		w.str(cl.Source)
	}
}

func readCell(r *reader) cell.Cell {
	k := r.byte()
	if r.err != nil {
		return cell.Cell{}
	}
	hasSource := k&hasSourceBit != 0
	kind := k &^ hasSourceBit

	var cl cell.Cell
	switch kind {
	case cellFormula:
		// A formula carries only its source; the value is recomputed on load.
		cl.Source = r.str()
		return cl
	case cellNumber:
		mant := r.varint()
		scale := r.uvarint()
		cl.Value = cell.Number(cell.NewDec(mant, int8(min(scale, 18))))
	case cellCurrency:
		mant := r.varint()
		scale := r.uvarint()
		cl.Value = cell.Currency(cell.NewDec(mant, int8(min(scale, 18))))
	case cellText:
		cl.Value = cell.Text(r.str())
	case cellError:
		cl.Value = cell.Error(cell.ErrorCode(r.byte()))
	case cellEmpty:
		// A cell can legitimately have a source and no value — typing a single
		// space does exactly that — so its payload is empty but its source
		// still follows.
	default:
		// An unknown kind byte means the section cannot be trusted to have the
		// right length, so stop rather than misread everything after it.
		r.fail()
		return cell.Cell{}
	}
	if hasSource {
		cl.Source = r.str()
	}
	return cl
}

// writeSheetState encodes one sheet's complete content.
func writeSheetState(w *writer, st journal.SheetState) {
	w.uvarint(uint64(st.ID))
	w.str(st.Name)
	w.uvarint(uint64(st.Rows))
	w.uvarint(uint64(st.Cols))
	w.uvarint(uint64(st.DefRowH))
	w.uvarint(uint64(st.DefColW))

	w.uvarint(uint64(len(st.Cells)))
	for _, e := range st.Cells {
		w.uvarint(uint64(e.Ref.Row))
		w.uvarint(uint64(e.Ref.Col))
		writeCell(w, e.Cell)
	}
	// Sizes are written as sorted slices so the encoding is deterministic.
	rows := sortedKeys(st.RowSizes)
	w.uvarint(uint64(len(rows)))
	for _, r := range rows {
		w.uvarint(uint64(r))
		w.uvarint(uint64(st.RowSizes[r]))
	}
	cols := sortedKeys(st.ColSizes)
	w.uvarint(uint64(len(cols)))
	for _, c := range cols {
		w.uvarint(uint64(c))
		w.uvarint(uint64(st.ColSizes[c]))
	}
}

func readSheetState(r *reader) journal.SheetState {
	var st journal.SheetState
	st.ID = uint32(r.uvarint())
	st.Name = r.str()
	st.Rows = uint32(r.uvarint())
	st.Cols = uint32(r.uvarint())
	st.DefRowH = int(r.uvarint())
	st.DefColW = int(r.uvarint())

	n := r.uvarint()
	if n > uint64(r.remaining()) { // one byte per entry is the floor
		r.fail()
		return st
	}
	st.Cells = make([]journal.CellEntry, 0, min(n, 1<<20))
	for i := uint64(0); i < n && r.err == nil; i++ {
		ref := grid.Ref{Row: uint32(r.uvarint()), Col: uint32(r.uvarint())}
		cl := readCell(r)
		st.Cells = append(st.Cells, journal.CellEntry{Ref: ref, Cell: cl})
	}
	rn := r.uvarint()
	if rn > 0 {
		st.RowSizes = make(map[uint32]int, min(rn, 1<<16))
	}
	for i := uint64(0); i < rn && r.err == nil; i++ {
		k := uint32(r.uvarint())
		st.RowSizes[k] = int(r.uvarint())
	}
	cn := r.uvarint()
	if cn > 0 {
		st.ColSizes = make(map[uint32]int, min(cn, 1<<16))
	}
	for i := uint64(0); i < cn && r.err == nil; i++ {
		k := uint32(r.uvarint())
		st.ColSizes[k] = int(r.uvarint())
	}
	return st
}

func writeWorkbookState(w *writer, st journal.WorkbookState) {
	w.uvarint(uint64(st.Active))
	w.uvarint(uint64(len(st.Sheets)))
	for _, s := range st.Sheets {
		writeSheetState(w, s)
	}
}

func readWorkbookState(r *reader) journal.WorkbookState {
	var st journal.WorkbookState
	st.Active = int(r.uvarint())
	n := r.uvarint()
	if n > uint64(r.remaining()) {
		r.fail()
		return st
	}
	st.Sheets = make([]journal.SheetState, 0, min(n, 1<<16))
	for i := uint64(0); i < n && r.err == nil; i++ {
		st.Sheets = append(st.Sheets, readSheetState(r))
	}
	return st
}

// ---------------------------------------------------------------------------
// Workbook STATE section
// ---------------------------------------------------------------------------

// encodeState writes every sheet of a workbook.
func encodeState(wb *grid.Workbook) []byte {
	var w writer
	for _, sh := range wb.Sheets() {
		st := journal.CaptureSheet(sh)
		w.byte(tagSheetBegin)
		writeSheetState(&w, st)
		w.byte(tagSheetEnd)
	}
	return w.buf
}

// decodeState rebuilds a workbook from a STATE section.
func decodeState(buf []byte) (*grid.Workbook, error) {
	wb := grid.NewWorkbook()
	// Start from nothing so that sheet order comes entirely from the file.
	for wb.Len() > 0 {
		wb.RemoveSheetAt(0)
	}
	r := &reader{buf: buf}
	for r.err == nil && r.remaining() > 0 {
		tag := r.byte()
		switch tag {
		case tagSheetBegin:
			st := readSheetState(r)
			st.Restore(wb, wb.Len())
			_ = r.byte() // tagSheetEnd
		case tagSheetEnd:
			// tolerate a stray terminator
		default:
			// An unknown record type: we cannot know its length, so the
			// section is unreadable rather than silently misparsed.
			return nil, errShort
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if wb.Len() == 0 {
		wb.PutSheet(0, grid.NewSheet(1, "Sheet1"))
	}
	if wb.Active() == nil {
		wb.SetActive(0)
	}
	return wb, nil
}

// ---------------------------------------------------------------------------
// HISTORY section
// ---------------------------------------------------------------------------

func writeDelta(w *writer, d journal.Delta) {
	w.byte(tagDeltaMeta)
	w.byte(byte(d.Kind))
	w.str(d.Label)
	w.varint(d.At.UnixMilli())

	for _, c := range d.Cells {
		w.byte(tagCellChange)
		w.uvarint(uint64(c.Sheet))
		w.uvarint(uint64(c.Ref.Row))
		w.uvarint(uint64(c.Ref.Col))
		writeCell(w, c.Before)
		writeCell(w, c.After)
	}
	for _, rc := range d.Rows {
		w.byte(tagRowChange)
		w.uvarint(uint64(rc.Sheet))
		w.uvarint(uint64(rc.Row))
		w.uvarint(uint64(rc.Before))
		w.uvarint(uint64(rc.After))
	}
	for _, cc := range d.Cols {
		w.byte(tagColChange)
		w.uvarint(uint64(cc.Sheet))
		w.uvarint(uint64(cc.Col))
		w.uvarint(uint64(cc.Before))
		w.uvarint(uint64(cc.After))
	}
	for _, a := range d.Added {
		w.byte(tagSheetAdd)
		w.uvarint(uint64(a.Index))
		writeSheetState(w, a.State)
	}
	for _, rm := range d.Removed {
		w.byte(tagSheetRemove)
		w.uvarint(uint64(rm.Index))
		writeSheetState(w, rm.State)
	}
	for _, rn := range d.Renamed {
		w.byte(tagSheetRename)
		w.uvarint(uint64(rn.ID))
		w.str(rn.Before)
		w.str(rn.After)
	}
	for _, s := range d.Snapshots {
		w.byte(tagSnapshot)
		w.uvarint(uint64(s.Index))
		w.uvarint(uint64(s.Sheet))
		writeSheetState(w, s.Before)
		writeSheetState(w, s.After)
	}
	if d.BeforeWorkbook != nil || d.AfterWorkbook != nil {
		w.byte(tagWorkbookState)
		if d.BeforeWorkbook != nil {
			w.byte(1)
			writeWorkbookState(w, *d.BeforeWorkbook)
		} else {
			w.byte(0)
		}
		if d.AfterWorkbook != nil {
			w.byte(1)
			writeWorkbookState(w, *d.AfterWorkbook)
		} else {
			w.byte(0)
		}
	}
	w.byte(tagEntryEnd)
}

func readDelta(r *reader) journal.Delta {
	var d journal.Delta
	for r.err == nil && r.remaining() > 0 {
		tag := r.byte()
		switch tag {
		case tagDeltaMeta:
			d.Kind = journal.Kind(r.byte())
			d.Label = r.str()
			d.At = timeFromMillis(r.varint())
		case tagCellChange:
			c := journal.CellChange{}
			c.Sheet = uint32(r.uvarint())
			c.Ref = grid.Ref{Row: uint32(r.uvarint()), Col: uint32(r.uvarint())}
			c.Before = readCell(r)
			c.After = readCell(r)
			d.Cells = append(d.Cells, c)
		case tagRowChange:
			c := journal.RowChange{}
			c.Sheet = uint32(r.uvarint())
			c.Row = uint32(r.uvarint())
			c.Before = int(r.uvarint())
			c.After = int(r.uvarint())
			d.Rows = append(d.Rows, c)
		case tagColChange:
			c := journal.ColChange{}
			c.Sheet = uint32(r.uvarint())
			c.Col = uint32(r.uvarint())
			c.Before = int(r.uvarint())
			c.After = int(r.uvarint())
			d.Cols = append(d.Cols, c)
		case tagSheetAdd:
			a := journal.SheetAdd{Index: int(r.uvarint())}
			a.State = readSheetState(r)
			d.Added = append(d.Added, a)
		case tagSheetRemove:
			rm := journal.SheetRemove{Index: int(r.uvarint())}
			rm.State = readSheetState(r)
			d.Removed = append(d.Removed, rm)
		case tagSheetRename:
			rn := journal.SheetRename{}
			rn.ID = uint32(r.uvarint())
			rn.Before = r.str()
			rn.After = r.str()
			d.Renamed = append(d.Renamed, rn)
		case tagSnapshot:
			s := journal.SnapshotChange{}
			s.Index = int(r.uvarint())
			s.Sheet = uint32(r.uvarint())
			s.Before = readSheetState(r)
			s.After = readSheetState(r)
			d.Snapshots = append(d.Snapshots, s)
		case tagWorkbookState:
			if r.byte() == 1 {
				st := readWorkbookState(r)
				d.BeforeWorkbook = &st
			}
			if r.byte() == 1 {
				st := readWorkbookState(r)
				d.AfterWorkbook = &st
			}
		case tagEntryEnd:
			return d
		default:
			r.fail()
			return d
		}
	}
	return d
}

// encodeHistory writes the rollback entries, newest first.
func encodeHistory(entries []checkpoint.Entry) []byte {
	var w writer
	w.uvarint(uint64(len(entries)))
	for _, e := range entries {
		d := e.Delta
		if d.Kind == 0 && d.Label == "" {
			d.Kind, d.Label, d.At = e.Kind, e.Label, e.At
		}
		writeDelta(&w, d)
	}
	return w.buf
}

func decodeHistory(buf []byte) []checkpoint.Entry {
	r := &reader{buf: buf}
	n := r.uvarint()
	out := make([]checkpoint.Entry, 0, min(n, 128))
	for i := uint64(0); i < n && r.err == nil; i++ {
		d := readDelta(r)
		out = append(out, checkpoint.Entry{
			Kind: d.Kind, Label: d.Label, At: d.At, Delta: d,
		})
	}
	return out
}
