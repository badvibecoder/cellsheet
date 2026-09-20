package sheetfile

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The container layout is specified in PHASE-1-SPEC.md §12.1.
const (
	magic       = "CELLSHEET"
	headerSize  = 64
	entrySize   = 48
	trailerSize = 32

	// FormatVersion is bumped when the byte layout changes.
	FormatVersion = 1
	// MinReaderVersion is the oldest reader that can make sense of this file.
	MinReaderVersion = 1
)

// Section identifiers. Unknown ids are preserved verbatim on rewrite, so a
// newer version of the program never loses data written by an older one.
const (
	SectionManifest uint16 = 1
	SectionState    uint16 = 2
	SectionHistory  uint16 = 3
)

// Section flag bits.
const (
	flagCompressed = 1 << 0
)

// Header is the fixed 64-byte file header.
type Header struct {
	FormatVersion    uint16
	MinReaderVersion uint16
	Flags            uint32
	Created          int64 // unix milliseconds
	Modified         int64
	SectionCount     uint32
	DirectoryOffset  uint64
}

// Section is one payload in the container.
type Section struct {
	ID      uint16
	Flags   uint16
	Data    []byte // uncompressed content
	Unknown bool   // preserved but not interpreted
}

// Errors the loader can report.
var (
	ErrNotACellFile = errors.New("not a cellsheet file")
	ErrWrongMagic   = errors.New("file does not start with the cellsheet signature")
	ErrTooNew       = errors.New("file was written by a newer version of cellsheet")
	ErrTruncated    = errors.New("file is truncated")
	ErrCorrupt      = errors.New("file failed its checksum")
	ErrMissingPart  = errors.New("file is missing a required section")
)

// encodeContainer serialises sections into the container format.
func encodeContainer(h Header, sections []Section) ([]byte, error) {
	if h.FormatVersion == 0 {
		h.FormatVersion = FormatVersion
	}
	if h.MinReaderVersion == 0 {
		h.MinReaderVersion = MinReaderVersion
	}
	h.SectionCount = uint32(len(sections))

	var body bytes.Buffer
	stored := make([][]byte, len(sections))
	flags := make([]uint16, len(sections))
	for i, s := range sections {
		data, compressed := compressSection(s.Data)
		stored[i] = data
		if compressed {
			flags[i] = flagCompressed
		} else {
			flags[i] = 0
		}
		_ = s.Flags
	}

	dirOffset := uint64(headerSize)
	payloadOffset := dirOffset + uint64(len(sections)*entrySize)
	h.DirectoryOffset = dirOffset

	// Directory.
	dir := make([]byte, 0, len(sections)*entrySize)
	offset := payloadOffset
	for i := range sections {
		e := make([]byte, entrySize)
		binary.LittleEndian.PutUint16(e[0:], sections[i].ID)
		binary.LittleEndian.PutUint16(e[2:], flags[i])
		binary.LittleEndian.PutUint64(e[4:], offset)
		binary.LittleEndian.PutUint64(e[12:], uint64(len(stored[i])))
		binary.LittleEndian.PutUint64(e[20:], uint64(len(sections[i].Data)))
		sum := sha256.Sum256(stored[i])
		copy(e[32:], sum[:16])
		dir = append(dir, e...)
		offset += uint64(len(stored[i]))
	}

	for _, s := range stored {
		body.Write(s)
	}

	// Header.
	hdr := make([]byte, headerSize)
	copy(hdr[0:], magic)
	binary.LittleEndian.PutUint16(hdr[9:], h.FormatVersion)
	binary.LittleEndian.PutUint16(hdr[11:], h.MinReaderVersion)
	binary.LittleEndian.PutUint32(hdr[13:], h.Flags)
	binary.LittleEndian.PutUint64(hdr[17:], uint64(h.Created))
	binary.LittleEndian.PutUint64(hdr[25:], uint64(h.Modified))
	binary.LittleEndian.PutUint64(hdr[33:], h.DirectoryOffset)
	binary.LittleEndian.PutUint32(hdr[41:], h.SectionCount)
	hsum := sha256.Sum256(hdr[:48])
	copy(hdr[48:], hsum[:16])

	out := make([]byte, 0, headerSize+len(dir)+body.Len()+trailerSize)
	out = append(out, hdr...)
	out = append(out, dir...)
	out = append(out, body.Bytes()...)
	whole := sha256.Sum256(out)
	out = append(out, whole[:]...)
	return out, nil
}

