package sheetfile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"

	_ "github.com/badvibecoder/cellsheet/internal/formula/functions"
)

// sample builds a workbook exercising every kind of content the format must
// carry: all five value kinds, a formula, row and column size overrides, and
// two sheets.
func sample(t *testing.T) *grid.Workbook {
	t.Helper()
	wb := grid.NewWorkbook()
	sh := wb.Active()

	set := func(ref, typed string) {
		r, ok := grid.ParseRef(ref)
		if !ok {
			t.Fatalf("bad ref %q", ref)
		}
		sh.Set(r.Row, r.Col, cell.Cell{Source: typed, Value: cell.Infer(typed)})
	}
	set("A1", "$50,000.00")
	set("B1", "$1,250.00")
	set("A2", "450")
	set("B2", "12.75")
	set("A3", "Operations")
	set("B3", "  spaced text  ")
	set("C1", "=SUM(A1:B1)")
	set("D1", "-0.005")

	r, _ := grid.ParseRef("A4")
	sh.Set(r.Row, r.Col, cell.Cell{Value: cell.Error(cell.ErrDivZero)})

	sh.SetRowHeight(4, 3)
	sh.SetColWidth(2, 40)

	if _, err := wb.AddSheet("Q3 Data"); err != nil {
		t.Fatal(err)
	}
	sh2 := wb.Sheets()[1]
	sh2.Set(0, 0, cell.Cell{Source: "second sheet", Value: cell.Text("second sheet")})
	return wb
}

func TestRoundTripWorkbook(t *testing.T) {
	wb := sample(t)
	doc := &Document{Workbook: wb, AppVersion: "test", Created: time.Now(), Modified: time.Now()}

	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workbook.Len() != 2 {
		t.Fatalf("sheet count = %d, want 2", got.Workbook.Len())
	}

	a := wb.Sheets()[0]
	b := got.Workbook.Sheets()[0]
	for row := uint32(0); row < 6; row++ {
		for col := uint32(0); col < 5; col++ {
			x, y := a.Get(row, col), b.Get(row, col)
			// A formula stores only its source; its value is recomputed.
			if x.IsFormula() {
				if y.Source != x.Source {
					t.Errorf("cell %s formula source = %q, want %q", grid.RefName(row, col), y.Source, x.Source)
				}
				continue
			}
			if x.Value.Kind != y.Value.Kind {
				t.Errorf("cell %s kind = %v, want %v", grid.RefName(row, col), y.Value.Kind, x.Value.Kind)
				continue
			}
			if x.Value.Display() != y.Value.Display() {
				t.Errorf("cell %s = %q, want %q", grid.RefName(row, col), y.Value.Display(), x.Value.Display())
			}
			if x.Source != y.Source {
				t.Errorf("cell %s source = %q, want %q", grid.RefName(row, col), y.Source, x.Source)
			}
		}
	}
	if got.Workbook.Sheets()[0].RowHeight(4) != 3 {
		t.Errorf("row height not preserved: %d", got.Workbook.Sheets()[0].RowHeight(4))
	}
	if got.Workbook.Sheets()[0].ColWidth(2) != 40 {
		t.Errorf("column width not preserved: %d", got.Workbook.Sheets()[0].ColWidth(2))
	}
	if got.Workbook.Sheets()[1].Name != "Q3 Data" {
		t.Errorf("sheet name = %q", got.Workbook.Sheets()[1].Name)
	}
}

// TestFormulaSurvivesAsSource is the promise that a file never contains a stale
// answer: only the source is stored, and the value is recomputed on load.
func TestFormulaSurvivesAsSource(t *testing.T) {
	wb := sample(t)
	doc := &Document{Workbook: wb}
	data, _ := Encode(doc)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	cl := got.Workbook.Sheets()[0].Get(0, 2)
	if cl.Source != "=SUM(A1:B1)" {
		t.Fatalf("formula source = %q", cl.Source)
	}
	if !cl.Value.IsEmpty() {
		t.Error("a formula's value must not be stored, only its source")
	}
	// After the engine recalculates, the value is real again.
	eng := calc.New(got.Workbook)
	defer eng.Close()
	eng.RecalcWorkbook(nil)
	if v := got.Workbook.Sheets()[0].Get(0, 2).Value.Display(); v != "$51,250.00" {
		t.Errorf("recalculated C1 = %q, want $51,250.00", v)
	}
}

