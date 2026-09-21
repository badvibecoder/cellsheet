// Package app is the composition root: it owns the workbook, the calculation
// engine, the history and the terminal program, and wires them together.
// Nothing else in the program constructs another component.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/badvibecoder/cellsheet/internal/calc"
	"github.com/badvibecoder/cellsheet/internal/checkpoint"
	"github.com/badvibecoder/cellsheet/internal/grid"
	"github.com/badvibecoder/cellsheet/internal/journal"
	"github.com/badvibecoder/cellsheet/internal/sheetfile"
	"github.com/badvibecoder/cellsheet/internal/tui/render"
	"github.com/badvibecoder/cellsheet/internal/tui/screens"
	"github.com/badvibecoder/cellsheet/internal/tui/theme"
)

// Version is the application version recorded in every workbook's manifest.
const Version = "0.1.0"

// Options configures a run.
type Options struct {
	// Path is the workbook to open; empty starts a new one.
	Path string
	// Color is "auto", "none", "16", "256" or "true".
	Color string
	// IsTerminal says whether stdout is a terminal, used by "auto".
	IsTerminal bool
}

// App is the Bubble Tea model for the whole program.
type App struct {
	Screen *screens.Model
	Eng    *calc.Engine
	Wb     *grid.Workbook
	Hist   *checkpoint.History
	Undo   *checkpoint.UndoStack
	Rec    *journal.Recorder

	path    string
	created time.Time
}

type tickMsg time.Time

// New builds an application. If opts.Path names an existing workbook it is
// loaded; otherwise a new one is started.
func New(opts Options) (*App, error) {
	a := &App{
		Undo: checkpoint.NewUndoStack(checkpoint.DefaultUndoLimit),
		Rec:  journal.NewRecorder(),
	}

	started := time.Now()
	if opts.Path != "" {
		doc, err := sheetfile.Load(opts.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
			// A path that does not exist yet is a new workbook to be saved
			// there, which is what a user typing "cellsheet budget.cell"
			// almost always means.
			started = time.Now()
			a.path = opts.Path
		} else {
			a.Wb = doc.Workbook
			a.path = opts.Path
			a.created = doc.Created
			a.Hist = checkpoint.New(time.Now(), checkpoint.DefaultInterval)
			a.Hist.Restore(doc.History, time.Now())
		}
	}
	if a.Wb == nil {
		a.Wb = grid.NewWorkbook()
		a.created = started
		a.Hist = checkpoint.New(started, checkpoint.DefaultInterval)
		a.Hist.Truncate(started, "Sheet initialized")
	}

	a.Eng = calc.New(a.Wb)
	// Formulas are stored as source, so a freshly loaded workbook must be
	// re-indexed and recalculated before it is shown.
	a.Eng.RecalcWorkbook(nil)

	a.Screen = screens.New(a.Wb, a.Eng, theme.Dark(),
		render.ParseColorMode(opts.Color, opts.IsTerminal), a.Hist)
	a.Screen.Path = a.path
	a.Screen.Rec = a.Rec
	a.Screen.Undo = a.Undo
	a.Screen.OnEdit = a.onEdit
	a.Screen.OnDirty = a.onDirty
	a.Screen.Dirty = a.Hist.Uncommitted()
	return a, nil
}

// onEdit is called after a recorded mutation: build the delta, push it onto the
// session undo stack, and decide whether it also deserves a checkpoint.
func (a *App) onEdit(kind journal.Kind, label string) {
	now := time.Now()

	switch kind {
	case journal.KindAuto, journal.KindManual:
		// A deliberate checkpoint covers everything since the last one, so it
		// folds the uncommitted edits together with anything just recorded.
		parts := a.Hist.TakePending()
		if d := a.Rec.Build(a.Wb, kind, label, now); !d.Empty() {
			parts = append(parts, d)
		}
		d := journal.Combine(parts)
		if d.Empty() {
			a.Screen.SetStatus(false, "Nothing new to check point")
			return
		}
		d.Kind, d.Label, d.At = kind, label, now
		a.Hist.Push(d)
		a.Screen.Dirty = true
		a.Screen.SetStatus(false, "Checkpoint taken")
		return
	}

	d := a.Rec.Build(a.Wb, kind, label, now)
	if d.Empty() {
		return
	}
	a.Undo.Push(d)
	a.Hist.Touch()

	switch kind {
	case journal.KindBulkPaste, journal.KindStructural:
		// Flush anything uncommitted first, so the notable checkpoint starts
		// from a clean baseline and can be reversed on its own.
		if parts := a.Hist.TakePending(); len(parts) > 0 {
			if c := journal.Combine(parts); !c.Empty() {
				c.Kind, c.Label, c.At = journal.KindAuto, "Auto-Checkpoint", now
				a.Hist.Push(c)
			}
		}
		a.Hist.Push(d)
	default:
		// Ordinary edits accumulate until the timer fires.
		a.Hist.AddPending(d)
	}
	a.Screen.Dirty = true
}

