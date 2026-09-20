package clipboard

import (
	"strings"
	"testing"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

func cellFor(typed string) cell.Cell {
	return cell.Cell{Source: typed, Value: cell.Infer(typed)}
}

// FuzzFromTSV: pasted text comes from other applications and from the
// clipboard, so it is as untrusted as any network input. Parsing must never
// panic, and the buffer must always be internally consistent.
func FuzzFromTSV(f *testing.F) {
	seeds := []string{
		"",
		"\n",
		"\t",
		"a\tb\tc\n1\t2\t3\n",
		"a\r\nb\r\n",
		"$50,000.00\tOperations\n12.75\tYES\n",
		"a\t\tc",
		"\t\t\t",
		"one line",
		strings.Repeat("x\t", 100) + "y",
		strings.Repeat("row\n", 100),
		"\x00\t\x01",
		"\"quoted\"\t'quoted'",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 1<<16 {
			t.Skip("a paste this large is handled by the chunked path, not here")
		}
		b := FromTSV(text)
		if b.Empty() {
			return
		}
		// Every advertised cell must be readable.
		for r := 0; r < b.H; r++ {
			for c := 0; c < b.W; c++ {
				cl := b.At(r, c)
				_ = cl.Value.Display()
				_ = cl.Source
			}
		}
		// Reading outside the buffer is always empty, never a panic.
		if !b.At(-1, -1).Value.IsEmpty() || !b.At(b.H, b.W).Value.IsEmpty() {
			t.Fatal("out-of-range reads must be empty")
		}
		// The dimensions must agree with the content actually stored.
		if b.W <= 0 || b.H <= 0 {
			t.Fatalf("a non-empty buffer reported %dx%d", b.W, b.H)
		}
		// The row and column counts must match the normalised text exactly:
		// both CRLF and a lone CR are line breaks, so the check is against the
		// normalised form rather than against the raw bytes.
		normalised := strings.ReplaceAll(text, "\r\n", "\n")
		normalised = strings.ReplaceAll(normalised, "\r", "\n")
		normalised = strings.TrimSuffix(normalised, "\n")
		if normalised != "" {
			wantRows := len(strings.Split(normalised, "\n"))
			if b.H != wantRows {
				t.Fatalf("text with %d normalised lines produced %d rows", wantRows, b.H)
			}
			wantCols := 0
			for _, line := range strings.Split(normalised, "\n") {
				if n := len(strings.Split(line, "\t")); n > wantCols {
					wantCols = n
				}
			}
			if b.W != wantCols {
				t.Fatalf("text with %d columns produced a width of %d", wantCols, b.W)
			}
		}
	})
}

// FuzzCopyRoundTrip: a block copied and pasted without any text conversion must
// be identical, because that is the ordinary copy/paste a user does all day.
func FuzzCopyRoundTrip(f *testing.F) {
	f.Add("$50,000.00", "Operations", "12.75", uint32(0), uint32(0))
	f.Add("", "  spaced  ", "-3", uint32(5), uint32(2))
	f.Fuzz(func(t *testing.T, a, b, c string, row, col uint32) {
		if len(a)+len(b)+len(c) > 1024 {
			t.Skip()
		}
		if row > 90 || col > 20 {
			t.Skip()
		}
		sh := grid.NewSheet(1, "S")
		sh.Set(row, col, cellFor(a))
		sh.Set(row+1, col, cellFor(b))
		sh.Set(row, col+1, cellFor(c))

		buf := Copy(sh, int(row), int(col), int(row)+1, int(col)+1)
		dst := grid.NewSheet(2, "D")
		for r := 0; r < buf.H; r++ {
			for c := 0; c < buf.W; c++ {
				dst.Set(uint32(r), uint32(c), buf.At(r, c))
			}
		}
		for r := 0; r < buf.H; r++ {
			for c := 0; c < buf.W; c++ {
				src := sh.Get(uint32(int(row)+r), uint32(int(col)+c))
				got := dst.Get(uint32(r), uint32(c))
				if src.Value.Kind != got.Value.Kind {
					t.Fatalf("kind changed at (%d,%d): %v -> %v", r, c, src.Value.Kind, got.Value.Kind)
				}
				if src.Value.Display() != got.Value.Display() {
					t.Fatalf("value changed at (%d,%d): %q -> %q", r, c, src.Value.Display(), got.Value.Display())
				}
				if src.Source != got.Source {
					t.Fatalf("source changed at (%d,%d): %q -> %q", r, c, src.Source, got.Source)
				}
			}
		}
	})
}