func TestTextIsPreservedVerbatim(t *testing.T) {
	wb := sample(t)
	doc := &Document{Workbook: wb}
	data, _ := Encode(doc)
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Workbook.Sheets()[0].Get(2, 1).Value.Str; v != "  spaced text  " {
		t.Errorf("text = %q, want it verbatim", v)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	now := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)

	var entries []checkpoint.Entry
	sh.Set(0, 0, cell.Cell{Source: "0", Value: cell.Number(cell.FromInt64(0))})
	prev := sh.GetRef(ref)
	sh.Set(0, 0, cell.Cell{Source: "$500.00", Value: cell.Infer("$500.00")})
	entries = append([]checkpoint.Entry{{
		Kind: checkpoint.KindCellEdited, Label: "Cell A1 edited", At: now,
		Delta: journal.Delta{
			Kind: checkpoint.KindCellEdited, Label: "Cell A1 edited", At: now,
			Cells: []journal.CellChange{{Sheet: sh.ID, Ref: ref, Before: prev, After: sh.GetRef(ref)}},
		},
	}}, entries...)

	before := journal.CaptureWorkbook(wb)
	sh.Set(0, 0, cell.Cell{Source: "999", Value: cell.Number(cell.FromInt64(999))})
	after := journal.CaptureWorkbook(wb)
	entries = append([]checkpoint.Entry{{
		Kind: checkpoint.KindStructural, Label: "Bulk Paste", At: now.Add(time.Minute),
		Delta: journal.Delta{
			Kind: checkpoint.KindStructural, Label: "Bulk Paste", At: now.Add(time.Minute),
			BeforeWorkbook: &before, AfterWorkbook: &after,
		},
	}}, entries...)

	doc := &Document{Workbook: wb, History: entries}
	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 2 {
		t.Fatalf("history entries = %d, want 2", len(got.History))
	}
	if got.History[0].Label != "Bulk Paste" || got.History[0].Kind != checkpoint.KindStructural {
		t.Errorf("entry 0 = %+v", got.History[0])
	}
	if got.History[0].Delta.AfterWorkbook == nil {
		t.Error("the whole-workbook payload was lost")
	}
	if got.History[1].Delta.Cells[0].After.Value.Display() != "$500.00" {
		t.Errorf("the cell delta was lost: %+v", got.History[1].Delta.Cells)
	}
}

