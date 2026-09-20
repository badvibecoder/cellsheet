// Package clipboard is the internal copy buffer.
//
// Decision D6: there is no system-clipboard integration. Copy and paste between
// cells always work, formulas and types are preserved, and nothing external is
// required. Pasting text *from* another application arrives through the
// terminal's own paste, which is parsed as tab-separated values.
package clipboard

import (
	"strings"

	"github.com/badvibecoder/cellsheet/internal/cell"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

// Buffer is a rectangular block of cells.
type Buffer struct {
	W, H int

	cells    []cell.Cell
	cut      bool
	sheet    uint32
	origin   grid.Ref
	fromText bool
}

// Empty reports whether the buffer holds nothing.
func (b *Buffer) Empty() bool { return b == nil || b.W == 0 || b.H == 0 }

// IsCut reports whether the contents were cut rather than copied, meaning the
// source should be cleared once they are pasted.
func (b *Buffer) IsCut() bool { return b != nil && b.cut }

// FromText reports whether the contents came from pasted text rather than from
// cells, which is worth knowing because such a paste carries values only.
func (b *Buffer) FromText() bool { return b != nil && b.fromText }

// SourceSheet and Origin identify where a cut came from.
func (b *Buffer) SourceSheet() uint32 { return b.sheet }
func (b *Buffer) Origin() grid.Ref    { return b.origin }

// At returns the cell at a buffer-relative position.
func (b *Buffer) At(r, c int) cell.Cell {
	if b == nil || r < 0 || c < 0 || r >= b.H || c >= b.W {
		return cell.Cell{}
	}
	return b.cells[r*b.W+c]
}

func newBuffer(w, h int) *Buffer {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Buffer{W: w, H: h, cells: make([]cell.Cell, w*h)}
}

// Copy captures a rectangle of cells. The source text is reconstructed where it
// is missing so that the block can be pasted without depending on the original
// sheet still existing.
func Copy(sh *grid.Sheet, r0, c0, r1, c1 int) *Buffer {
	b := newBuffer(c1-c0+1, r1-r0+1)
	b.sheet = sh.ID
	b.origin = grid.Ref{Row: uint32(r0), Col: uint32(c0)}
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			cl := sh.Get(uint32(r), uint32(c))
			cl = withSource(cl)
			b.cells[(r-r0)*b.W+(c-c0)] = cl
		}
	}
	return b
}

// Cut is Copy, marked so that the source is cleared when it is pasted.
func Cut(sh *grid.Sheet, r0, c0, r1, c1 int) *Buffer {
	b := Copy(sh, r0, c0, r1, c1)
	b.cut = true
	return b
}

// withSource leaves a cell exactly as it is. A paste writes the stored value
// directly rather than re-reading it from text, so nothing needs reconstructing
// — and reconstructing from the display would be lossy for currency.
func withSource(cl cell.Cell) cell.Cell { return cl }

// FromTSV parses tab-separated text, as produced by another spreadsheet or by
// copying a table out of a document. Rows are separated by newlines and cells
// by tabs; a trailing newline is ignored.
func FromTSV(text string) *Buffer {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return &Buffer{fromText: true}
	}
	lines := strings.Split(text, "\n")
	h := len(lines)
	w := 0
	grid := make([][]string, h)
	for i, line := range lines {
		fields := strings.Split(line, "\t")
		grid[i] = fields
		if len(fields) > w {
			w = len(fields)
		}
	}
	b := newBuffer(w, h)
	b.fromText = true
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			if c >= len(grid[r]) {
				continue // a ragged line leaves the remaining cells empty
			}
			text := grid[r][c]
			if text == "" {
				continue
			}
			b.cells[r*w+c] = cell.Cell{Source: text, Value: cell.Infer(text)}
		}
	}
	return b
}

// ToTSV renders a buffer as tab-separated text, which is the form another
// application expects.
func ToTSV(b *Buffer) string {
	if b.Empty() {
		return ""
	}
	var sb strings.Builder
	for r := 0; r < b.H; r++ {
		if r > 0 {
			sb.WriteByte('\n')
		}
		for c := 0; c < b.W; c++ {
			if c > 0 {
				sb.WriteByte('\t')
			}
			sb.WriteString(b.At(r, c).Value.Display())
		}
	}
	return sb.String()
}

// Size is the buffer's dimensions.
func (b *Buffer) Size() (w, h int) {
	if b == nil {
		return 0, 0
	}
	return b.W, b.H
}
