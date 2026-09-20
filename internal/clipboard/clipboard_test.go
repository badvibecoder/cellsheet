package clipboard

import (
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

func sheetWith(rows ...[]string) *grid.Sheet {
	sh := grid.NewSheet(1, "S")
	for r, row := range rows {
		for c, text := range row {
			sh.Set(uint32(r), uint32(c), cell.Cell{Source: text, Value: cell.Infer(text)})
		}
	}
	return sh
}

func TestCopyCapturesAKindPreservingBlock(t *testing.T) {
	sh := sheetWith(
		[]string{"$50,000.00", "450"},
		[]string{"Operations", "=SUM(A1:B1)"},
	)
	b := Copy(sh, 0, 0, 1, 1)
	if b.W != 2 || b.H != 2 {
		t.Fatalf("buffer is %dx%d, want 2x2", b.W, b.H)
	}
	if got := b.At(0, 0).Value.Display(); got != "$50,000.00" {
		t.Errorf("A1 = %q", got)
	}
	if got := b.At(0, 1).Value.Kind; got != cell.KindNumber {
		t.Errorf("B1 kind = %v, want Number", got)
	}
	if !b.At(1, 1).IsFormula() {
		t.Error("the formula should be preserved as a formula")
	}
	if b.IsCut() {
		t.Error("a copy must not be marked as a cut")
	}
}

func TestCutIsFlagged(t *testing.T) {
	sh := sheetWith([]string{"a"})
	b := Cut(sh, 0, 0, 0, 0)
	if !b.IsCut() {
		t.Error("a cut must be flagged so the source is cleared after pasting")
	}
}

// TestCopyDoesNotInventASource is the fix for the precision bug the fuzzer
// found: the display form is lossy for currency, so a copy must carry the
// stored value and must never fabricate a source from the display.
func TestCopyDoesNotInventASource(t *testing.T) {
	sh := grid.NewSheet(1, "S")
	sh.Set(0, 0, cell.Cell{Value: cell.Currency(cell.NewDec(1, 4))}) // $0.0001
	b := Copy(sh, 0, 0, 0, 0)
	if got := b.At(0, 0).Source; got != "" {
		t.Errorf("source = %q; the display %q is lossy and must not become the source",
			got, sh.Get(0, 0).Value.Display())
	}
	if !b.At(0, 0).Value.Num.Equal(cell.NewDec(1, 4)) {
		t.Errorf("the copied value lost precision: %s", b.At(0, 0).Value.Num.String())
	}
}

func TestFromTSV(t *testing.T) {
	b := FromTSV("a\tb\tc\n1\t2.5\t$300\n")
	if b.W != 3 || b.H != 2 {
		t.Fatalf("buffer is %dx%d, want 3x2", b.W, b.H)
	}
	if got := b.At(0, 0).Value.Str; got != "a" {
		t.Errorf("(0,0) = %q", got)
	}
	if got := b.At(1, 1).Value.Display(); got != "2.5" {
		t.Errorf("(1,1) = %q", got)
	}
	if got := b.At(1, 2).Value.Kind; got != cell.KindCurrency {
		t.Errorf("(1,2) kind = %v, want Currency", got)
	}
	if !b.FromText() {
		t.Error("a text paste should be marked as such")
	}
}

func TestFromTSVRaggedLines(t *testing.T) {
	b := FromTSV("a\tb\tc\nd")
	if b.W != 3 || b.H != 2 {
		t.Fatalf("buffer is %dx%d, want 3x2", b.W, b.H)
	}
	if !b.At(1, 2).Value.IsEmpty() {
		t.Error("a short line should leave the remaining cells empty")
	}
}

func TestFromTSVNormalisesLineEndings(t *testing.T) {
	b := FromTSV("a\r\nb\r\n")
	if b.H != 2 {
		t.Fatalf("h = %d, want 2", b.H)
	}
}

func TestFromTSVEmpty(t *testing.T) {
	if b := FromTSV(""); !b.Empty() {
		t.Error("an empty paste should produce an empty buffer")
	}
	if b := FromTSV("\n"); !b.Empty() {
		t.Error("a single newline should produce an empty buffer")
	}
}

func TestToTSV(t *testing.T) {
	sh := sheetWith(
		[]string{"$50,000.00", "Operations"},
		[]string{"450", ""},
	)
	b := Copy(sh, 0, 0, 1, 1)
	got := ToTSV(b)
	want := "$50,000.00\tOperations\n450\t"
	if got != want {
		t.Errorf("ToTSV = %q, want %q", got, want)
	}
}

func TestRoundTripThroughTSV(t *testing.T) {
	sh := sheetWith(
		[]string{"$1,250.00", "12.75"},
		[]string{"a label", "-3"},
	)
	b := Copy(sh, 0, 0, 1, 1)
	again := FromTSV(ToTSV(b))
	if again.W != 2 || again.H != 2 {
		t.Fatalf("round trip produced %dx%d", again.W, again.H)
	}
	for r := 0; r < 2; r++ {
		for c := 0; c < 2; c++ {
			if a, b2 := b.At(r, c).Value.Display(), again.At(r, c).Value.Display(); a != b2 {
				t.Errorf("(%d,%d) round trip: %q -> %q", r, c, a, b2)
			}
		}
	}
}

func TestEmptyBufferAccessIsSafe(t *testing.T) {
	var b *Buffer
	if !b.Empty() {
		t.Error("a nil buffer is empty")
	}
	if got := b.At(0, 0); !got.Value.IsEmpty() {
		t.Error("reading a nil buffer should give an empty cell")
	}
	if b.IsCut() || b.FromText() {
		t.Error("a nil buffer is neither a cut nor text")
	}
}

func TestOutOfRangeReadsAreEmpty(t *testing.T) {
	sh := sheetWith([]string{"a", "b"})
	b := Copy(sh, 0, 0, 0, 1)
	if !b.At(5, 5).Value.IsEmpty() {
		t.Error("an out-of-range read should give an empty cell")
	}
	if !b.At(-1, 0).Value.IsEmpty() {
		t.Error("a negative read should give an empty cell")
	}
}
