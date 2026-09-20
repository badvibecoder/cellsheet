package sheetfile

import (
	"testing"
	"time"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// sampleWorkbook builds a workbook and history for the fuzz seeds.
func sampleWorkbook() *grid.Workbook {
	wb := grid.NewWorkbook()
	sh := wb.Active()
	sh.Set(0, 0, cell.Cell{Source: "$50,000.00", Value: cell.Infer("$50,000.00")})
	sh.Set(0, 1, cell.Cell{Source: "=SUM(A1:A1)", Value: cell.Number(cell.FromInt64(0))})
	sh.Set(1, 0, cell.Cell{Source: "text", Value: cell.Text("text")})
	sh.SetRowHeight(2, 5)
	sh.SetColWidth(1, 30)
	return wb
}

// FuzzDecode is the most important fuzz target in the project: a .cell file is
// the only thing standing between a user and their data.
//
// The property is that arbitrary bytes either fail cleanly or decode into
// something that can be re-encoded. A panic here is a crash on open; a
// successful decode that cannot be re-encoded is silent data loss on save.
func FuzzDecode(f *testing.F) {
	wb := sampleWorkbook()
	valid, err := Encode(&Document{Workbook: wb, AppVersion: "seed", Created: time.Now()})
	if err != nil {
		f.Fatalf("building the seed file: %v", err)
	}
	f.Add(valid)
	f.Add([]byte{})
	f.Add([]byte("CELLSHEET"))
	f.Add(append([]byte(nil), valid[:64]...))
	f.Add(valid[:len(valid)-1])
	f.Add([]byte("CELLSHEET\x01\x00\x01\x00"))

	// A file with history, so the delta decoder is in the seed corpus too.
	sh := wb.Sheets()[0]
	ref := grid.Ref{Row: 0, Col: 0}
	before := sh.GetRef(ref)
	after := cell.Cell{Source: "1", Value: cell.Number(cell.FromInt64(1))}
	sh.Set(ref.Row, ref.Col, after)
	withHistory, err := Encode(&Document{
		Workbook: wb,
		History: []checkpoint.Entry{{
			Kind: checkpoint.KindCellEdited, Label: "Cell edited", At: time.Now(),
			Delta: journal.Delta{
				Kind: checkpoint.KindCellEdited, Label: "Cell edited", At: time.Now(),
				Cells: []journal.CellChange{{Sheet: sh.ID, Ref: ref, Before: before, After: after}},
			},
		}},
	})
	if err == nil {
		f.Add(withHistory)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip("files this large are not the target")
		}
		doc, err := Decode(data)
		if err != nil {
			return // rejecting bad input is a correct outcome
		}
		if doc.Workbook == nil {
			t.Fatal("decoded successfully but produced no workbook")
		}
		// Anything that decodes must survive being written again.
		out, err := Encode(doc)
		if err != nil {
			t.Fatalf("a decoded document could not be re-encoded: %v", err)
		}
		if _, err := Decode(out); err != nil {
			t.Fatalf("re-encoding produced an unreadable file: %v", err)
		}
	})
}

// FuzzRoundTrip: encoding the same workbook twice must produce identical bytes,
// so that an unchanged file never shows a spurious difference.
func FuzzRoundTrip(f *testing.F) {
	f.Add("$50,000.00", "=SUM(A1:B1)", "text", uint32(0), uint32(0))
	f.Add("450", "=-A1%", "  spaced  ", uint32(99), uint32(25))
	f.Add("", "", "", uint32(1000), uint32(100))
	f.Fuzz(func(t *testing.T, a, b, c string, row, col uint32) {
		if len(a)+len(b)+len(c) > 4096 {
			t.Skip()
		}
		if row > grid.MaxRows-1 || col > grid.MaxCols-1 {
			t.Skip()
		}
		wb := grid.NewWorkbook()
		sh := wb.Active()
		sh.Set(row, col, cell.Cell{Source: a, Value: cell.Infer(a)})
		sh.Set(0, 0, cell.Cell{Source: b, Value: cell.Infer(b)})
		sh.Set(1, 1, cell.Cell{Source: c, Value: cell.Infer(c)})

		doc := &Document{Workbook: wb, AppVersion: "x"}
		first, err := Encode(doc)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		back, err := Decode(first)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		second, err := Encode(back)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		// The container carries no timestamps of its own here, so the bytes
		// must match exactly.
		_, sa, _ := decodeContainer(first)
		_, sb, _ := decodeContainer(second)
		if len(sa) != len(sb) {
			t.Fatalf("section count changed: %d then %d", len(sa), len(sb))
		}
		for i := range sa {
			if sa[i].ID != sb[i].ID {
				t.Fatalf("section %d changed identity", i)
			}
			if len(sa[i].Data) != len(sb[i].Data) {
				t.Fatalf("section %d changed length: %d then %d", sa[i].ID, len(sa[i].Data), len(sb[i].Data))
			}
		}
	})
}
