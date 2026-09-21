// Command cellsheet is a terminal spreadsheet.
//
// Usage:
//
//	cellsheet                  start a new workbook
//	cellsheet budget.cell      open one
//	cellsheet -color=none      no escape sequences at all
//	cellsheet -version
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/badvibecoder/cellsheet/internal/app"
)

// version is the release version, and can be stamped at build time by the
// Makefile with -ldflags "-X main.version=...".
var version = "0.1.0"

func main() {
	color := flag.String("color", "auto", "colour: auto, none, 16, 256 or true")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Printf("cellsheet %s\n", version)
		return
	}

	path := flag.Arg(0)
	if path == "" && flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	isTerminal := stdoutIsTerminal()

	if err := app.Run(app.Options{
		Path:       path,
		Color:      *color,
		IsTerminal: isTerminal,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "cellsheet:", err)
		os.Exit(1)
	}
}

// stdoutIsTerminal avoids a dependency on golang.org/x/term, which would raise
// the minimum Go version for no benefit: a character device is all we need to
// know in order to choose a default colour mode.
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `cellsheet — a terminal spreadsheet

Usage:
  cellsheet [file.cell]      open a workbook, or start a new one
  cellsheet -version

Options:
  -color MODE    auto, none, 16, 256 or true (default auto)

Keys:
  Arrows             move the cursor        Shift+arrows   extend a selection
  Enter or typing    edit a cell            F2             edit in place
  Ctrl+S             save                   Ctrl+Q         quit
  Ctrl+Z / Ctrl+Y    undo / redo            Ctrl+B         checkpoint now
  Alt+F/E/R/V        menus                  +/- on a header  resize
  Alt+D / Alt+C      delete row / column    Left at column A  reach the row header

File a bug or read the design: see PHASE-1-SPEC.md
`)
}
