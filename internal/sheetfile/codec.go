package sheetfile

import (
	"encoding/binary"
	"errors"
)

// errShort is returned when a section ends in the middle of a value, which is
// how a truncated or corrupt payload is detected.
var errShort = errors.New("section ended unexpectedly")

// writer builds a byte payload with length-prefixed fields.
type writer struct{ buf []byte }

func (w *writer) byte(b byte) { w.buf = append(w.buf, b) }

func (w *writer) uvarint(v uint64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], v)
	w.buf = append(w.buf, tmp[:n]...)
}

// varint writes a signed value using zigzag so small negatives stay small.
func (w *writer) varint(v int64) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutVarint(tmp[:], v)
	w.buf = append(w.buf, tmp[:n]...)
}

func (w *writer) str(s string) {
	w.uvarint(uint64(len(s)))
	w.buf = append(w.buf, s...)
}

func (w *writer) bytes(b []byte) {
	w.uvarint(uint64(len(b)))
	w.buf = append(w.buf, b...)
}

// reader reads back what writer produced. Once set, err short-circuits every
// later read, so callers can check once at the end.
type reader struct {
	buf []byte
	pos int
	err error
}

func (r *reader) fail() {
	if r.err == nil {
		r.err = errShort
	}
}

func (r *reader) byte() byte {
	if r.err != nil || r.pos >= len(r.buf) {
		r.fail()
		return 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *reader) uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.buf[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *reader) varint() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.buf[r.pos:])
	if n <= 0 {
		r.fail()
		return 0
	}
	r.pos += n
	return v
}

func (r *reader) str() string {
	n := r.uvarint()
	if r.err != nil {
		return ""
	}
	if n > uint64(len(r.buf)-r.pos) {
		r.fail()
		return ""
	}
	s := string(r.buf[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s
}

func (r *reader) bytes() []byte {
	n := r.uvarint()
	if r.err != nil {
		return nil
	}
	if n > uint64(len(r.buf)-r.pos) {
		r.fail()
		return nil
	}
	b := r.buf[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b
}

// remaining is how many bytes are left unread.
func (r *reader) remaining() int { return len(r.buf) - r.pos }
