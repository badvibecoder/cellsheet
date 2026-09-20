// Package checkpoint owns the rollback history and the undo stack.
//
// The two do different jobs and neither replaces the other (PHASE-1-SPEC.md
// §13.4): undo is per-edit and lives only for the session, while checkpoints are
// coarse, labelled and written into the .cell file. Both are built from the same
// journal deltas, so neither is extra bookkeeping.
package checkpoint

import (
	"errors"
	"sync"
	"time"

	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
)

// Max is the number of rollback points kept, per the original requirement.
const Max = 99

// DefaultInterval is how often an automatic checkpoint is taken.
const DefaultInterval = 3 * time.Minute

// ErrOutOfRange is returned for an index outside 1..Max.
var ErrOutOfRange = errors.New("rollback index out of range")

// Kind is why a checkpoint exists, which is what gives it its label. It is an
// alias for the journal's kind so that the recorder and the history agree by
// construction.
type Kind = journal.Kind

const (
	KindInitial    = journal.KindInitial
	KindAuto       = journal.KindAuto
	KindCellEdited = journal.KindCellEdited
	KindRowEdited  = journal.KindRowEdited
	KindBulkPaste  = journal.KindBulkPaste
	KindStructural = journal.KindStructural
	KindManual     = journal.KindManual
	KindRollback   = journal.KindRollback
)

// Entry is one rollback point. Index 0 is always the current state.
//
// For i >= 1, the stored delta describes the edit that produced the state at
// index i-1, so ApplyReverse (the "before" side) moves from index i-1 to index
// i, and ApplyForward (the "after" side) moves back. Rolling back to index k
// therefore means applying the reverse deltas 1..k in order.
type Entry struct {
	Kind  Kind
	Label string
	At    time.Time
	Delta journal.Delta
}

// History is the ring of rollback points for one open workbook. It is safe for
// concurrent use, because the checkpoint timer runs on its own goroutine while
// the user keeps typing.
type History struct {
	mu       sync.Mutex
	entries  []Entry // newest first; entries[0] is the current state
	pending  []journal.Delta
	interval time.Duration
	last     time.Time
	dirty    bool
}

// New creates a history containing only the current state.
func New(now time.Time, interval time.Duration) *History {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &History{
		entries:  []Entry{{Kind: KindInitial, Label: "Sheet initialized", At: now}},
		interval: interval,
		last:     now,
	}
}

// Interval is how often automatic checkpoints are due.
func (h *History) Interval() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.interval
}

// Touch records that the workbook changed, which arms the timer and moves the
// current state's timestamp forward. The timer only fires when this has been
// called since the last checkpoint, which is how "only when a change is
// detected" is implemented without re-serialising anything.
func (h *History) Touch() { h.TouchAt(time.Now()) }

// TouchAt is Touch with an explicit clock, for tests.
func (h *History) TouchAt(now time.Time) {
	h.mu.Lock()
	h.dirty = true
	h.entries[0].At = now
	h.mu.Unlock()
}

// Dirty reports whether anything has changed since the last checkpoint.
func (h *History) Dirty() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dirty
}

// Due reports whether an automatic checkpoint should be taken now.
func (h *History) Due(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dirty && now.Sub(h.last) >= h.interval
}

// AddPending records an edit that has not been checkpointed yet. Pending edits
// form the first link of the rollback chain, which is what makes "roll back to
// index 1" discard uncommitted work rather than doing nothing.
func (h *History) AddPending(d journal.Delta) {
	if d.Empty() {
		return
	}
	h.mu.Lock()
	h.pending = append(h.pending, d)
	h.mu.Unlock()
}

// TakePending returns the pending edits and clears them.
func (h *History) TakePending() []journal.Delta {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.pending
	h.pending = nil
	return out
}

// PendingLen is how many uncommitted changes are waiting.
func (h *History) PendingLen() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pending)
}

// Push installs a checkpoint directly after the synthetic current-state entry,
// so index 1 is always the most recent saved state and index 0 always means
// "where we are now".
func (h *History) Push(d journal.Delta) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pushLocked(d)
}

// Mark is an alias for Push, for call sites where a checkpoint is deliberate
// rather than scheduled.
func (h *History) Mark(d journal.Delta) { h.Push(d) }

func (h *History) pushLocked(d journal.Delta) {
	e := Entry{Kind: d.Kind, Label: d.Label, At: d.At, Delta: d}
	out := make([]Entry, 0, min(len(h.entries)+1, Max+1))
	out = append(out, Entry{Kind: KindInitial, Label: "Current State", At: e.At})
	out = append(out, e)
	out = append(out, h.entries[1:]...)
	if len(out) > Max+1 { // the current state plus Max rollback steps
		out = out[:Max+1]
	}
	h.entries = out
	h.last = e.At
	h.dirty = false
}