// TestRollbackSurvivesASave is the point of the whole format: the rollback
// points must work in a reloaded file.
func TestRollbackSurvivesASave(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	ref := grid.Ref{Row: 0, Col: 0}
	base := time.Date(2026, 3, 4, 22, 0, 0, 0, time.Local)
	h := checkpoint.New(base, time.Minute)

	sh.Set(0, 0, cell.Cell{Source: "0", Value: cell.Number(cell.FromInt64(0))})
	for i := 1; i <= 3; i++ {
		prev := sh.GetRef(ref)
		cl := cell.Cell{Source: string(rune('0' + i)), Value: cell.Number(cell.FromInt64(int64(i)))}
		sh.Set(ref.Row, ref.Col, cl)
		h.Push(journal.Delta{
			Kind: checkpoint.KindCellEdited, Label: "Cell edited", At: base.Add(time.Duration(i) * time.Minute),
			Cells: []journal.CellChange{{Sheet: sh.ID, Ref: ref, Before: prev, After: cl}},
		})
	}

	doc := &Document{Workbook: wb, History: h.Snapshot()}
	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	h2 := checkpoint.New(base, time.Minute)
	h2.Restore(loaded.History, base)
	if h2.Len() != 3 {
		t.Fatalf("reloaded history has %d steps, want 3", h2.Len())
	}
	if err := h2.Rollback(loaded.Workbook, 3, base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if v := loaded.Workbook.Active().Get(0, 0).Value.Display(); v != "0" {
		t.Errorf("after rolling back the reloaded file, A1 = %s, want 0", v)
	}
}

func TestEncodingIsDeterministic(t *testing.T) {
	wb := sample(t)
	doc := &Document{Workbook: wb, AppVersion: "test"}
	a, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc2 := &Document{Workbook: sample(t), AppVersion: "test"}
	b, err := Encode(doc2)
	if err != nil {
		t.Fatal(err)
	}
	// The container embeds timestamps, so compare the section payloads only.
	_, sa, _ := decodeContainer(a)
	_, sb, _ := decodeContainer(b)
	for i := range sa {
		if sa[i].ID != sb[i].ID || !bytes.Equal(sa[i].Data, sb[i].Data) {
			t.Errorf("section %d differs between two identical workbooks", sa[i].ID)
		}
	}
}

func TestCompressionHelps(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	for r := uint32(0); r < 500; r++ {
		sh.Set(r, 0, cell.Cell{Source: "repeat", Value: cell.Text("a fairly repetitive string")})
	}
	doc := &Document{Workbook: wb}
	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	raw := len(encodeState(wb))
	if len(data) >= raw {
		t.Errorf("compressed file is %d bytes for %d bytes of state; compression is not working", len(data), raw)
	}
}

// ---------------------------------------------------------------------------
// Corruption and version handling
// ---------------------------------------------------------------------------

func TestRejectsWrongMagic(t *testing.T) {
	wb := grid.NewWorkbook()
	data, _ := Encode(&Document{Workbook: wb})
	data[0] = 'X'
	if _, err := Decode(data); !errors.Is(err, ErrWrongMagic) {
		t.Errorf("err = %v, want ErrWrongMagic", err)
	}
}

func TestRejectsTruncatedFile(t *testing.T) {
	wb := grid.NewWorkbook()
	data, _ := Encode(&Document{Workbook: wb})
	for _, n := range []int{0, 10, 63, 64, len(data) - 1} {
		if _, err := Decode(data[:n]); err == nil {
			t.Errorf("a file truncated to %d bytes should be rejected", n)
		}
	}
}

func TestDetectsCorruption(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	for r := uint32(0); r < 20; r++ {
		sh.Set(r, 0, cell.Cell{Source: "x", Value: cell.Text("some content that will be corrupted")})
	}
	data, _ := Encode(&Document{Workbook: wb})

	// Flip a bit in the middle of the payload: the trailer hash must catch it.
	corrupt := append([]byte(nil), data...)
	corrupt[len(corrupt)/2] ^= 0xFF
	if _, err := Decode(corrupt); !errors.Is(err, ErrCorrupt) {
		t.Errorf("err = %v, want ErrCorrupt", err)
	}

	// Editing the header must be caught by the header checksum.
	corrupt2 := append([]byte(nil), data...)
	corrupt2[9] ^= 0x01
	if _, err := Decode(corrupt2); !errors.Is(err, ErrCorrupt) {
		t.Errorf("err = %v, want ErrCorrupt for an edited header", err)
	}
}

func TestRefusesFutureFile(t *testing.T) {
	sections := []Section{{ID: SectionState, Data: encodeState(grid.NewWorkbook())}}
	data, err := encodeContainer(Header{
		FormatVersion:    FormatVersion + 5,
		MinReaderVersion: MinReaderVersion + 5,
	}, sections)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); !errors.Is(err, ErrTooNew) {
		t.Errorf("err = %v, want ErrTooNew", err)
	}
}

// TestUnknownSectionsArePreserved covers forward compatibility: a file written
// by a newer version must survive a save by this one.
func TestUnknownSectionsArePreserved(t *testing.T) {
	wb := grid.NewWorkbook()
	extra := Section{ID: 99, Data: []byte("data from a future version")}
	doc := &Document{Workbook: wb, Unknown: []Section{extra}}
	data, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unknown) != 1 || string(got.Unknown[0].Data) != string(extra.Data) {
		t.Fatalf("the unknown section was not carried through: %+v", got.Unknown)
	}
	// And saving again keeps it.
	again, err := Encode(got)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(again)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Unknown) != 1 {
		t.Error("the unknown section was lost on the second save")
	}
}