// decodeContainer parses the container, verifying every checksum.
func decodeContainer(data []byte) (Header, []Section, error) {
	var h Header
	if len(data) < headerSize+trailerSize {
		return h, nil, ErrTruncated
	}
	if string(data[0:len(magic)]) != magic {
		return h, nil, ErrWrongMagic
	}
	h.FormatVersion = binary.LittleEndian.Uint16(data[9:])
	h.MinReaderVersion = binary.LittleEndian.Uint16(data[11:])
	h.Flags = binary.LittleEndian.Uint32(data[13:])
	h.Created = int64(binary.LittleEndian.Uint64(data[17:]))
	h.Modified = int64(binary.LittleEndian.Uint64(data[25:]))
	h.DirectoryOffset = binary.LittleEndian.Uint64(data[33:])
	h.SectionCount = binary.LittleEndian.Uint32(data[41:])

	if h.MinReaderVersion > FormatVersion {
		return h, nil, fmt.Errorf("%w (needs reader version %d, this is %d)",
			ErrTooNew, h.MinReaderVersion, FormatVersion)
	}
	// The header protects itself against an edited version number.
	hsum := sha256.Sum256(data[:48])
	if !bytes.Equal(hsum[:16], data[48:64]) {
		return h, nil, fmt.Errorf("%w: header", ErrCorrupt)
	}
	// The trailer protects everything before it.
	whole := sha256.Sum256(data[:len(data)-trailerSize])
	if !bytes.Equal(whole[:], data[len(data)-trailerSize:]) {
		return h, nil, fmt.Errorf("%w: file trailer", ErrCorrupt)
	}

	dirStart := int(h.DirectoryOffset)
	dirEnd := dirStart + int(h.SectionCount)*entrySize
	if dirStart < headerSize || dirEnd > len(data)-trailerSize {
		return h, nil, ErrTruncated
	}

	sections := make([]Section, 0, h.SectionCount)
	for i := 0; i < int(h.SectionCount); i++ {
		e := data[dirStart+i*entrySize : dirStart+(i+1)*entrySize]
		id := binary.LittleEndian.Uint16(e[0:])
		flags := binary.LittleEndian.Uint16(e[2:])
		off := binary.LittleEndian.Uint64(e[4:])
		clen := binary.LittleEndian.Uint64(e[12:])
		ulen := binary.LittleEndian.Uint64(e[20:])

		if off+clen > uint64(len(data)-trailerSize) {
			return h, nil, fmt.Errorf("%w: section %d points past the end", ErrTruncated, id)
		}
		raw := data[off : off+clen]
		sum := sha256.Sum256(raw)
		if !bytes.Equal(sum[:16], e[32:48]) {
			return h, nil, fmt.Errorf("%w: section %d", ErrCorrupt, id)
		}
		payload := raw
		if flags&flagCompressed != 0 {
			plain, err := decompressSection(raw, int(ulen))
			if err != nil {
				return h, nil, fmt.Errorf("%w: section %d could not be decompressed", ErrCorrupt, id)
			}
			payload = plain
		}
		sections = append(sections, Section{ID: id, Flags: flags, Data: payload})
	}
	return h, sections, nil
}

// compressSection compresses a payload, reporting whether it helped. A section
// that does not shrink is stored raw: spending CPU to make a file bigger is
// never the right answer.
func compressSection(data []byte) ([]byte, bool) {
	if len(data) == 0 {
		return data, false
	}
	var buf bytes.Buffer
	zw, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return data, false
	}
	if _, err := zw.Write(data); err != nil {
		return data, false
	}
	if err := zw.Close(); err != nil {
		return data, false
	}
	if buf.Len() >= len(data) {
		return data, false
	}
	return buf.Bytes(), true
}

func decompressSection(data []byte, want int) ([]byte, error) {
	zr := flate.NewReader(bytes.NewReader(data))
	defer zr.Close()
	out := make([]byte, 0, want)
	buf := make([]byte, 32*1024)
	for {
		n, err := zr.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
			if len(out) > want {
				// A decompression bomb: refuse rather than allocate.
				return nil, ErrCorrupt
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if len(out) != want {
		return nil, ErrCorrupt
	}
	return out, nil
}