// onDirty is called after a change that was not recorded, such as undo, redo or
// rollback.
func (a *App) onDirty() {
	a.Hist.Touch()
	a.Screen.Dirty = true
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd { return tickCmd() }

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update implements tea.Model. It delegates to the screen and then acts on the
// requests the screen raised, which keeps file handling out of the interface.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tickMsg); ok {
		a.onTick(time.Time(m))
		return a, tickCmd()
	}
	_, cmd := a.Screen.Update(msg)
	return a, tea.Batch(cmd, a.react())
}

func (a *App) onTick(now time.Time) {
	if a.Hist.Due(now) {
		a.onEdit(journal.KindAuto, "Auto-Checkpoint")
	}
	a.Screen.Dirty = a.Hist.Uncommitted()
}

// react turns the screen's requests into application actions.
func (a *App) react() tea.Cmd {
	s := a.Screen
	if s.Quit {
		return tea.Quit
	}
	if s.NewRequested {
		s.NewRequested = false
		a.newWorkbook()
	}
	if s.OpenRequested {
		s.OpenRequested = false
		path := s.PendingPath
		s.PendingPath = ""
		if path != "" {
			if err := a.open(path); err != nil {
				s.SetStatus(true, "Open failed: %v", err)
			}
		}
	}
	if s.SaveRequested {
		s.SaveRequested = false
		path := a.path
		if s.SaveAs || path == "" {
			s.SaveAs = false
			path = s.PendingPath
			s.PendingPath = ""
		}
		if path == "" {
			s.SetStatus(true, "No file name")
			return nil
		}
		if err := a.save(ensureCellExt(path)); err != nil {
			s.SetStatus(true, "Save failed: %v", err)
			return nil
		}
	}
	return nil
}

func (a *App) save(path string) error {
	doc := &sheetfile.Document{
		Workbook:   a.Wb,
		History:    a.Hist.Snapshot(),
		Created:    a.created,
		Modified:   time.Now(),
		AppVersion: Version,
		Cursor:     a.Screen.Cur,
	}
	if err := sheetfile.Save(path, doc); err != nil {
		return err
	}
	a.path = path
	a.Screen.Path = path
	a.Hist.MarkSaved()
	a.Screen.Dirty = false
	a.Screen.SetStatus(false, "Saved %s", filepath.Base(path))
	return nil
}

func (a *App) open(path string) error {
	doc, err := sheetfile.Load(path)
	if err != nil {
		return err
	}
	a.adopt(doc.Workbook, doc.Created, doc.History)
	a.path = path
	a.Screen.Path = path
	a.Screen.SetStatus(false, "Opened %s", filepath.Base(path))
	return nil
}

func (a *App) newWorkbook() {
	now := time.Now()
	a.adopt(grid.NewWorkbook(), now, nil)
	a.path = ""
	a.Screen.Path = ""
	a.Screen.SetStatus(false, "New workbook")
}

// adopt swaps in a different workbook, rebuilding everything that depends on it.
func (a *App) adopt(wb *grid.Workbook, created time.Time, history []checkpoint.Entry) {
	a.Eng.Close()
	a.Wb = wb
	a.Eng = calc.New(wb)
	a.Eng.RecalcWorkbook(nil)
	a.created = created
	a.Undo.Reset()
	a.Rec = journal.NewRecorder()

	now := time.Now()
	a.Hist = checkpoint.New(now, checkpoint.DefaultInterval)
	if len(history) > 0 {
		a.Hist.Restore(history, now)
	} else {
		a.Hist.Truncate(created, "Sheet initialized")
	}

	s := a.Screen
	s.Wb = wb
	s.Eng = a.Eng
	s.History = a.Hist
	s.Rec = a.Rec
	s.Cur = grid.Ref{}
	s.Anchor = grid.Ref{}
	s.HasSel = false
	s.TopRow, s.LeftCol = 0, 0
	s.Dirty = false
}

// ensureCellExt appends the workbook extension when a path has none, so that
// "cellsheet budget" and a bare name typed at the prompt both produce
// budget.cell rather than a file the program will not open later.
func ensureCellExt(path string) string {
	if filepath.Ext(path) == "" {
		return path + sheetfile.Ext
	}
	return path
}

// View implements tea.Model.
func (a *App) View() string { return a.Screen.View() }

// Close releases the worker pool.
func (a *App) Close() { a.Eng.Close() }

// Run starts the terminal program in the alternate screen.
func Run(opts Options) error {
	a, err := New(opts)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	defer a.Close()
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