func TestMissingStateSection(t *testing.T) {
	data, err := encodeContainer(Header{}, []Section{{ID: SectionManifest, Data: []byte("{}")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); !errors.Is(err, ErrMissingPart) {
		t.Errorf("err = %v, want ErrMissingPart", err)
	}
}

// ---------------------------------------------------------------------------
// Saving to disk
// ---------------------------------------------------------------------------

func TestSaveAndLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "budget.cell")
	wb := sample(t)
	created := time.Date(2026, 3, 4, 21, 47, 12, 0, time.Local)

	if err := Save(path, &Document{Workbook: wb, Created: created, AppVersion: "test"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("the file is empty")
	}
	// No temporary file may be left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}

	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Workbook.Len() != 2 {
		t.Errorf("loaded %d sheets, want 2", doc.Workbook.Len())
	}
	if doc.AppVersion != "test" {
		t.Errorf("app version = %q", doc.AppVersion)
	}
}

// TestSaveKeepsABackup: overwriting must leave the previous version recoverable.
func TestSaveKeepsABackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "budget.cell")

	first := grid.NewWorkbook()
	first.Active().Set(0, 0, cell.Cell{Source: "first", Value: cell.Text("first")})
	if err := Save(path, &Document{Workbook: first}); err != nil {
		t.Fatal(err)
	}

	second := grid.NewWorkbook()
	second.Active().Set(0, 0, cell.Cell{Source: "second", Value: cell.Text("second")})
	if err := Save(path, &Document{Workbook: second}); err != nil {
		t.Fatal(err)
	}

	doc, err := Load(path + ".bak")
	if err != nil {
		t.Fatalf("the backup is missing or unreadable: %v", err)
	}
	if got := doc.Workbook.Active().Get(0, 0).Value.Display(); got != "first" {
		t.Errorf("the backup holds %q, want the previous version", got)
	}
	// And the live file holds the new version.
	doc2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc2.Workbook.Active().Get(0, 0).Value.Display(); got != "second" {
		t.Errorf("the live file holds %q, want the new version", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.cell")); !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}

// TestSubCentCurrencySurvivesAFile is the end-to-end half of the same bug: a
// sub-cent amount must be identical after a save and a load, because the source
// text is stored rather than reconstructed from the display.
func TestSubCentCurrencySurvivesAFile(t *testing.T) {
	wb := grid.NewWorkbook()
	ref, _ := grid.ParseRef("A1")
	wb.Active().Set(ref.Row, ref.Col, cell.Cell{
		Source: "$0.0001",
		Value:  cell.Infer("$0.0001"),
	})

	data, err := Encode(&Document{Workbook: wb})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	cl := got.Workbook.Active().GetRef(ref)
	if !cl.Value.Num.Equal(cell.NewDec(1, 4)) {
		t.Errorf("value after a round trip = %s, want 0.0001", cl.Value.Num.String())
	}
	if cl.Source != "$0.0001" {
		t.Errorf("source after a round trip = %q, want the typed text", cl.Source)
	}
}

// TestWhitespaceOnlyCellRoundTrips is the bug the file fuzzer found.
//
// Typing a single space stores a cell with a source and no value. The writer
// emitted a source string for it but the reader did not consume one, so every
// record after it in the STATE section was read from the wrong offset — a
// whitespace cell silently corrupted the rest of the file.
func TestWhitespaceOnlyCellRoundTrips(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()

	blank := cell.Cell{Source: " ", Value: cell.Infer(" ")}
	if !blank.Value.IsEmpty() {
		t.Fatalf("precondition: a single space should infer as empty, got %v", blank.Value.Kind)
	}
	sh.Set(0, 0, blank)
	// Cells after it must survive, because they are what a misaligned read
	// would destroy.
	sh.Set(1, 0, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
	sh.Set(2, 0, cell.Cell{Source: "Operations", Value: cell.Infer("Operations")})
	sh.Set(3, 0, cell.Cell{Source: "=SUM(A2:A3)", Value: cell.Number(cell.FromInt64(0))})

	data, err := Encode(&Document{Workbook: wb})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("a workbook containing a whitespace cell failed to load: %v", err)
	}
	gs := got.Workbook.Active()
	if got := gs.Get(1, 0).Value.Display(); got != "$1,250.00" {
		t.Errorf("A2 = %q, want $1,250.00 (the read was misaligned)", got)
	}
	if got := gs.Get(2, 0).Value.Display(); got != "Operations" {
		t.Errorf("A3 = %q, want Operations", got)
	}
	if !gs.Get(3, 0).IsFormula() || gs.Get(3, 0).Source != "=SUM(A2:A3)" {
		t.Errorf("A4 = %+v, want the formula source", gs.Get(3, 0))
	}
	if src := gs.Get(0, 0).Source; src != " " {
		t.Errorf("the whitespace cell's source = %q, want a single space", src)
	}
}

// TestLongRunOfWhitespaceCellsRoundTrips hammers the same path, because one
// misaligned record corrupts everything downstream and a single cell might get
// lucky about where the next record starts.
func TestLongRunOfWhitespaceCellsRoundTrips(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	for r := uint32(0); r < 50; r++ {
		switch r % 3 {
		case 0:
			sh.Set(r, 0, cell.Cell{Source: "   ", Value: cell.Infer("   ")})
		case 1:
			sh.Set(r, 0, cell.Cell{Source: "$1.00", Value: cell.Infer("$1.00")})
		default:
			sh.Set(r, 0, cell.Cell{Source: "label", Value: cell.Infer("label")})
		}
	}
	data, _ := Encode(&Document{Workbook: wb})
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	gs := got.Workbook.Active()
	for r := uint32(0); r < 50; r++ {
		wantKind := cell.KindCurrency
		if r%3 == 0 {
			wantKind = cell.KindEmpty
		} else if r%3 == 2 {
			wantKind = cell.KindText
		}
		if k := gs.Get(r, 0).Value.Kind; k != wantKind {
			t.Fatalf("row %d kind = %v, want %v", r+1, k, wantKind)
		}
	}
}

// TestMinusEqualsFormulaSurvivesAsAFormula is the consequence of a bug found
// while fixing the user's report: the Cell.IsFormula method tested for a leading
// '=' itself instead of delegating, so a "-=SUM(...)" cell was recognised when
// typed but written to disk as a frozen value. It would then never recalculate.
func TestMinusEqualsFormulaSurvivesAsAFormula(t *testing.T) {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	sh.Set(0, 0, cell.Cell{Source: "$50,000.00", Value: cell.Infer("$50,000.00")})
	sh.Set(0, 1, cell.Cell{Source: "$1,250.00", Value: cell.Infer("$1,250.00")})
	sh.Set(0, 2, cell.Cell{Source: "-=SUM(A1:B1)"})

	data, err := Encode(&Document{Workbook: wb})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	cl := got.Workbook.Active().Get(0, 2)
	if !cl.IsFormula() {
		t.Fatalf("after a round trip the cell is no longer a formula: kind=%v source=%q",
			cl.Value.Kind, cl.Source)
	}
	if cl.Source != "-=SUM(A1:B1)" {
		t.Errorf("source = %q", cl.Source)
	}
	// And it must still calculate, including after a precedent changes.
	eng := calc.New(got.Workbook)
	defer eng.Close()
	eng.RecalcWorkbook(nil)
	if v := got.Workbook.Active().Get(0, 2).Value.Display(); v != "-$51,250.00" {
		t.Errorf("recalculated to %q, want -$51,250.00", v)
	}
	got.Workbook.Active().Set(0, 0, cell.Cell{Source: "$60,000.00", Value: cell.Infer("$60,000.00")})
	eng.Recalc(context.Background(), got.Workbook.Active(), []grid.Ref{{Row: 0, Col: 0}})
	if v := got.Workbook.Active().Get(0, 2).Value.Display(); v != "-$61,250.00" {
		t.Errorf("after editing A1 the formula gave %q, want -$61,250.00 (it is no longer live)", v)
	}
}