// chainLocked builds the rollback chain: one link per rollback index, starting
// at index 1. The first link is the uncommitted work when there is any, so that
// rolling back to index 1 returns to the last checkpoint.
func (h *History) chainLocked() []Entry {
	var out []Entry
	if len(h.pending) > 0 {
		c := journal.Combine(h.pending)
		if !c.Empty() {
			out = append(out, Entry{
				Kind: journal.KindAuto, Label: "Uncommitted changes",
				At: h.entries[0].At, Delta: c,
			})
		}
	}
	out = append(out, h.entries[1:]...)
	return out
}

// Len is how many rollback steps are available. Index 0 is the current state
// and is not counted, matching the status bar's "[Rev n/99]".
func (h *History) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.chainLocked())
}

// Max returns the highest valid rollback index.
func (h *History) Max() int { return h.Len() }

// Label describes a rollback index; index 0 is the current state.
func (h *History) Label(i int) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if i == 0 {
		return "Current State"
	}
	chain := h.chainLocked()
	if i < 0 || i > len(chain) {
		return ""
	}
	return chain[i-1].Label
}

// When is the timestamp of a rollback index.
func (h *History) When(i int) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	if i == 0 {
		return h.entries[0].At
	}
	chain := h.chainLocked()
	if i < 0 || i > len(chain) {
		return time.Time{}
	}
	return chain[i-1].At
}

// Uncommitted reports whether the current state differs from what is on disk.
func (h *History) Uncommitted() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dirty
}

// MarkSaved records that everything has been written to disk.
func (h *History) MarkSaved() {
	h.mu.Lock()
	h.dirty = false
	h.mu.Unlock()
}

// NextIn is the time until the next automatic checkpoint is due.
func (h *History) NextIn() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	left := h.interval - time.Since(h.last)
	if left < 0 {
		return 0
	}
	return left
}

// Snapshot returns a copy of the entries, newest first.
func (h *History) Snapshot() []Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Entry(nil), h.entries...)
}

// Restore replaces the whole history, used after loading a file.
func (h *History) Restore(entries []Entry, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(entries) == 0 {
		h.entries = []Entry{{Kind: KindInitial, Label: "Current State", At: now}}
	} else {
		h.entries = append([]Entry(nil), entries...)
	}
	h.last = now
	h.dirty = false
	h.pending = nil
}

// Truncate keeps only the current state, used when a new workbook replaces the
// old one.
func (h *History) Truncate(now time.Time, label string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = []Entry{{Kind: KindInitial, Label: label, At: now}}
	h.last = now
	h.dirty = false
}

// Rollback moves the workbook to the state at rollback index k.
//
// The pre-rollback state is preserved as the new index 1 (decision D13), so a
// rollback can itself be rolled back: losing work to a mis-click is not
// acceptable. It is implemented by snapshotting the whole workbook before and
// after, which stays obviously correct even when the rollback adds or removes
// sheets.
//
// The new entry is *prepended* to the existing chain rather than replacing part
// of it, so no history is ever discarded by a rollback. The practical effect is
// that after rolling back to index k, indexes 2 upwards replay the timeline you
// came from, one state at a time — which is exactly what makes "roll back, look
// around, then roll forward again" safe.
func (h *History) Rollback(wb *grid.Workbook, k int, now time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	chain := h.chainLocked()
	if k < 1 || k > len(chain) {
		return ErrOutOfRange
	}

	before := journal.CaptureWorkbook(wb)
	for i := 0; i < k; i++ {
		// Reverse walks backwards through the chain: index 0 is the present.
		chain[i].Delta.ApplyReverse(wb)
	}
	after := journal.CaptureWorkbook(wb)

	entry := Entry{
		Kind:  KindRollback,
		Label: "Rolled back to index " + itoa(k),
		At:    now,
		Delta: journal.Delta{
			Kind:           KindRollback,
			Label:          "Rolled back to index " + itoa(k),
			At:             now,
			BeforeWorkbook: &before,
			AfterWorkbook:  &after,
		},
	}

	// The links the rollback did not consume remain as history behind the new
	// current state, so a rollback never discards older checkpoints.
	kept := append([]Entry{entry}, chain[k:]...)
	h.pending = nil
	h.entries = append([]Entry{{Kind: KindInitial, Label: "Current State", At: now}}, kept...)
	if len(h.entries) > Max+1 {
		h.entries = h.entries[:Max+1]
	}
	h.last = now
	h.dirty = true
	return nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
