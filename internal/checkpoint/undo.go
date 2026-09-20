package checkpoint

import (
	"sync"

	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// DefaultUndoLimit bounds the in-memory undo stack. It is generous because a
// delta for one cell edit is a few dozen bytes.
const DefaultUndoLimit = 10_000

// UndoStack is the session's per-edit undo history (decision D3): every
// individual change is recorded, so undo is never coarser than one keystroke.
// It is deliberately separate from the checkpoint ring, which is coarse and
// persisted.
type UndoStack struct {
	mu     sync.Mutex
	deltas []journal.Delta
	pos    int // how many deltas have been applied; deltas[pos-1] is the newest
	limit  int
}

// NewUndoStack creates an empty stack.
func NewUndoStack(limit int) *UndoStack {
	if limit <= 0 {
		limit = DefaultUndoLimit
	}
	return &UndoStack{limit: limit}
}

// Push records a delta that has just been applied.
func (u *UndoStack) Push(d journal.Delta) {
	if d.Empty() {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	// Anything that had been undone is discarded: the user has taken a new
	// branch, and keeping the old one would make redo lie.
	if u.pos < len(u.deltas) {
		u.deltas = u.deltas[:u.pos]
	}
	u.deltas = append(u.deltas, d)
	if len(u.deltas) > u.limit {
		drop := len(u.deltas) - u.limit
		u.deltas = append([]journal.Delta(nil), u.deltas[drop:]...)
		u.pos -= drop
	}
	u.pos = len(u.deltas)
}

// CanUndo reports whether there is anything to undo.
func (u *UndoStack) CanUndo() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.pos > 0
}

// CanRedo reports whether there is anything to redo.
func (u *UndoStack) CanRedo() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.pos < len(u.deltas)
}

// Undo reverses the most recent change and returns its delta.
func (u *UndoStack) Undo(wb *grid.Workbook) (journal.Delta, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.pos == 0 {
		return journal.Delta{}, false
	}
	u.pos--
	d := u.deltas[u.pos]
	d.ApplyReverse(wb)
	return d, true
}

// Redo reapplies the most recently undone change.
func (u *UndoStack) Redo(wb *grid.Workbook) (journal.Delta, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.pos >= len(u.deltas) {
		return journal.Delta{}, false
	}
	d := u.deltas[u.pos]
	d.ApplyForward(wb)
	u.pos++
	return d, true
}

// Len is how many changes can be undone.
func (u *UndoStack) Len() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.pos
}

// Reset empties the stack, used when a different workbook is opened.
func (u *UndoStack) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.deltas = nil
	u.pos = 0
}
