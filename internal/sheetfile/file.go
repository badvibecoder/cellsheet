// Package sheetfile reads and writes .cell workbooks.
//
// A .cell file is standalone: it contains the current workbook and its rollback
// history, checksummed, in a single container. The layout is specified in
// PHASE-1-SPEC.md §12 and the encoders live beside this file.
package sheetfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
)

// Ext is the workbook file extension.
const Ext = ".cell"

// Document is everything a .cell file holds.
type Document struct {
	Workbook   *grid.Workbook
	History    []checkpoint.Entry
	Created    time.Time
	Modified   time.Time
	AppVersion string
	Cursor     grid.Ref
	Active     int

	// Unknown carries sections written by a newer version of the program, so
	// that saving never destroys data this version does not understand.
	Unknown []Section
}

type manifestSheet struct {
	Name             string `json:"name"`
	Rows             uint32 `json:"rows"`
	Cols             uint32 `json:"cols"`
	DefaultRowHeight int    `json:"defaultRowHeight"`
	DefaultColWidth  int    `json:"defaultColWidth"`
}

type manifestCursor struct {
	Sheet int `json:"sheet"`
	Row   int `json:"row"`
	Col   int `json:"col"`
}

type manifestHistory struct {
	Count     int    `json:"count"`
	HeadLabel string `json:"headLabel"`
}

type manifest struct {
	FormatVersion    int             `json:"formatVersion"`
	MinReaderVersion int             `json:"minReaderVersion"`
	AppVersion       string          `json:"appVersion"`
	Created          string          `json:"created"`
	Modified         string          `json:"modified"`
	ActiveSheet      int             `json:"activeSheet"`
	Cursor           manifestCursor  `json:"cursor"`
	Sheets           []manifestSheet `json:"sheets"`
	History          manifestHistory `json:"history"`
	Scratch          map[string]any  `json:"scratch"`
}

// Encode serialises a document into the container format.
func Encode(doc *Document) ([]byte, error) {
	if doc.Workbook == nil {
		return nil, errors.New("sheetfile: nothing to save")
	}
	now := time.Now()
	if doc.Modified.IsZero() {
		doc.Modified = now
	}
	if doc.Created.IsZero() {
		doc.Created = doc.Modified
	}

	m := manifest{
		FormatVersion:    FormatVersion,
		MinReaderVersion: MinReaderVersion,
		AppVersion:       doc.AppVersion,
		Created:          doc.Created.Format(time.RFC3339),
		Modified:         doc.Modified.Format(time.RFC3339),
		ActiveSheet:      doc.Workbook.ActiveIndex(),
		Cursor: manifestCursor{
			Sheet: doc.Workbook.ActiveIndex(),
			Row:   int(doc.Cursor.Row),
			Col:   int(doc.Cursor.Col),
		},
		Scratch: map[string]any{},
	}
	for _, sh := range doc.Workbook.Sheets() {
		m.Sheets = append(m.Sheets, manifestSheet{
			Name:             sh.Name,
			Rows:             sh.Rows(),
			Cols:             sh.Cols(),
			DefaultRowHeight: sh.DefaultRowHeight(),
			DefaultColWidth:  sh.DefaultColWidth(),
		})
	}
	m.History.Count = len(doc.History)
	if len(doc.History) > 0 {
		m.History.HeadLabel = doc.History[0].Label
	}

	mb, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	sections := []Section{
		{ID: SectionManifest, Data: mb},
		{ID: SectionState, Data: encodeState(doc.Workbook)},
		{ID: SectionHistory, Data: encodeHistory(doc.History)},
	}
	sections = append(sections, doc.Unknown...)

	hdr := Header{
		FormatVersion:    FormatVersion,
		MinReaderVersion: MinReaderVersion,
		Created:          doc.Created.UnixMilli(),
		Modified:         doc.Modified.UnixMilli(),
	}
	return encodeContainer(hdr, sections)
}

// Decode parses a container back into a document.
func Decode(data []byte) (*Document, error) {
	hdr, sections, err := decodeContainer(data)
	if err != nil {
		return nil, err
	}
	doc := &Document{
		Created:  timeFromMillis(hdr.Created),
		Modified: timeFromMillis(hdr.Modified),
	}
	var haveState bool
	for _, s := range sections {
		switch s.ID {
		case SectionManifest:
			var m manifest
			if err := json.Unmarshal(s.Data, &m); err == nil {
				doc.AppVersion = m.AppVersion
				doc.Active = m.ActiveSheet
				doc.Cursor = grid.Ref{Row: uint32(max(m.Cursor.Row, 0)), Col: uint32(max(m.Cursor.Col, 0))}
			}
		case SectionState:
			wb, err := decodeState(s.Data)
			if err != nil {
				return nil, fmt.Errorf("%w: STATE section: %v", ErrCorrupt, err)
			}
			doc.Workbook = wb
			haveState = true
		case SectionHistory:
			doc.History = decodeHistory(s.Data)
		default:
			doc.Unknown = append(doc.Unknown, s)
		}
	}
	if !haveState || doc.Workbook == nil {
		return nil, fmt.Errorf("%w: STATE section", ErrMissingPart)
	}
	doc.Workbook.SetActive(doc.Active)
	return doc, nil
}

// Save writes a workbook to disk atomically.
//
// The file is written to a temporary file in the same directory, flushed to
// disk, and then renamed over the target. A crash at any point leaves either
// the old file or the new one, never a half-written mixture. The previous
// version is kept alongside as .bak.
func Save(path string, doc *Document) error {
	data, err := Encode(doc)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".cellsheet-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// If anything below fails, do not leave litter behind.
		if _, err := os.Stat(tmpName); err == nil {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Flush the contents before the rename, so the rename can never point at
	// an incomplete file.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		// Keep one previous version. A failure here is not fatal: the save
		// itself is what matters.
		_ = copyFile(path, path+".bak")
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Flush the directory entry so the rename survives a power loss.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Load reads a workbook from disk.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := Decode(data)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
