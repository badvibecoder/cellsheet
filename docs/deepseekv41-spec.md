# cellsheet — Phase 1 Specification

**A terminal spreadsheet. One file, one binary, no dependencies.**

This document is the complete Phase 1 design. It is deliberately self-contained:
if every other file in this repository were lost, this document plus the
decision log in §3 would be enough to rebuild the program as specified.

| | |
|---|---|
| Project | cellsheet |
| Module | `cellsheet` (local module — no remote repository yet) |
| Language | Go 1.24 or newer |
| Platforms | Linux amd64 / arm64 (macOS and Windows stay compilable, unsupported for now) |
| File extension | `.cell` |
| Status | **Phase 1 complete.** Phase 2 (prototype) authorised. |
| Detailed companions | [`docs/01-architecture.md`](docs/01-architecture.md), [`docs/02-calculation-rules.md`](docs/02-calculation-rules.md), [`docs/03-cell-file-format.md`](docs/03-cell-file-format.md), [`docs/04-tui-layout.md`](docs/04-tui-layout.md), [`docs/05-modularity.md`](docs/05-modularity.md), [`docs/06-open-questions.md`](docs/06-open-questions.md), [`docs/07-cross-sheet-formulas.md`](docs/07-cross-sheet-formulas.md) |

---

## Table of contents

1. [What cellsheet is](#1-what-cellsheet-is)
2. [The original requirements](#2-the-original-requirements)
3. [Decision log](#3-decision-log)
4. [Technology and constraints](#4-technology-and-constraints)
5. [Repository layout](#5-repository-layout)
6. [Domain model](#6-domain-model)
7. [Calculation rules](#7-calculation-rules)
8. [Calculation engine and concurrency](#8-calculation-engine-and-concurrency)
9. [Rendering architecture](#9-rendering-architecture)
10. [TUI layout specification](#10-tui-layout-specification)
11. [Keyboard map](#11-keyboard-map)
12. [The `.cell` file format](#12-the-cell-file-format)
13. [Checkpointing and rollback](#13-checkpointing-and-rollback)
14. [Modularity and extension points](#14-modularity-and-extension-points)
15. [Reliability engineering](#15-reliability-engineering)
16. [Performance budgets](#16-performance-budgets)
17. [Compatibility](#17-compatibility)
18. [Cross-sheet formulas: the plan for later](#18-cross-sheet-formulas-the-plan-for-later)
19. [Phases](#19-phases)
20. [Phase 2 build plan](#20-phase-2-build-plan)
21. [Phase 3 test plan](#21-phase-3-test-plan)
22. [Glossary](#22-glossary)
23. [Build, install and release](#23-build-install-and-release)
24. [Final state and verification](#24-final-state-and-verification)

---

## 1. What cellsheet is

A two-dimensional spreadsheet that runs in a terminal. Rows, columns, tabbed
sheets, Excel-like keyboard shortcuts, typed cells (number / currency / text),
formulas, menus, automatic checkpointing and rollback — all in a single statically
linked binary that needs nothing else installed.

The design targets three properties above all:

1. **It must never lose your data.** Saving is atomic, files are checksummed, and
   the previous version is always kept.
2. **It must be modular to the point of boredom.** Adding a formula function means
   adding one file and rebuilding. No registry to edit, no switch to extend.
3. **It must feel instant.** Keystroke to screen in under one frame, no matter how
   large the sheet.

Explicitly *not* in v1: CSV import/export, dates, booleans, charts, formatting
beyond what is described here, cross-sheet calculations, mouse support, and any
network or remote-repository coupling.

---

## 2. The original requirements

Reproduced so this document can be read on its own. These are the user's words,
lightly formatted; where a requirement conflicted with terminal reality, §3
records how it was resolved.

### 2.1 General

- Terminal-based spreadsheet, two-dimensional, rows and columns, tabbed sheets
  similar to basic Excel functionality.
- Excel-like keyboard shortcuts:
  - `Ctrl+Q` quit
  - `Ctrl+S` save file
  - `Ctrl+C` copy cell
  - `Ctrl+V` paste cell
  - `Ctrl+D` delete row
  - `Ctrl+Shift+D` delete column

### 2.2 Rows

- Start at `r1`, 100 rows by default.
- If data pastes into the last rows, another 100 rows are created automatically.
- Cursor over the `r1` row name, press `+` or `-` to increase or shrink that
  single row's height, one character at a time.

### 2.3 Columns

- Start at `A`…`Z`, then `AA`…`AZ`, then `BA`…`BZ`, continuing to `AAA`, `AAAA`
  and so on.
- If data pastes into the last columns, another 26 columns are added.
- Cursor over the `A` column name, press `+` or `-` to increase or shrink that
  single column's width by one character at a time.

### 2.4 Cell data types

- **Numbers** — whole, integer, signed, floating point. Mathematical operations
  must account for each data type and convert at calculation time. The output
  format depends on the sign and on whether the value is fractional: if either
  operand carries a sign or a fraction, the receiving cell must end up a signed
  float. A ruleset and order of operations must be defined.
- **Currency** — numbers with a `$` at the front. Formatted as US dollars:
  `$50000` becomes `$50,000.00`. Must support mathematical operations.
- **Text** — everything else. Mathematics is never attempted on text.

### 2.5 Functions

- Excel-like functions, starting with `=SUM()`.
  - `=SUM(a1:d1)` — add a run of cells.
  - `=SUM(a1 + d1)` — normal operands `+ - / * %`.
  - `-=SUM(a1 + d1)` — the negative of the calculated value.
- Functions must be modular: adding a single module file to the repository
  increases the application's functionality.

### 2.6 Menu / TUI

- At minimum a top menu with a File header, reachable with Alt keys.
- `Alt+F` opens the File menu with Save, Exit, Save As, Create New.

### 2.7 Checkpointing

- Save diffs of the spreadsheet every 3 minutes, only when a change is detected.
- A top menu item for Rollback, allowing rollback up to 99 detected changes.
- These start at 0 = the current state. An immediate revert is rollback to 1.
- The menu lists the last 9 plus current, with date/timestamps.
- Going back further: a menu option to type a specific rollback number, showing
  the current maximum available (e.g. 47).

### 2.8 File type

- `*.cell`, standalone: contains the current copy of the spreadsheet and all
  rollbacks up to 99 behind the current state.

### 2.9 Look

The user supplied two ASCII mockups: the main screen, and the Rollback menu open
over the grid. The main screen is reproduced in §10 and in the README; the
prototype that rendered it lives in the project's development tree.

### 2.10 Data processing

- Written with multithreading in mind and asynchronous processing where
  applicable. Fifty calculations at a time should spin up several threads.

### 2.11 Phases

Each phase requires user validation before the next begins:
**1 Design → 2 Prototype → 3 Testing → 4 User testing → 5 Polish and documentation.**

---

## 3. Decision log

Every decision, with the reasoning that produced it. `D` items are settled.

| # | Decision | Reasoning |
|---|---|---|
| D1 | **The interface is approved**, with one addition: the active cell needs a visible highlight. Implemented as heavy `║` bars, reverse-video contents, and `[r1]` / `[C]` brackets on the row and column headers. | Colour alone is invisible on monochrome terminals and weak for colour-blind users; brackets are ASCII, cost no width, and are reinforced by colour. |
| D2 | **The cursor is driven by the arrow keys**, with `Shift`+arrows extending a range selection. | Requested. Also the mechanism that drives copy/paste. |
| D3 | **Undo/redo is high-resolution**: every individual change in a session is recorded, so undo is per-edit and never coarser than one keystroke. | Checkpoints are up to 3 minutes apart; a spreadsheet without fine undo feels broken within a minute. |
| D4 | **Checkpoints happen on the 3-minute timer *and* on notable events** (bulk paste, structural change, rollback, manual). | The mock shows labels like `Row 4 edited` at non-boundary times, which the timer alone cannot produce. |
| D5 | **Linux is the only supported target for v1.** macOS and Windows stay compiling in the test matrix. | Requested. Keeping the code portable is nearly free; porting later is not. |
| D6 | **Clipboard is internal only.** | No external programs, so "the binary is all you need" holds. |
| D7 | **Structural edits use `Alt` keys**: `Alt+D` deletes the row, `Alt+C` deletes the column. | `Ctrl+D` and `Ctrl+Shift+D` are the *same byte* in most terminals. Alt is unambiguous and the menu bar already depends on it. |
| D8 | *(superseded by D9 and D10)* | |
| D9 | **Numbers are exact fixed-point decimals**, not floating point. | `$0.10 + $0.20` must be exactly `$0.30`. For budgets, floating-point money errors are a defect, not a compatibility feature. |
| D10 | **Modules are compile-time files.** Add one file to a folder, rebuild, the feature exists. | `"Add a file to add a feature"` cannot mean runtime loading in a compiled language; see §14.6 for what scripting would cost. |
| D11 | **Text is left-aligned, numbers right-aligned** (the Excel convention). | Easier to scan a column of labels; this is what "Excel-like" means. |
| D12 | **v1 calculates within a single sheet.** Cross-sheet syntax is still lexed and preserved verbatim, and evaluates to `#REF!` with a clear message. | Cheap insurance: no `.cell` file ever needs migrating when the feature lands. Full plan in §18. |
| D13 | **Accepted:** `%` means postfix percent with `MOD(a,b)` for remainder; negative money displays as `-$500.00`; `YES`/`NO` stay text in v1; rollback is reversible; a manual "Create checkpoint now" (`Ctrl+B`) exists. | Excel vocabulary where it exists; explicit choices where Excel is ambiguous. |
| D14 | **Not adopted:** system clipboard, mouse support, crash journal, optional config file. | Fewer moving parts; every capability has a keyboard equivalent already. |
| D15 | **No CSV import or export in v1.** Tab-separated *paste* is in v1. | Requested. Paste covers getting data in from other applications. |
| D16 | **No remote repository.** The module is plain `cellsheet`; everything stays local. | Requested. Changing it later is `go mod edit -module`. |
| D17 | The single source of truth for the design is this document. | Requested, so the project can be recreated from one file. |

### 3.2 Decisions taken after Phase 1

Every entry here came from building, testing or using the program. Each one is
already reflected in the body of this document.

| # | Decision | Why it changed |
|---|---|---|
| D18 | **`-=SUM(A1:A2)` is a formula**, the same as `=-SUM(A1:A2)`. | The lexer understood the form but `cell.IsFormula` only looked for a leading `=`, so it was stored as plain text and never calculated. Reported in use. |
| D19 | **Enter on a cell with content opens it for editing.** Enter on an empty cell still moves down. | Typing a value and pressing Enter made the cell impossible to amend without retyping it. Reported in use. |
| D20 | **A missing closing bracket is added silently on commit.** | `=SUM(A1:A4` is the one malformed formula whose intent is unambiguous. Reported in use. |
| D21 | **Both headers are reachable and move along themselves.** `↑` at row 1 focuses the column header, `←` at column A the row header; arrows then walk along the header. | There was no way to reach the column header, so column widths could not be changed at all. Reported in use. |
| D22 | **An explicitly sized column keeps its width.** The last column stretches to fill only while its width was never set; otherwise the space goes to the next column, clipped. | The stretch gave back exactly what a resize added, so widening the rightmost column appeared to do nothing. Found while fixing D21. |
| D23 | **A tall row draws its text on one line**, at `height/2 + 1`. | The renderer drew the value once per line, so a four-line row repeated its contents four times. Reported in use. |
| D24 | **The frame is exactly the height of the terminal**, with no trailing newline. | A trailing newline scrolled the terminal by one line and pushed the menu bar off the top. Reported in use. |
| D25 | **The global keys work while a cell is being edited.** `Ctrl+S`, `Ctrl+Q` and `Ctrl+B` commit first, then act. | A half-typed value made saving silently stop responding. Found while fixing D24. |
| D26 | **Resize keys are matched by rune, not by key string.** A run of `+` applies that many steps. | A held key or a coalesced read arrives as one event, and `"+++"` matched nothing, so it started an edit instead of resizing. |
| D27 | **`Cell.IsFormula` delegates to the package function.** | It was a second, independent check, so a `-=` formula was recognised when typed but saved as a frozen value that never recalculated. Found by a test asserting the two agreed. |
| D28 | **A literal cell always stores its typed text**, and nothing ever reconstructs a cell from its display. | Currency shows two decimals, so `$0.0001` displays as `$0.00`; three code paths rounded the value away. Found by fuzzing. |
| D29 | **The cell writer and reader are exactly symmetric**, including for a cell with a source and no value. | Typing a space produced such a cell, the writer emitted a source the reader did not consume, and everything after it in the file was misread. Found by fuzzing. |

### 3.1 The two conflicts with terminal reality

Recorded plainly, because they are the kind of thing that otherwise gets
rediscovered painfully later.

**`Ctrl+Shift+D` cannot be delivered.** A terminal sends `Ctrl+D` and
`Ctrl+Shift+D` as the same byte (`0x04`). Nothing in the stream distinguishes
them. It works only on terminals implementing the kitty keyboard protocol
(kitty, wezterm, foot) and silently does nothing on GNOME Terminal, Konsole,
xterm and Alacritty. Resolved by D7.

**`Ctrl+C` no longer interrupts.** In a full-screen terminal program, raw mode
delivers `Ctrl+C` as a key, so copy works — at the cost of the usual interrupt.
`Ctrl+Q` quits, as specified.

---

## 4. Technology and constraints

### 4.1 Language: Go 1.24+

- Static binaries, real concurrency in the language, fast builds, and a standard
  library that covers compression, checksums and JSON without third parties.
- `CGO_ENABLED=0` produces a binary with no C library dependency, so it runs on
  any Linux from the last decade without matching a distro's glibc.

### 4.2 TUI library: Bubble Tea v1 (with Lip Gloss)

| Option | Verdict |
|---|---|
| **Bubble Tea v1.3.x** | Handles raw mode, Alt/Meta keys, bracketed paste, mouse, resize, and — critically — restoring the terminal if the program exits unexpectedly. |
| Hand-rolled ANSI | Full control, but re-implementing cross-terminal key decoding and crash-safe terminal restore is weeks of work and a classic source of broken terminals. |
| `tcell` | Good library, lower level than needed; the widget layer would still be ours. |

**Bubble Tea's declared minimum is Go 1.24.0** (verified from its `go.mod`), so
the module declares `go 1.24`.

Bubble Tea v2 exists in beta; we stay on the stable v1 line.

### 4.3 Rendering: our own compositor

Bubble Tea's renderer diffs lines of a string. A spreadsheet needs floating
panels drawn *over* the grid, hiding cells behind them, while the grid keeps its
borders. So we render into a 2-D grid of styled runes and composite overlays —
which is exactly what `prototype/look/canvas.go` already does.

Cost: ~400 lines of compositor we own. Benefit: overlays, exact border
junctions, correct wide-character handling — none of which line-diffing gives us.

### 4.4 Dependencies

| Need | Choice |
|---|---|
| Event loop, input, terminal control | `github.com/charmbracelet/bubbletea` |
| Colour downgrade detection | `github.com/muesli/termenv` (transitive) |
| Character widths | `github.com/mattn/go-runewidth` (transitive) |
| Compression | `compress/flate` (stdlib) |
| Checksums | `crypto/sha256` (stdlib) |
| Money arithmetic | our own fixed-point decimal (~250 lines) |
| Everything else | stdlib |

**Total third-party dependencies: one library and its tree**, vendored into
`vendor/` so builds work offline.

### 4.5 Hard constraints

| Constraint | How it is met |
|---|---|
| One binary, nothing else | No config file required, no data files, no external processes, no cgo. |
| Runs on any system from the last 5–6 years | Static binary; runtime feature detection for colour, Unicode and mouse. |
| Additive modularity | Every extension point is a package that self-registers in `init()`. |

---

## 5. Repository layout

Folders marked ★ are **extension points**: adding a file there adds a feature.

```
cellsheet/
├── go.mod                      module github.com/badvibecoder/cellsheet, go 1.24
├── go.sum
├── vendor/                     third-party code, committed for offline builds
├── Makefile                    build, test, fuzz, install
├── README.md                   what the project is and how to use it
│
├── bin/                        build output; the binary lands here, not in git
│
├── cmd/
│   └── cellsheet/
│       └── main.go             the only main(): flags, then start the app
│
├── internal/
│   ├── app/                    composition root: wires everything, owns lifetime
│   │
│   ├── cell/                   value kinds, exact decimal, inference, formatting
│   │
│   ├── grid/                   the model: workbook, sheets, sparse cells, geometry
│   │
│   ├── formula/
│   │   ├── ast/                syntax tree
│   │   ├── lexer/
│   │   ├── parser/             includes AutoClose, the bracket repair
│   │   ├── eval/               evaluator + workbook-scoped dependency graph
│   │   ├── ops/            ★   arithmetic operators and type promotion
│   │   └── functions/      ★   one file per function: sum.go, median.go, …
│   │
│   ├── calc/                   async engine: waves, worker pool, cancellation
│   ├── journal/                the operation log (what changed, and why)
│   ├── checkpoint/             history ring, reverse deltas, the 3-minute timer
│   ├── clipboard/              the internal copy buffer and tab-separated paste
│   ├── sheetfile/              the .cell container: read, write, migrate
│   │
│   ├── arch/                   architecture tests; no production code
│   ├── perf/                   budget assertions, pty tests, build matrix
│   │
│   └── tui/
│       ├── render/             the canvas compositor
│       ├── layout/             geometry: where every line goes
│       ├── screens/            the grid screen, menus, dialogs
│       └── theme/              colour tokens
│
└── docs/
    ├── deepseekv41-spec.md     this document
    └── manual.md               how to install and use the program
```

Two directories named in the Phase 1 plan do not exist, because nothing needed
them: `internal/platform` (no OS-specific code was required — the only
platform-dependent file is the pty test, guarded by a build tag) and the
`menu/items`, `keys/bindings` and `sheetfile/codec` extension points (the menus
and key map are small enough to live in one file each, and they are still
registration-style extension points — see §14). `internal/commands` was folded
into `internal/app`.

The Phase 1 look prototype lived in `prototype/look/` during design. It is not
part of the program and is not shipped; its compositor became
`internal/tui/render`.

**The dependency rule:** a package may only import packages strictly below it in
the table in §15.6. The arrows are strict, and a test fails the build if they are
broken.

---

## 6. Domain model

### 6.1 Sparse storage

A spreadsheet is mostly empty. Dense storage of 1,048,576 × 16,384 cells would
need ~130 GB, so cells are stored sparsely.

```go
// internal/grid/workbook.go
type Workbook struct {
    sheets []*Sheet
    index  map[string]*Sheet // lower-cased name -> sheet
    active int
}

// internal/grid/sheet.go
type Sheet struct {
    ID      uint32
    Name    string
    Cells   map[CellRef]cell.Cell // only cells that exist
    Rows    map[uint32]RowMeta    // only rows that differ from default
    Cols    map[uint32]ColMeta    // only columns that differ from default
    RowN    uint32                // how many rows currently exist
    ColN    uint32                // how many columns currently exist
    DefRowH int                   // default 1 line
    DefColW int                   // default 18 characters
}

type CellRef struct{ Row, Col uint32 }
```

A 100 × 26 sheet with 8 populated cells stores 8 cells plus two small maps.
Lookup is O(1). Default row height and column width live on the sheet, so
`Rows`/`Cols` stay empty until something is actually resized.

### 6.2 Limits

| Quantity | Limit | Note |
|---|---|---|
| Rows | 1,048,576 | Matches Excel, so any future export lands somewhere sane. |
| Columns | 16,384 | Matches Excel (`XFD`). |
| Row height | 1–20 lines | A held-down key cannot destroy the layout. |
| Column width | 4–80 characters | Same. |
| Checkpoints | 99 | Per the requirement. |

Reaching a limit produces a clear message, never a silent truncation.

### 6.3 Auto-growth

- A new sheet starts at **100 rows × 26 columns (A–Z)**.
- Pasting data past the last row adds **100 rows**.
- Pasting data past the last column adds **26 columns**.
- Growth is computed from the *shape of the paste* in one step, before any cell
  is written, so a 500-cell paste causes one resize, not 500.

### 6.4 Column naming

Excel's bijective base-26: `A…Z`, `AA…AZ`, `BA…BZ`, … `ZZ`, `AAA`, `AAAA`, …

```go
func ColName(i int) string { // 0 -> "A", 25 -> "Z", 26 -> "AA", 701 -> "ZZ", 702 -> "AAA"
    var b [16]byte
    n := len(b)
    for {
        n--
        b[n] = byte('A' + i%26)
        i = i/26 - 1
        if i < 0 {
            break
        }
    }
    return string(b[n:])
}
```

The `Z → AA` and `ZZ → AAA` boundaries are where naive modulo arithmetic fails;
both are explicit test cases. This function is already implemented and working in
`prototype/look/screen.go`.

### 6.5 Row and column resizing

- Cursor on a **row header** + `+`/`-` → that row grows or shrinks by one line.
- Cursor on a **column header** + `+`/`-` → that column grows or shrinks by one
  character.
- Out-of-range presses are ignored — no error, no wrap-around.

The geometry keeps **prefix sums** of row heights and column widths so that
"which row is under screen line 37?" is O(log n) rather than a walk. This is what
keeps scrolling smooth at 100,000 rows.

---

## 7. Calculation rules

### 7.1 Value kinds

| Kind | Meaning | Example |
|---|---|---|
| Empty | Nothing entered | *(blank)* |
| Number | A quantity with no currency | `450`, `12.75`, `-3` |
| Currency | A quantity of US dollars | `$50,000.00` |
| Text | Anything else; never used in maths | `Operations`, `YES` |
| Error | A calculation that could not be performed | `#DIV/0!` |

Two further properties are tracked and never shown as a separate kind: **sign**
and **fractional**.

> **The key design decision.** Numbers are never stored in a narrow type that
> could lose a sign or a fraction. Every number is an exact decimal that can
> always represent a sign and up to 18 decimal places. Promotion rules decide the
> *kind* of a result and how it displays — but there is never a lossy conversion.
> This is what satisfies the requirement that a calculation involving a sign or a
> float yields a "signed float".

### 7.2 Exact decimal representation

`float64` cannot represent `0.10` exactly; adding `$0.10` to `$0.20` gives
`0.30000000000000004`. Excel hides this by displaying 15 significant digits,
which is why spreadsheets famously disagree with calculators about money. We do
not have that problem.

```go
// internal/cell/decimal.go
type Dec struct {
    Mant  int64 // the digits, without a decimal point
    Scale int8  // how many of those digits are after the point
}
// 50000.00 -> Mant: 5000000, Scale: 2
// 12.75    -> Mant: 1275,    Scale: 2
// 3        -> Mant: 3,       Scale: 0
```

| Operation | Rule | Example |
|---|---|---|
| Add / subtract | Align scales, add digits, keep the larger scale | `1.5 + 1.25 = 2.75` |
| Multiply | Multiply digits, **add** scales | `1.5 × 1.25 = 1.875` |
| Divide | Compute to 12 decimal places, round half away from zero | `1 ÷ 3 = 0.333333333333` |
| Percent | Divide by 100 | `50% = 0.5` |
| Power | Integer exponents repeat-multiply; others via the division rule | `2^10 = 1024` |

**Range:** about ±9.2 × 10¹⁸. Exceeding it yields `#NUM!`, never a wrong answer.

### 7.3 Inferring a cell's kind from typed input

Applied **in this order**; first match wins.

| # | Test | Result |
|---|---|---|
| 1 | Starts with `=` | **Formula**; kind comes from the result |
| 2 | Empty or only spaces | **Empty** |
| 3 | Matches the currency pattern | **Currency** |
| 4 | Matches the number pattern | **Number** (integer form if no decimal point) |
| 5 | Anything else | **Text** |

**Currency pattern:** `[+|-] $ [digits with optional thousands commas] [ . digits ]`

| Input | Parsed | Displayed |
|---|---|---|
| `$50000` | Currency 50000 | `$50,000.00` |
| `$50,000` | Currency 50000 | `$50,000.00` |
| `$1,250.00` | Currency 1250 | `$1,250.00` |
| `-$500` or `$-500` | Currency −500 | `-$500.00` |
| `$0.5` | Currency 0.5 | `$0.50` |

Currency is **always** displayed with two decimals and thousands separators, US
format. Display only — the stored value keeps full precision.

**Number pattern:** `[+|-] [digits] [ . digits ] [ (e|E) [+|-] digits ]`, with at
least one digit somewhere. A leading `.` is accepted, so `.5` is `0.5`.

| Input | Parsed | Displayed |
|---|---|---|
| `450` | Number 450 | `450` |
| `-3` | Number −3 | `-3` |
| `12.75` | Number 12.75 | `12.75` |
| `50,000` | Number 50000 | `50000` |
| `1e3` | Number 1000 | `1000` |
| `10.00` | Number 10 | `10` |

Plain numbers get **no** thousands separators; currency always does.

**Deliberately not recognised in v1** (all become Text): `TRUE`/`FALSE`, `50%`
in a cell, `(500)` as negative, and dates.

### 7.4 Combining types in arithmetic

**Step 1 — can it happen?**

| Operand | Behaviour |
|---|---|
| Error | Propagates: `=1 + #DIV/0!` is `#DIV/0!` |
| Text | `#VALUE!` — maths is never attempted on text |
| Empty | Treated as `0` in arithmetic |

These checks happen before any arithmetic, so a text cell can never produce a
nonsense number.

**Step 2 — what kind is the answer?**

| Operation | Left | Right | Result | Example |
|---|---|---|---|---|
| `+` `-` | Number | Number | Number | `450 + 12.75 = 462.75` |
| `+` `-` | Currency | Currency | Currency | `$50,000.00 + $1,250.00 = $51,250.00` |
| `+` `-` | Currency | Number | Currency | `$500.00 - 20 = $480.00` |
| `+` `-` | Number | Currency | Currency | `20 + $500.00 = $520.00` |
| `×` | Number | Number | Number | `450 × 12.75 = 5737.50` |
| `×` | Currency | Number | Currency | `$500.00 × 3 = $1,500.00` |
| `×` | Number | Currency | Currency | `3 × $500.00 = $1,500.00` |
| `×` | Currency | Currency | **Number** | dollars × dollars is a squared quantity, not dollars |
| `÷` | Number | Number | Number (always fractional) | `10 ÷ 4 = 2.5` |
| `÷` | Currency | Number | Currency | `$100.00 ÷ 4 = $25.00` |
| `÷` | Currency | Currency | **Number** (a ratio) | `$200.00 ÷ $50.00 = 4` |
| `÷` | Number | Currency | **Number** | `100 ÷ $50.00 = 2` |
| `÷` by zero | any | zero | `#DIV/0!` | |
| `%` (postfix) | Number | — | Number | `50% = 0.5` |
| `%` (postfix) | Currency | — | Currency | `$50.00% = $0.50` |
| `^` | Number | Number | Number | `2^10 = 1024` |
| `^` | Currency | Number | Currency | `$100.00^2 = $10,000.00` |
| `MOD(a,b)` | Currency | Currency | Currency | `MOD($100.00,$30.00) = $10.00` |
| comparisons | any | any | Number (1 = true, 0 = false) | `$5.00 > $3.00 = 1` |

**In one sentence:** currency survives addition, subtraction and scaling by a
plain number; it does not survive multiplication by currency or division by
currency, because the units no longer make sense.

**Sign and fraction propagation:** a negative value keeps its sign through any
operation that can produce one; nothing is silently abs()-ed. If either operand
is fractional, the result keeps full fractional precision. A whole result
displays without a decimal point but retains its full precision internally.

### 7.5 Formula grammar

```
expression     := comparison
comparison     := additive ( ("=" | "<>" | "<" | ">" | "<=" | ">=") additive )?
additive       := multiplicative ( ("+" | "-") multiplicative )*
multiplicative := unary ( ("*" | "/") unary )*
unary          := ("-" | "+") unary | power
power          := postfix ( "^" unary )?          right-associative
postfix        := primary ("%")*
primary        := NUMBER | CURRENCY | STRING
                | CELLREF | RANGE | FUNCTION | "(" expression ")"
function       := NAME "(" [ argument ("," argument)* ] ")"
argument       := expression | RANGE
range          := CELLREF ":" CELLREF
cellref        := [SHEET "!"] COLUMN ROW
SHEET          := NAME | "'" quoted-name "'"
```

### 7.6 Order of operations

Highest first.

| Level | Operator | Associativity | Note |
|---|---|---|---|
| 1 | `( … )` | — | Innermost first |
| 2 | `x%` | left | Postfix percent; binds tighter than negation, so `-50%` is `-0.5` |
| 3 | `^` | **right** | `2^3^2 = 512`, matching Excel |
| 4 | `-x`, `+x` (unary) | right | `-2^2 = -4`, matching Excel |
| 5 | `*`, `/` | left | `8/4/2 = 1` |
| 6 | `+`, `-` | left | `1-2-3 = -4` |
| 7 | comparisons | left | Lowest; result `1` or `0` |

### 7.7 Ranges

`A1:D1` means the rectangle from A1 to D1 inclusive.

- Only valid as a **function argument**; `=A1:D1` alone is `#VALUE!`.
- May be written in either direction (`D1:A1` is the same rectangle).
- May span rows and columns (`A1:C5` is 15 cells).
- **Skips empty cells and text** when summed or averaged — Excel's behaviour, and
  what makes subtotalling a column with a text header work.
- Promotes to **Currency** if any cell in it is Currency.

A **scalar** argument that is text produces `#VALUE!`. So `=SUM(A1:A3)` tolerates
text in A2, but `=SUM(A2, 1)` does not. This distinction is tested explicitly.

### 7.8 The user's examples, spelled out

| Written | Parses as | Result |
|---|---|---|
| `=SUM(A1:D1)` | function `SUM`, one range argument | Adds A1+B1+C1+D1, skipping text and blanks |
| `=SUM(A1 + D1)` | function `SUM`, one expression argument | Evaluates `A1 + D1`, then sums that value |
| `-=SUM(A1 + D1)` | unary minus on the call | The negated total; also accepted as `=-SUM(A1+D1)` |
| `=SUM(A1:D1, F1, 10)` | three arguments | Mixed ranges, cells and literals |
| `=A1 * 1.07` | arithmetic | Currency × Number → Currency |

`-=` is not standard Excel; it is accepted as a convenience and normalised to
`-SUM(...)`.

Case and whitespace are insignificant: `=sum(a1:d1)`, `=SUM(A1:D1)` and
`= Sum ( A1 : D1 )` are identical.

### 7.8.1 One repair is made automatically

A formula that stops in the middle of a call is closed for the user:

```
=SUM(A1:A4        ->  =SUM(A1:A4)
=((1+2            ->  =((1+2))
```

This is the only repair the program makes, because it is the only one that is
unambiguously what the user meant. Brackets inside text literals and quoted
sheet names are not counted, a formula that already parses is never touched, and
*more* closing brackets than opening ones are left alone — silently dropping one
could change what was written. Everything else malformed is reported as an
error.

Note that `-=SUM(A1:A2)` and `=-SUM(A1:A2)` are both accepted and mean the same
thing: the negative of the sum.

### 7.9 Errors

| Error | Meaning | Triggered by |
|---|---|---|
| `#VALUE!` | Wrong kind of value | Text in arithmetic; text as a scalar argument; a bare range |
| `#DIV/0!` | Division by zero | `=1/0`; a denominator that is 0 or empty |
| `#NUM!` | Out of range | Overflow past ±9.2 × 10¹⁸; `=(-1)^0.5` |
| `#CIRC!` | Circular reference | `A1 = B1`, `B1 = A1` |
| `#NAME?` | Unknown function | `=SUMM(A1)` |
| `#REF!` | Reference points nowhere | A deleted row/column, a missing sheet, a disabled cross-sheet reference |
| `#ERR!` | Internal failure | A function panicked; isolated to one cell |

Errors propagate through a calculation and clear as soon as the cause is fixed.
An error cell displays centred, in red.

### 7.10 Functions

Each function is a small, pure, self-registering module:

```go
// internal/formula/functions/sum.go
package functions

import (
    "cellsheet/internal/cell"
    "cellsheet/internal/formula/registry"
)

func init() {
    registry.Register(registry.Spec{
        Name:       "SUM",
        Summary:    "Adds all the numbers in a range or list of values.",
        MinArgs:    1,
        MaxArgs:    -1, // variadic
        TakesRange: true,
        Fn: func(_ *registry.Ctx, args []cell.Arg) cell.Value {
            total := cell.ZeroNumber()
            for _, a := range args {
                for _, v := range a.Values() { // ranges expand here
                    if v.Skip() {              // empty or text inside a range
                        continue
                    }
                    total = total.Add(v)       // currency promotion happens here
                }
            }
            return total
        },
    })
}
```

That is the entire integration. Every function gets argument-count checking, kind
checking, error propagation, panic isolation and parallel safety from the
registry, for free.

| Wave | Functions |
|---|---|
| **v1 (Phase 2)** | `SUM` |
| v1.1 | `AVERAGE`, `MIN`, `MAX`, `COUNT`, `COUNTA`, `PRODUCT`, `ABS`, `ROUND`, `MOD` |
| v1.2 | `IF`, `AND`, `OR`, `NOT` (after booleans are settled) |
| v1.3 | `VLOOKUP`, `INDEX`, `MATCH`, `CONCAT`, `LEN`, `UPPER`, `LOWER` |
| Later | `PMT`, `IRR`, `NPV` — the reason exact decimals matter |

Volatile functions (`NOW`, `TODAY`, `RAND`) declare `Volatile: true` and force a
full recalculation. None are in v1.

---

## 8. Calculation engine and concurrency

This is where the multithreading requirement lives.

### 8.1 Dependency tracking, not full recalculation

Every formula cell records what it reads. Editing `A1` recomputes only the cells
that transitively depend on it — not the other 2,599.

- `dependents[A1] = {C1, F7, …}` — who must be recomputed when A1 changes.
- `precedents[C1] = {A1, B1}` — what C1 must wait for.

Cycles are detected when the graph is **built**, not at evaluation time, and the
offending cells become `#CIRC!` instead of hanging the program.

### 8.2 Wave scheduling across goroutines

After an edit, the engine computes the dirty set and sorts it into **waves**:

```
Wave 0:  C1 = SUM(A1:B1)      <- A1,B1 changed; no formula dependencies
Wave 1:  E1 = C1 * 1.07       <- needs C1
Wave 2:  E2 = E1 - D1         <- needs E1
```

Cells **inside** a wave have no dependency on each other, so they evaluate
concurrently. Cells **between** waves are strictly ordered. This gives full
parallelism with no risk of reading a half-computed value, and it is
**deterministic**: the answer never depends on scheduling.

```
        ┌───────────────┐
        │  dirty set    │
        └───────┬───────┘
                ▼
        ┌───────────────┐   wave 0 ─▶ [worker][worker][worker][worker]
        │ wave planner  │   wave 1 ─▶ [worker][worker]
        │ (topological) │   wave 2 ─▶ [worker]
        └───────────────┘
```

### 8.3 The worker pool

| Property | Decision |
|---|---|
| Pool size | `min(runtime.NumCPU(), 8)`, user-tunable |
| Lifetime | One shared pool per running application, not one per recalculation |
| Threshold | Below ~64 dirty cells, evaluate inline — fanning 3 cells to 8 workers is slower than computing them |
| Cancellation | Each recalculation carries a generation number; a newer edit abandons the older generation and its results are discarded |
| Panic isolation | Every function call is wrapped so a broken module yields `#ERR!` in one cell, not a crash |

The UI never blocks. A recalculation is a Bubble Tea command; the grid stays
responsive and a `recalcDone` message updates the affected cells. For typical
sheets the cycle is sub-millisecond and invisible; for a pathological sheet the
status bar shows progress rather than freezing.

---

## 9. Rendering architecture

```
 model change
      │
      ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ layout:      │──▶│ draw base    │──▶│ composite    │──▶│ encode to    │
│ rows/cols in │   │ frame: grid, │   │ overlays:    │   │ ANSI, one    │
│ view, x/y of │   │ headers,     │   │ menus,       │   │ write() per  │
│ every line   │   │ tabs, status │   │ dialogs      │   │ frame        │
└──────────────┘   └──────────────┘   └──────────────┘   └──────────────┘
   O(visible)         O(visible)         O(panel)          O(screen)
```

Four properties matter:

1. **Only visible cells are touched.** Cost is proportional to the terminal size
   (~4,800 cells at 120 × 40), never to the sheet size. A 100-row and a
   1,000,000-row workbook render in the same time.
2. **Panels composite over the base**, so the Rollback menu genuinely hides what
   is behind it.
3. **One write per frame**, assembled in memory, which eliminates tearing.
4. **The frame is exactly as many lines as the terminal has rows, and does not
   end with a newline.** A trailing newline advances the cursor past the last
   row, the terminal scrolls up by one, and the top line — the menu bar —
   leaves the screen. This is why the frame's height is asserted directly rather
   than left to the golden comparison alone.

### 9.1 The canvas

```go
type Style struct{ Fg, Bg Color; Attr Attr }
type Canvas struct { W, H int; cells []canvasCell }

func (c *Canvas) Set(x, y int, r rune, s Style)                 // bounds-checked
func (c *Canvas) Draw(x, y int, text string, s Style)           // width-aware
func (c *Canvas) Fill(x, y, w, h int, r rune, s Style)
func (c *Canvas) HRule(y, x0, x1 int, h rune, j map[int]rune, s Style)
func (c *Canvas) VRule(x, y0, y1 int, v rune, j map[int]rune, s Style)
func (c *Canvas) Blit(src *Canvas, ox, oy int)                  // opaque overlay
func (c *Canvas) String(mode colorMode) string                  // ANSI, minimal codes
```

A working implementation is in `prototype/look/canvas.go` and is intended to
move into `internal/tui/render` essentially unchanged.

### 9.2 Input model

All input becomes a **command** (`EditCell`, `PasteRange`, `ResizeRow`,
`RollbackTo`, …). Keys map to commands; commands apply to the model. No key
handler knows how the model works and no model knows about keyboards. This is
what makes the UI testable without a terminal: feed the model a list of keys and
assert the resulting state.

---

## 10. TUI layout specification

### 10.1 Regions, top to bottom

```
 ┌─ File (Alt+F) ── Edit ── Rollback (Alt+R) ── View ─────── cellsheet: FILE [●] ─┐  y=0   menu bar
 ├──────┬────────────────────────────────────────────────────────────────────────┤  y=1   rule
 │  C1  │ fx: =SUM(A1:B1)                                                        │  y=2   formula bar
 ├──────┴──────┬──────────────┬──────────────┬──────────────┬─────────────────── ┤  y=3   rule + column tees
 │             │      A       │      B       │      C       │        D           │  y=4   column headers
 ├─────────────┼──────────────┼──────────────┼──────────────┼────────────────────┤  y=5   rule
 │ r1          │   $50,000.00 │    $1,250.00 │║  $51,250.00║│                    │  y=6+  grid rows
 │             │              │              │              │                    │        (height + 1 lines each)
 ├─────────────┴──────────────┴──────────────┴──────────────┴────────────────────┤        closing rule
 │ [ Sheet1* ]  [ Sheet2 ]  [ + ]                                                 │  h-4   sheet tabs
 ├────────────────────────────────────────────────────────────────────────────────┤  h-3   rule
 │ READY │ 100R x 26C │ Active: C1 (Currency) │ … │ ^S Save │ ^Q Quit │ +/- …    │  h-2   status bar
 └────────────────────────────────────────────────────────────────────────────────┘  h-1   bottom border
```

**Fixed overhead: 10 rows.** A 32-row terminal gives 22 rows to the grid.
Minimum supported size **60 × 16**; below that a "Terminal too small" panel.

### 10.2 Horizontal measurements

| Element | Width | Rule |
|---|---|---|
| Left/right frame | 1 each | |
| Row-number gutter | **13** | Must fit `r1000000` and `(height: 99)` |
| Reference box | **6** | Fits `C1` and `AA100`; grows if needed |
| Column interior | **18** | Default; per-column override 4–80 |
| Column total | **19** | Interior + one border |

The first column's left border sits at `x = 1 + gutterWidth = 14`.

**Widening a column widens it on screen.** Every visible column is drawn at the
width the sheet says it has, except for one case:

- The **last visible column stretches** to meet the right frame when its width
  is still the default, so the remaining space is never left as a gap. At 120
  columns that yields exactly 5 visible columns (A–E), with E taking the
  leftover.
- A column whose width was **set explicitly keeps that width**, and the space
  that cannot hold another whole column is given to the next column, shown
  clipped. This matters because the stretch would otherwise give back exactly
  what a resize added, making the resize appear to do nothing.

`+`/`-` on a header reflows the columns after it, and the view scrolls to keep
the column being resized on screen.

### 10.3 Grid rows

Each row occupies **its height in lines plus one separator line**. Row selection
is greedy top-down; if a single line is left over at the bottom it is absorbed
into the last visible row, so the closing rule always lands exactly on the bottom
of the grid area — no doubled rules, no ragged edge.

**A cell's text is drawn on exactly one of its row's lines**, vertically placed
like this:

| Row height | Line the text sits on |
|---|---|
| 1 | 1 |
| 2 | 2 (the lower of the two middle lines) |
| 3 | 2 (the middle) |
| 4 | 3 (the lower of the two middle lines) |
| 5 | 3 (the middle) |

In general, line `height / 2 + 1`, counting from one. Every other line of a tall
row is blank. Without this the renderer drew the value once per line, so a
four-line row showed its contents four times. The cursor's heavy `║` bars run
the full height of a tall cell, so it stays obviously selected even though its
text sits on one line.

**Values are laid out in `width − 1` columns with one trailing blank.** That
margin is why `$50,000.00` sits one space clear of its border.

### 10.4 The active cell

Marked in three places at once:

| Cue | Where |
|---|---|
| `║` heavy bars | Both vertical borders of the cell |
| Reverse video | The cell's contents |
| `[r1]` | Row-number gutter |
| `[C]` | Column header |
| Reference box + status bar | `C1` and `Active: C1 (Currency)` |

Brackets rather than colour alone: colour is invisible on monochrome terminals
and weak for colour-blind users. Brackets are ASCII, cost no width, and are
reinforced by colour when available. **The cell's width never changes when the
cursor moves.**

### 10.5 Selection

The active cell stays put while the selection grows away from it (Excel's
behaviour). In colour the block is tinted with a dark slate background; in
monochrome its perimeter is drawn with dashed rules (`┄` horizontal, `┆`
vertical). While more than one cell is selected the status bar shows
`Sum: $65,770.50 · 2R x 2C selected`.

### 10.6 Status bar segments

| Segment | Example |
|---|---|
| Mode | `READY` (also `EDIT`, `POINT`, `MENU`, `ROLLBACK`, `RESIZE`) |
| Selection summary | `Sum: $65,770.50 · 2R x 2C selected` or `100R x 26C` |
| Active cell | `Active: C1 (Currency)` |
| Checkpoint | `Checkpoint: 2m 14s [Rev 4/99]` |
| Key hints | `^S Save`, `^Q Quit`, `+/- Resize Header` |

Trailing segments that do not fit are dropped whole rather than chopped mid-word.

### 10.7 Overlays

**File menu** (`Alt+F`) hangs below the word `File`; items with right-aligned
shortcuts, full-width separators, the highlighted item in reverse video.

**Rollback menu** (`Alt+R`) is centred, opening just below the formula bar:

```
     ┌──────────────────────────────────────────────────────────┐
     │ 0: Current State          [10:14:02 PM] (Uncommitted)    │
     │ 1: Auto-Checkpoint        [10:11:00 PM]                  │
     │ 2: Auto-Checkpoint        [10:08:00 PM]                  │
     │ 3: Auto-Checkpoint        [10:05:00 PM]                  │
     │ 4: Row 4 edited           [10:02:15 PM]                  │
     │ 5: Bulk Paste (+12 cells) [09:59:00 PM]                  │
     │ 6: Auto-Checkpoint        [09:56:00 PM]                  │
     │ 7: Auto-Checkpoint        [09:53:00 PM]                  │
     │ 8: Sheet initialized      [09:50:00 PM]                  │
     │ 9: Initial file load      [09:47:12 PM]                  │
     │──────────────────────────────────────────────────────────│
     │ J: Jump to custom rollback index... (Max available: 47)  │
     └──────────────────────────────────────────────────────────┘
```

`J` opens the numeric prompt, which restates the valid range and validates before
anything changes:

```
     ┌────────────────────────────────────────┐
     │ Rollback                               │
     │ Rollback to index (1-47): 4█           │
     │ Enter = go   Esc = cancel              │
     └────────────────────────────────────────┘
```

### 10.8 Resize mode

The header must be *reached* before it can be resized, which is what the
original brief means by "moving cursor over the `r1` row name":

| From | Key | Goes to |
|---|---|---|
| Any cell, column A | `←` | The row header for that row |
| Row 1 | `↑` | The column header for that column |
| Column header | `←` / `→` | The neighbouring column, staying on the header |
| Either header | `↓` or `→` (row header) | Back into the grid |
| Either header | `Esc` | Back into the grid |

`+`/`-` with the cursor on a header resizes that one row or column. The status bar
shows `RESIZE ROW r5 (height 2)  +/-` and the header is highlighted. `Esc` leaves
the mode. Limits 1–20 lines and 4–80 characters; further presses are ignored.

### 10.9 Deliberate deviations from the user's sketch

| # | Sketch | Built | Why |
|---|---|---|---|
| 1 | Active cell interior 2 characters narrower | Interior width unchanged; borders swapped to `║`, contents reversed | Cells must not shift when the cursor moves |
| 2 | No active-cell indicator beyond the bars | Added `[r1]` / `[C]` header brackets | Colour-only highlights vanish on monochrome terminals |
| 3 | All text right-aligned | Text left, numbers right (D11) | Excel's convention; easier to scan |
| 4 | Rollback divider drawn `\|----\|` | Proper `├────┤` rule | Consistent with the rest of the frame |
| 5 | Values flush against the right border | One trailing space | The sketch shows this too; it is a real rule |
| 6 | No bottom border under the status bar | `└────┘` | The frame must close |
| 7 | `●` in the title | Same, `○` when saved | Ambiguous-width character; see §17 |

---

## 11. Keyboard map

### Always available

| Key | Action |
|---|---|
| `Ctrl+Q` | Quit |
| `Ctrl+S` | Save |
| `Ctrl+Shift+S` | Save As |
| `Ctrl+N` | New workbook |
| `Ctrl+O` | Open |
| `Ctrl+B` | Create a checkpoint now |
| `Ctrl+T` | Add a sheet |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Previous / next sheet, wrapping |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo (per-edit, in memory) |
| `Alt+F` / `Alt+E` / `Alt+R` / `Alt+V` | File / Edit / Rollback / View menus |
| `F1` | Help and the function list |
| `F2` | Edit the active cell |
| `Esc` | Cancel / close the current menu or mode |

The global keys are global: `Ctrl+S`, `Ctrl+Q` and `Ctrl+B` also work **while a
cell is being edited**, committing the edit first and then acting. Without that,
a half-typed value made saving silently stop responding (D25).

### Navigation

| Key | Action |
|---|---|
| `↑` `↓` `←` `→` | Move one cell |
| `Home` / `End` | First / last populated column in the row |
| `Ctrl+Home` / `Ctrl+End` | First cell / last populated cell |
| `PgUp` / `PgDn` | One screenful of rows |
| `Tab` / `Shift+Tab` | Right / left, wrapping to the next row |
| `Enter` | On a cell with content: open it for editing, so it can be amended rather than retyped. On an empty cell: move down. While editing: commit and move down |
| `Shift+Enter` | Up, committing an edit first |
| `Ctrl+←` / `Ctrl+→` | Next non-empty cell (skips blanks) |
| `Ctrl+G` | Go to a typed address |

### Selection and editing

| Key | Action |
|---|---|
| `Shift` + arrows | Extend the selection |
| `Ctrl+Shift+←` / `→` | Extend to the next non-empty cell |
| `Ctrl+A` | Select the used area; again for the whole sheet |
| `Shift+Space` / `Ctrl+Space` | Select the whole row / column |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste |
| `Delete` | Clear the selection's contents |
| Insert row / column | Via the Edit menu |
| `Alt+D` | **Delete the current row** (D7) |
| `Alt+C` | **Delete the current column** (D7) |
| `+` / `-` (on a header) | Resize that row or column by one unit |

### Reaching a header, and resizing

`+`/`-` resize the row or column whose header the cursor is on, so the header has
to be reachable with the keyboard:

| From | Key | Goes to |
|---|---|---|
| Any cell, column A | `←` | The row header for that row |
| Row 1 | `↑` | The column header for that column |
| Row header | `↑` / `↓` | The row above / below, staying on the header |
| Column header | `←` / `→` | The column left / right, staying on the header |
| Row header | `→` | Back into the grid |
| Column header | `↓` | Back into the grid |
| Either header | `Esc` | Back into the grid |

Staying on the header is what lets several rows or columns be sized one after
another (D21). Resizing reflows the columns after the one being changed, and
scrolls the view so the resized row or column stays on screen.

Resize keys are matched **by rune, not by key string**, so a held key or a
coalesced read applies one step per character rather than starting an edit (D26).
A mixed run such as `+-` is not a gesture and is treated as typed text.

### Editing

| Key | Action |
|---|---|
| Printable characters | Start an edit with that text; while editing, append |
| `Enter` | Commit and move down |
| `Tab` | Commit and move right |
| `Esc` | Abandon the edit |
| `Backspace` | Delete the last character |
| `Ctrl+U` | Clear the edit buffer |
| `F2` | Edit the active cell, seeded with its current content |

A formula that stops in the middle of a call is closed for you on commit — see
§7.8.1.

Terminal caveats for `Ctrl+Shift+D`, `Ctrl+C` and `Alt` are in §3.1.

---

## 12. The `.cell` file format

A `.cell` file is standalone: it holds the current workbook **and** up to 99
rollback points. No sidecar files, no database, no server.

### 12.1 Container

```
 byte 0
 ┌─────────────────────────────────────────────────────────┐
 │ HEADER — fixed 64 bytes                                 │
 │   magic "CELLSHEET", format version, minimum reader     │
 │   version, flags, created/modified timestamps,          │
 │   section count, directory offset, header checksum      │
 ├─────────────────────────────────────────────────────────┤
 │ SECTION DIRECTORY — 48 bytes per section                │
 │   id, offset, compressed length, uncompressed length,   │
 │   SHA-256 (truncated to 24 bytes), flags                │
 ├─────────────────────────────────────────────────────────┤
 │ SECTION PAYLOADS                                         │
 │   ┌──────────┐ ┌──────────┐ ┌──────────┐                │
 │   │ MANIFEST │ │  STATE   │ │ HISTORY  │                │
 │   └──────────┘ └──────────┘ └──────────┘                │
 ├─────────────────────────────────────────────────────────┤
 │ TRAILER — SHA-256 of everything above                   │
 └─────────────────────────────────────────────────────────┘
```

Little-endian throughout. Unknown section ids are preserved verbatim on rewrite,
so a newer version never loses data written by an older one.

**Why not ZIP, JSON or SQLite:** ZIP gives no place for checksums; plain JSON is
5–10× larger and slow at 100,000 cells (we do use JSON *inside* `MANIFEST`, where
readability beats bytes); SQLite is a large dependency that breaks the single
small static binary promise. The custom container is ~400 lines of stdlib.

### 12.2 `MANIFEST` — JSON, compressed

```json
{
  "formatVersion": 1,
  "minReaderVersion": 1,
  "appVersion": "0.2.0",
  "created":  "2026-03-04T21:47:12-05:00",
  "modified": "2026-03-04T22:14:02-05:00",
  "activeSheet": 0,
  "cursor": { "sheet": 0, "row": 0, "col": 2 },
  "sheets": [
    { "name": "Sheet1", "rows": 100, "cols": 26, "defaultRowHeight": 1, "defaultColWidth": 18 },
    { "name": "Sheet2", "rows": 100, "cols": 26, "defaultRowHeight": 1, "defaultColWidth": 18 }
  ],
  "history": { "count": 5, "headLabel": "Current State" },
  "scratch": {}
}
```

`scratch` lets a future version record things an older version should carry but
not interpret.

### 12.3 `STATE` — binary, compressed

Varint-tagged records, per sheet:

```
SHEET_BEGIN(sheetID, nameRef, rows, cols, defaultH, defaultW)
  ROW_META(row, height)          // only rows differing from default
  COL_META(col, width)           // only columns differing from default
  CELL(row, col, kind, payload)  // only non-empty cells
    kind 0 = Empty     (deletes; appears only in deltas)
    kind 1 = Number    sign, mantissa varint, scale
    kind 2 = Currency  sign, mantissa varint, scale
    kind 3 = Text      UTF-8 bytes
    kind 4 = Formula   UTF-8 source; the value is recomputed on load
    kind 5 = Error     error code
SHEET_END
```

Formulas are stored as **source text, never as cached results**, and recalculated
on load. A file can therefore never contain a stale answer, and the formula
language can improve without invalidating old files.

> **Cost, stated honestly:** opening a workbook with 50,000 formulas recalculates
> 50,000 formulas. At v1 targets that is well under a second. If it ever becomes
> a problem, add an optional cached-value section that is always re-verified
> against the source — never trusted blindly.

### 12.4 `HISTORY` — binary, compressed

Up to 99 entries, newest first. Index 0 is the current state and is not a delta;
entries 1–99 are the reverse deltas.

```
HISTORY(count, headRevisionID)
  ENTRY(
    index,        // 1..99
    revisionID,   // monotonically increasing, never reused
    timestamp,    // unix milliseconds + UTC offset minutes
    kind,         // Auto | BulkPaste | RowEdited | ColEdited | Structural | Rollback | Initial | Manual
    label,        // "Auto-Checkpoint", "Bulk Paste (+12 cells)", "Row 4 edited"
    summary,      // counts: cells added/removed/changed, rows/cols added/removed
    payload       // the reverse delta
  ) x count-1
```

Reverse-delta operations mirror the `STATE` records but express *undo*:

```
UNDO_CELL(sheet, row, col, previousKind, previousPayload)  // Empty clears
UNDO_ROW_META(sheet, row, previousHeight)
UNDO_COL_META(sheet, col, previousWidth)
UNDO_ROWS(sheet, at, count, removedCells...)
UNDO_COLS(sheet, at, count, removedCells...)
UNDO_SHEET(id, definition)
```

Applying entry *k* transforms the workbook from state *k−1* into state *k*.
Rolling back to index *k* means applying entries 1…*k*. Editing one cell produces
a delta of a few dozen bytes.

### 12.5 Size estimates

A 100 × 26 budget sheet with a few hundred populated cells:

| Part | Uncompressed | Compressed |
|---|---|---|
| HEADER + directory | ~200 B | — |
| MANIFEST | ~600 B | ~400 B |
| STATE | ~12 KB | ~3 KB |
| HISTORY (99 small deltas) | ~40 KB | ~8 KB |
| **Total** | | **≈ 12 KB** |

A 50,000-cell workbook lands at 1–2 MB. A 99-checkpoint history of one large
paste stays small because deltas describe changes, not whole copies.

### 12.6 Naming and defaults

- Extension `.cell` (case-insensitive on read).
- New workbooks default to `untitled.cell` until saved.
- Title shows `cellsheet: Q3_budget.cell [●]` where `●` means unsaved changes,
  `○` means everything is saved.
- Saving over an existing file keeps one previous copy as `.cell.bak`.

---

## 13. Checkpointing and rollback

### 13.1 Policy

```
        edit happens
             │
             ▼
   ┌───────────────────┐   no change    ┌──────────────┐
   │ operation journal │───────────────▶│  do nothing  │
   │  (in memory)      │  (hash match)  └──────────────┘
   └─────────┬─────────┘
             │ timer fires (every 3 min) or a marked event occurs
             ▼
   ┌───────────────────┐
   │  build reverse    │──▶ append to HISTORY ring (max 99)
   │  delta from the   │
   │  journal          │
   └───────────────────┘
```

1. The timer runs **every 3 minutes**.
2. It fires **only if the workbook changed** since the last checkpoint. Change is
   detected with a running hash maintained as edits are applied — not by
   re-serialising the workbook.
3. Marked events create a checkpoint immediately: bulk paste, structural change,
   rollback, and manual (`Ctrl+B`). These are the points worth returning to.
4. The ring holds **99** entries; adding the 100th discards the oldest, which is
   why "max available" grows and then settles.
5. Checkpoints reach the file on save. Until then they exist only in memory and
   index 0 is labelled `(Uncommitted)`.

### 13.2 Labels

| Kind | Label | Created when |
|---|---|---|
| `Initial` | `Initial file load` | A workbook is opened |
| `Sheet initialized` | `Sheet initialized` | A new workbook, or a sheet is added |
| `Auto` | `Auto-Checkpoint` | The timer fires and something changed |
| `RowEdited` | `Row 4 edited` | A row-level edit completes |
| `CellEdited` | `Cell C1 edited` | A single-cell edit worth marking |
| `BulkPaste` | `Bulk Paste (+12 cells)` | A paste of more than one cell |
| `Structural` | `Row 4 deleted`, `Column C inserted` | A structural change |
| `Manual` | `Manual checkpoint` | `Ctrl+B` |
| `Rollback` | `Rolled back to index 5` | A rollback occurred |

Labels come from the operation journal — the program knows *why* something
changed because every edit arrives as a named operation, not a raw write.

### 13.3 Rollback semantics

- **Index 0** is always the current state.
- **Index *k*** is *k* steps in the past; "immediate revert" is index 1.
- Rolling back to *k* applies reverse deltas 1…*k*.
- **The current state is always preserved first.** A rollback creates a new
  checkpoint at index 1 labelled `Rolled back from index k`, so a rollback can
  itself be rolled back. Losing work to a mis-click is not acceptable; this costs
  one history slot and buys complete safety.
- The custom-jump prompt validates 1…max *before* changing state.
- Rolling back with unsaved changes prompts first, because the pre-rollback state
  exists only in memory at that moment.

Worked example — at index 0, roll back to 5:

```
Before                          After
 0: Current State                0: Rolled-back state (was 5)
 1: Auto-Checkpoint              1: Rolled back from index 5   <- the old state 0
 2: Auto-Checkpoint              2: old 6
 3: Auto-Checkpoint              3: old 7
 4: Row 4 edited                 4: old 8
 5: Bulk Paste (+12 cells)       5: old 9
 6: Auto-Checkpoint              ...
```

### 13.4 Undo is a separate mechanism

| | Undo (`Ctrl+Z`) | Rollback |
|---|---|---|
| Granularity | One change | One checkpoint (up to 3 minutes) |
| Lifetime | The current session | Persisted in the file |
| Cost | A few bytes in memory | A reverse delta in the `.cell` file |
| Purpose | "I made a mistake just now" | "Take me back to how this looked" |

Both are built from the same operation journal, so neither is extra bookkeeping.

---

## 14. Modularity and extension points

### 14.1 The honest limit

Go programs are compiled. A running binary cannot read a new `.go` file and grow
a new function. So "add a file" means:

> Add one file to a folder, run `go build`, and the feature exists.
> **No other file in the repository needs editing.**

That is a real and valuable property. Growing features *without rebuilding* would
require an embedded scripting language and is analysed in §14.6.

### 14.2 The extension points

| # | To add… | Drop a file in… | And you get |
|---|---|---|---|
| 1 | A formula function | `internal/formula/functions/` | `=MYFUNC()` in any cell |
| 2 | An arithmetic operator | `internal/formula/ops/` | A new infix or prefix operator |
| 3 | A number format | `internal/cell/formats/` | A new display style |
| 4 | A menu item | `internal/tui/menu/items/` | An entry under File/Edit/Rollback/View |
| 5 | A key binding | `internal/tui/keys/bindings/` | A shortcut mapped to a command |
| 6 | A command | `internal/app/commands/` | An action any binding or menu item can invoke |
| 7 | A file codec | `internal/sheetfile/codec/` | A new format (not used in v1) |
| 8 | A whole screen | `internal/tui/screens/` | A new view |

### 14.3 How self-registration works

```
        program starts
              │
              ▼
   ┌──────────────────────┐
   │  registry package    │◀── functions/sum.go      init(): Register("SUM", …)
   │  (a map, and a lock) │◀── functions/median.go   init(): Register("MEDIAN", …)
   │                      │◀── functions/round.go    init(): Register("ROUND", …)
   └──────────┬───────────┘
              ▼
      the evaluator asks the registry by name
```

Because `sum.go` and `median.go` are in the **same package**, adding a file to
that folder requires zero changes elsewhere. Go compiles all files in a directory
together, so the new `init()` simply runs. There is no list to keep in sync —
which is exactly the failure mode this design avoids.

### 14.4 Walkthrough: adding `=MEDIAN()`

**Step 1.** Create `internal/formula/functions/median.go` with an `init()` that
calls `registry.Register` (see the full example in §7.10 for the shape).
**Step 2.** `go build`. **Step 3.** `=MEDIAN(A1:A20)` works.

Documentation, argument checking, error propagation, panic isolation and parallel
safety are all handled by the registry.

### 14.5 Rules that keep modules from breaking the program

| Rule | Enforced by |
|---|---|
| Functions must be pure — no globals, no I/O, no clock | Review, plus a test running the whole registry concurrently under `-race` |
| Functions must never panic | The evaluator recovers per call and returns `#ERR!` in that one cell |
| Argument count is the registry's job | `MinArgs` / `MaxArgs` |
| Bad argument kinds are caught before the function runs | The registry |
| Avoidable per-call allocation is a bug | Benchmarks in the test suite |
| A module must not reach into the UI or file format | The import rule (§5), enforced by a test |

**One bad module degrades one cell**, not the program.

### 14.6 If you ever want modules without rebuilding

| Approach | Gain | Cost |
|---|---|---|
| **Compile-time files** *(chosen, D10)* | Simplest, fastest, safest, full access to internals, one language | Requires `go build` after adding a file |
| Embedded scripting (Starlark or Lua) | Add a script at runtime; users write functions without Go | A dependency and a second language; 10–100× slower; worse errors; a sandbox to maintain |
| Go plugins (`.so`) | Runtime loading of compiled Go | Linux/macOS only, exact compiler-version matching, effectively unshippable. **Not viable.** |

The registry accepts a script-backed `Spec` without rework, so this decision can
be revisited without undoing anything.

---

## 15. Reliability engineering

### 15.1 Crash safety, in order of importance

1. **Atomic save.** Write `name.cell.tmp` in the same directory, `fsync`, then
   `rename()` over the original. A crash mid-save leaves the old file intact;
   `rename` within a directory is atomic on every filesystem we target.
2. **Terminal restore.** If the program panics or is killed, the terminal must
   not be left in raw mode. Bubble Tea handles the normal paths; we add a
   top-level `recover` and a written instruction to run `reset` in the worst case.
3. **Checksums.** SHA-256 per section, plus a whole-file trailer hash. A damaged
   file is detected and reported by section name.
4. **Backup on overwrite.** The previous version is kept as `name.cell.bak`.
5. **Single-writer lock.** `name.cell.lock` holds the owning PID. A second
   instance opens read-only and says so; a lock whose PID is gone is reclaimed.

### 15.2 Failure table

| Failure | Response |
|---|---|
| Formula function panics | That cell becomes `#ERR!`; the program continues |
| Circular reference | `#CIRC!` on the cycle, detected at graph-build time |
| Corrupt `.cell` file | Report which section failed its checksum; offer the `.bak` |
| `MANIFEST` or `STATE` fails checksum | Do not load; offer the `.bak` |
| Only `HISTORY` fails checksum | Open with a warning and no rollback points — a damaged history must never cost you data |
| Disk full during save | Save aborts, original untouched, clear message |
| Terminal resized to 40 × 10 | "Terminal too small" panel stating the required size |
| Wide characters (CJK, emoji) in cells | Width-aware truncation; columns stay aligned |
| Column narrower than its number | `####`, like Excel, rather than a misleading partial number |
| Interrupted mid-paste | Paste is one atomic transaction: all of it or none |
| Two instances, one file | Second instance refuses to write; opens read-only with a warning |
| Illegal package import | Test suite fails the build |

### 15.3 What the program writes to disk

Exactly three things, all beside the `.cell` file:

- `name.cell` — the workbook
- `name.cell.bak` — the previous version
- `name.cell.lock` — the single-writer lock

No config file, no cache, no journal in v1 (D14). This is what makes "the binary
is all you need" literally true.

### 15.4 Terminal compatibility notes

| Issue | Handling |
|---|---|
| `●` (U+25CF) is ambiguous-width | The title is laid out left-to-right and the remaining space filled with `─`, so a mis-measured `●` shifts only the fill, never the frame. A theme option selects `*` instead. |
| CJK and emoji occupy two columns | All padding and truncation is width-aware (`go-runewidth`); over-long wide text is truncated with `…` |
| Colour depth | True colour → 256 → 16 → none, detected at startup, overridable with `-color` |
| No mouse | Every action has a keyboard equivalent (D14) |
| Terminals without Alt | Menus are also reachable with `F10` and the arrow keys |
| Very narrow terminal | "Terminal too small" panel below 60 × 16 |

### 15.5 Versioning

- **`formatVersion`** — bumped when the byte layout changes.
- **`minReaderVersion`** — the oldest reader that can still make sense of the
  file. An older reader refuses and says so plainly.
- A *newer* reader reads the sections it understands and preserves the rest.

### 15.6 The dependency rule is enforced

Sixteen packages are assigned a layer, and a package may import only packages
strictly below it:

| Layer | Packages |
|---|---|
| 0 | `cell`, `formula/lexer`, `tui/layout`, `tui/render` |
| 1 | `formula/ast`, `formula/registry`, `tui/theme` |
| 2 | `formula/ops`, `grid` |
| 3 | `formula/parser`, `journal`, `clipboard` |
| 4 | `formula/functions`, `formula/eval` |
| 5 | `calc`, `checkpoint` |
| 6 | `sheetfile` |
| 7 | `tui/screens` |
| 8 | `app` |
| 9 | `perf` |
| 10 | `cmd/cellsheet` |

`internal/arch` holds the test, which also:

- fails when a **new package** is not in the table, so nothing escapes the rule
  by accident;
- detects **import cycles**;
- keeps `cell` and `tui/render` as **leaves**, because the value model must not
  learn about the grid and the compositor must stay reusable;
- asserts that only `cmd/cellsheet` imports `app`.

The rule applies to production code. Test files are exempt, deliberately: a test
is its own composition root and legitimately wires together things the production
code must not.

Architecture that is not enforced is architecture that decays. The guard was
verified to fire by temporarily importing `tui/render` from `cell`, which fails
with `internal/cell (layer 0) imports internal/tui/render (layer 0)`.

---

## 16. Performance budgets

Measured on a modest 4-core machine. If a budget is missed it is a bug with a
failing test, not a footnote.

| Operation | Budget |
|---|---|
| Cold start to first frame | < 100 ms |
| Keystroke to screen update | < 16 ms (one frame at 60 Hz) |
| Scroll one row or column | < 8 ms |
| Load 1 MB / 50,000-cell workbook | < 300 ms |
| Save 1 MB / 50,000-cell workbook | < 200 ms |
| Full recalculation of 10,000 formulas | < 500 ms |
| Recalculation after a single edit | < 2 ms |
| Memory, 100,000 populated cells | < 60 MB |

---

## 17. Compatibility

| Target | Status |
|---|---|
| **Linux amd64 / arm64** | **The only supported target for v1.** Static binary, no glibc dependency. |
| macOS 11+ amd64 / arm64 | Kept compiling in the test matrix; untested and unsupported |
| Windows 10+ amd64 | Kept compiling; unsupported |

The build matrix (`GOOS ∈ {linux, darwin, windows} × GOARCH ∈ {amd64, arm64}`
must compile) stays in CI from day one, because keeping the code portable is
nearly free while porting it later is not. Terminal feature detection is runtime,
not build-time.

---

## 18. Cross-sheet formulas: the plan for later

**Designed, deliberately not built** (D12). v1 calculates within a single sheet.

### 18.1 What v1 must do today so "later" is cheap

| # | Rule | Why |
|---|---|---|
| R1 | The lexer **recognises** `Sheet!A1` and `'Sheet name'!A1` as valid tokens | Otherwise the user gets a confusing parse error and tools may mangle the text |
| R2 | A formula is stored in the file as **source text, verbatim** | Opening an old file just works when the feature lands — **no migration, ever** |
| R3 | v1 evaluates an unresolved cross-sheet reference to `#REF!` with the status bar explaining `Cross-sheet references are not enabled yet` | Honest failure instead of a wrong number |

R2 is already guaranteed by §12.3. R1 and R3 are small lexer/parser tasks that
belong in Phase 2. **Do not skip R1** — it is the difference between adding a
feature and migrating every file.

### 18.2 The genuinely hard decision: name or identity?

| Approach | Verdict |
|---|---|
| **Store the name in the formula; rewrite formulas on rename** | ✅ **Chosen.** Formulas stay readable (`=Sheet2!A1`); a rename is one pass over formula cells producing a single undoable operation |
| Store a stable sheet ID (`=#7!A1`) | Formulas become unreadable and uneditable by hand. Rejected |
| Name plus a resolved ID hint | Extra state to keep consistent for no real gain. Rejected |

The parser produces `Ref{SheetName string, Row, Col int}` — the name, not an ID.
The *resolver* converts name → `*Sheet` at graph-build time. On rename, the
workbook walks every formula cell once, rewrites matching `Ref.SheetName`, and
re-serialises: one operation, one undo step, one journal entry.

### 18.3 The heart of the change

Today the dependency graph is per-sheet with node identity `(row, col)`. After,
it is workbook-scoped with identity `(sheetID, row, col)`:

```go
type NodeKey struct {
    Sheet uint32
    Row   uint32
    Col   uint32
}
```

| Concern | Change |
|---|---|
| Precedent extraction | `Ref{Sheet: ""}` resolves to the owning sheet; otherwise via `Workbook.SheetByName` |
| Dirty propagation | Unchanged algorithm; the dirty set may now span sheets |
| Wave planning | **Unchanged** — waves come from the graph, so cross-sheet edges are just extra levels |
| Parallelism | **Unchanged**, and a bonus: cells on different sheets parallelise trivially |
| Cycle detection | Now spans sheets. Works unchanged **once node identity includes the sheet** — the single most important change |
| Recalculation trigger | Editing a sheet invalidates dependents on all sheets, including invisible ones |

Because the graph already keys on a node rather than a cell object, and wave
planning is derived rather than hand-written, this is a change of key type plus
resolution — **not a rewrite of the engine**.

### 18.4 Error and edge-case table

| Situation | Result |
|---|---|
| `=Sheet2!A1`, A1 empty | `0` in arithmetic, blank displayed |
| `=Sheet2!A1`, A1 text | `#VALUE!` (same rule as local text) |
| `=SUM(Sheet2!A1:B5)` containing text | Text skipped, like a local range |
| Sheet2 does not exist | `#REF!`; status bar says `No sheet named "Sheet2"` |
| Sheet2 was renamed | Formulas rewritten on rename; no error |
| Sheet2 was deleted | `#REF!` in every formula that referenced it |
| `A1 = Sheet2!B1` and `Sheet2!B1 = A1` | Both `#CIRC!` |
| `=Sheet2!A1:Sheet3!B5` | `#VALUE!` — a range cannot span sheets |
| `=Sheet1:Sheet3!A1` (3-D) | `#VALUE!` — unsupported, message says so |
| `=sheet2!a1` | Same as `=Sheet2!A1` |
| `='Q3 Budget'!A1` | Works; quoted names allow spaces |
| `='It''s Q3'!A1` | Works; doubled apostrophe is the escape |

### 18.5 Implementation order

| Step | Work |
|---|---|
| 1 | Lexer + parser accept `Sheet!Ref` and quoted names (R1) — **belongs in Phase 2** |
| 2 | Evaluator returns `#REF!` for unresolved cross-sheet refs (R3) — **belongs in Phase 2** |
| 3 | Node identity becomes `(sheetID, row, col)`; graph becomes workbook-scoped |
| 4 | Resolution: `SheetByName` + `Ref` → node |
| 5 | Dirty propagation and waves across sheets |
| 6 | Cross-sheet cycle detection |
| 7 | Sheet rename rewrites formulas as one undoable operation |
| 8 | Sheet delete → `#REF!` with a warning dialog |
| 9 | TUI pointing mode across tabs (insert `Sheet2!` when the tab changes mid-formula) |
| 10 | Documentation and the function-list help |

Steps 3–10 are the feature and can be scheduled whenever it is wanted. **Nothing
requires a file-format change, a data migration, or an engine rewrite.**

---

## 19. Phases

| Phase | What happens | Gate |
|---|---|---|
| **1. Design** | Architecture, calc rules, file format, TUI layout, modularity, this document, the look prototype | ✅ Complete and approved |
| **2. Prototype** | A working program: grid, typing, formulas, menus, save/load, checkpoints | ✅ Complete |
| **3. Testing** | Automated suite; rendering and correctness bugs hunted before the user saw it | ✅ Complete — 214 tests, 22 golden frames, 11 fuzz targets |
| **4. User testing** | The user used it and reported bugs | ✅ Complete — 8 defects found and fixed (§3.2) |
| 5. Polish and documentation | This specification, the README, the build, and the migration into the public repository | ✅ Complete |

### 19.1 Phase 2 progress — complete

Every step of the build plan in §20 is implemented and covered by tests that
pass under `go test -race ./...`, with `go vet ./...` clean.

| Step | Deliverable | State |
|---|---|---|
| 1 | Repo skeleton, `vendor/`, Bubble Tea, alt screen opens and restores cleanly | ✅ Verified under a real pty |
| 2 | Canvas compositor and geometry in `internal/tui/render` and `internal/tui/layout` | ✅ Golden-file tested |
| 3 | `internal/cell`: exact decimal, kinds, inference, formatting | ✅ Table-tested |
| 4 | `internal/grid`: sparse sheets, workbook, naming, growth, resize | ✅ Boundary-tested |
| 5 | Keyboard navigation, scroll-into-view | ✅ Key-driven tests |
| 6 | Cell editing and the formula bar | ✅ |
| 7 | Selection, dashed perimeter, status-bar sum | ✅ |
| 8 | Formula lexer, parser, evaluator, registry, `SUM`, cross-sheet rules R1/R3 | ✅ |
| 9 | `internal/calc`: dependency graph, waves, worker pool, cancellation | ✅ Determinism-tested |
| 10 | Menu bar and the four menus | ✅ All items functional |
| 11 | `internal/sheetfile`: the `.cell` container | ✅ Round-trip, corruption and version tests |
| 12 | Operation journal, 3-minute timer, reverse deltas | ✅ |
| 13 | Rollback menu, `J` prompt, reversible rollback | ✅ |
| 14 | Undo/redo from the journal | ✅ Per-edit, bounded |
| 15 | Clipboard, ranges, tab-separated paste | ✅ |
| 16 | Sheet tabs: add, rename, switch, delete | ✅ Undoable |
| 17 | Structural edits: insert and delete row/column | ✅ Undoable |
| 18 | Resize mode | ✅ Bounds-tested |
| 19 | Polish against the approved look | ✅ Golden render test |

**What runs today:**

```bash
go run ./cmd/cellsheet [file.cell]
```

An interactive spreadsheet with the approved appearance. Cursor movement,
multi-cell selection, editing, exact-decimal arithmetic, `=SUM()`,
dependency-tracked recalculation across a worker pool, copy/cut/paste including
tab-separated paste from another application, sheet tabs, resizable rows and
columns, structural edits, save/open with atomic writes and a `.bak`, a
99-checkpoint rollback ring with a labelled menu and custom-index prompt, and
per-edit undo and redo.

**Known and deliberate limitations**, all as designed in Phase 1:

- Pasting a formula copies it verbatim; relative references are not translated.
  That belongs with absolute addressing, which is out of scope (§18.2).
- Cross-sheet references are lexed and preserved but evaluate to `#REF!` until
  the dependency graph is workbook-scoped (§18).
- No CSV import or export (D15), no system clipboard, no mouse, no config file
  (D14).

---

## 20. Phase 2 build plan

In order. Each step is independently runnable and testable, and leaves the
program working.

| Step | Deliverable | Proves |
|---|---|---|
| 1 | Repo skeleton, `vendor/`, Bubble Tea wired up, alt-screen opens and closes cleanly | The binary runs and restores the terminal |
| 2 | Canvas compositor and geometry moved from the prototype into `internal/tui/render` and `internal/tui/layout` | The approved look renders from the real packages |
| 3 | `internal/cell`: exact decimal, `Dec` arithmetic, kind inference, currency formatting | Table tests for every row of §7.3 and §7.2 |
| 4 | `internal/grid`: workbook, sheets, sparse cells, auto-growth, `ColName`, resize metadata | Growth and naming boundary tests |
| 5 | Keyboard navigation: arrows, Home/End, PgUp/PgDn, Tab/Enter, scroll-into-view | Feed keys to the model, assert the cursor |
| 6 | Cell editing: type, `F2`, Esc, Enter, formula bar | Edit round-trips |
| 7 | Selection: `Shift`+arrows, dashed perimeter, status-bar sum | Selection rectangle tests |
| 8 | `internal/formula`: lexer, parser, AST, evaluator, registry, `SUM`, and rule R1/R3 for cross-sheet syntax | Parser table tests; `=SUM(A1:B1)` evaluates |
| 9 | `internal/calc`: dependency graph, waves, worker pool, cancellation | Sequential == parallel under `-race` |
| 10 | Menu bar, File menu, New / Save / Save As / Exit; `Alt+F` | Menu navigation tests |
| 11 | `internal/sheetfile`: write and read a valid `.cell`, atomic save, `.bak`, checksums | Round-trip and corruption tests |
| 12 | `internal/journal` + `internal/checkpoint`: operation log, 3-minute timer, marked events, reverse deltas | Ring semantics; 99-entry rollover |
| 13 | Rollback menu, `J` prompt, reversible rollback, `Ctrl+B` manual checkpoint | Rollback table from §13.3 |
| 14 | Undo/redo stack from the same journal | Per-edit undo tests |
| 15 | Clipboard: copy/cut/paste, ranges, tab-separated paste from the terminal with auto-growth | Paste shape tests |
| 16 | Sheet tabs: add, rename, switch, delete | Sheet lifecycle tests |
| 17 | `Alt+D` / `Alt+C` delete row/column, `Ctrl+Shift++` insert | Structural edit tests |
| 18 | Resize mode `+`/`-` on headers | Bounds tests |
| 19 | Polish pass against the approved look | Visual comparison |

Steps 1–9 are the ones worth using early; the rest build on them. Cross-sheet
rules R1 and R3 ride along in step 8 because they are cheap insurance.

---

## 21. Phase 3 test plan — delivered

The plan below was written during Phase 1 so that Phase 2 would be built to be
testable. It is kept as written; the *Delivered* column records what actually
exists, and [`PHASE-3-TEST-REPORT.md`](PHASE-3-TEST-REPORT.md) is the full
report.

### 21.1 Unit tests, by package

| Package | Planned coverage | Delivered |
|---|---|---|
| `cell` | Decimal arithmetic, overflow, rounding, kind inference, formatting, property tests | ✅ 20 test functions |
| `grid` | `ColName` boundaries, auto-growth, sparse storage, resize bounds | ✅ 8 |
| `formula/lexer`, `parser` | Table-driven grammar, precedence, associativity, error positions | ✅ 4 |
| `formula/eval` | Promotion table, range versus scalar text, error propagation, cycles | ✅ 10 |
| `calc` | Waves, determinism, cancellation, threshold path | ✅ 15 |
| `sheetfile` | Round-trip, checksum failure, truncation, unknown sections, version refusal | ✅ 21 |
| `checkpoint` | Ring rollover, reverse deltas, reversibility, labels | ✅ 12 |
| `tui/layout` | Geometry at many sizes, closing-rule placement, minimum size | ✅ in `tui/screens` and `perf` |
| `tui/render` | Golden frames, wide-character alignment | ✅ see 21.2 |

### 21.2 Golden-file rendering tests

**Delivered:** 21 frozen frames in `internal/tui/screens/testdata`, across seven
terminal sizes and fourteen screens or states, with universal invariants checked
for every one. Regenerate with `-update`.

### 21.3 Concurrency

**Delivered:** `-race` across every package, a 500-cell parallel fan-out, a
determinism test asserting that sequential and parallel evaluation agree over a
layered workbook, a stress test over 2,000 alternating recalcs, and a purity
check that runs the whole function library concurrently.

### 21.4 Fuzzing

**Delivered:** ten targets — the parser, decimal round-trips, value inference,
the promotion table, the `.cell` reader, container round-trips, formula
evaluation, evaluation determinism, tab-separated paste, and block copy
round-trips. Over 67 million executions this round, which found two serious
bugs (see the report, §5).

### 21.5 Terminal integration

**Delivered:** two tests that drive a real pseudo-terminal — one for the shipped
binary on a clean exit, one for a deliberately panicking model to prove the
restoration guarantee the program relies on.

### 21.6 Benchmarks

**Delivered:** ten benchmarks plus `internal/perf`, which asserts every §16
budget as a test. All nine budgets are met, most by more than an order of
magnitude.

### 21.7 Cross-cutting

| Item | Delivered |
|---|---|
| Import-rule test | ✅ `internal/arch`, verified to fire |
| Registry purity test | ✅ |
| Build matrix, six configurations | ✅ opt-in via `CELLSHEETS_MATRIX=1` |

---

## 22. Glossary

| Term | Meaning |
|---|---|
| **Workbook** | One `.cell` file. Contains sheets and history. |
| **Sheet** | One grid of rows and columns; what a tab shows. |
| **Cell** | One box. Holds a value, a formula, or nothing. |
| **Value** | The typed content of a cell (number, currency, text, error). |
| **Kind** | Which of those five a value is. |
| **Promotion** | Deciding a result's kind from its operands (e.g. Currency × Number → Currency). |
| **Exact decimal** | The fixed-point number representation; never loses cents. |
| **Precedent** | A cell a formula reads. |
| **Dependent** | A cell that reads this one. |
| **Wave** | A set of cells with no dependencies on each other, evaluated in parallel. |
| **Operation journal** | The running list of named edits, feeding both undo and checkpoints. |
| **Checkpoint** | A saved point in time, persisted in the file, that you can roll back to. |
| **Reverse delta** | The instructions that undo one checkpoint step. |
| **Canvas** | The 2-D rune buffer a frame is composed into before being written to the terminal. |
| **Overlay** | A panel composited on top of the base frame (menus, dialogs). |
| **Extension point** | A package folder where adding a file adds a feature. |

---

## 23. Build, install and release

### 23.1 Building

```bash
make            # build bin/cellsheet
make test       # go test ./...
make race       # go test -race ./...
make fuzz       # a short pass over every fuzz target
make install    # go install ./cmd/cellsheet
make clean
```

Or without make:

```bash
go build -o bin/cellsheet ./cmd/cellsheet
```

The binary is **statically linked** (`CGO_ENABLED=0`) and needs nothing else
installed. The Makefile sets that, and strips debug symbols for release builds.

`bin/` is where the executable lives and is not committed; it holds only a
`.gitkeep` so the directory exists after a clone.

### 23.2 Dependencies

One direct dependency, `github.com/charmbracelet/bubbletea v1.3.10`, plus
`golang.org/x/sys` for the pty test. Both are **vendored**, so a clone builds
with no network and no module cache:

```bash
go build -mod=vendor -o bin/cellsheet ./cmd/cellsheet
```

Attribution for the vendored code is in `vendor/`, which Go generates.

### 23.3 Cross-compiling

```bash
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -o bin/cellsheet-linux-arm64  ./cmd/cellsheet
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o bin/cellsheet-darwin-arm64 ./cmd/cellsheet
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/cellsheet.exe         ./cmd/cellsheet
```

Only Linux is supported (D5). The others are kept compiling on purpose, and
`CELLSHEETS_MATRIX=1 go test ./internal/perf/` checks all six combinations.

---

## 24. Final state and verification

### 24.1 What was delivered

A terminal spreadsheet that runs, saves, reloads and rolls back, with the
interface approved in Phase 1 and eight defects found in use fixed.

| | |
|---|---|
| Test functions | 214 |
| Golden frames | 22 |
| Fuzz targets | 11 |
| Benchmarks | 10 |
| Packages under test | 16 of 16 |
| Fuzz executions across the suite | over 200 million |
| `go vet ./...` | clean |
| `gofmt` | clean |
| `go test -race ./...` | green |
| Build matrix | 6 of 6 configurations compile |
| Static binary | ~4.8 MB, no C library |

### 24.2 Measured against the §16 budgets

| Operation | Budget | Measured |
|---|---|---|
| Cold start to first frame | 100 ms | 1.2 ms |
| Keystroke to screen (render 120×40) | 16 ms | 55 µs |
| Scroll one row and render | 8 ms | 56 µs |
| Recalculation after a single edit | 2 ms | 72 µs |
| Full recalculation of 10,000 formulas | 500 ms | 15.2 ms |
| Save 50,000 cells | 200 ms | 28 ms |
| Load 50,000 cells | 300 ms | 6.6 ms |
| Memory, 100,000 populated cells | 60 MB | 10.0 MB |

A 50,000-cell workbook saves to 139 KB. The §16 estimate of 1–2 MB was
conservative.

### 24.3 Defects found and fixed

Recorded here in full. The list, with
the phase that found each:

**Found by fuzzing (Phase 3)**

1. A whitespace-only cell corrupted everything after it in a saved file: the
   writer emitted a source string the reader did not consume, so every following
   record was read from the wrong offset (D29).
2. Sub-cent currency lost precision: `$0.0001` displays as `$0.00`, and three
   code paths reconstructed the value from that display (D28).

**Found by inspection and golden frames (Phase 3)**

3. The cursor could scroll off the bottom of the grid: the viewport used a row
   count that did not match the layout's actual row plan.
4. The title truncated into the frame corner at narrow widths.
5. The viewport scrolled one column further left than necessary.

**Found by the user in use (Phase 4)**

6. `-=SUM(...)` was stored as text and never calculated (D18).
7. Enter on a populated cell overwrote it (D19).
8. `=SUM(A1:A4` was an error rather than being closed (D20).
9. The column header was unreachable, so columns could not be resized (D21).
10. The menu bar did not render: the frame ended with a newline, which scrolled
    the terminal and pushed the top line off the screen (D24).
11. Widening a column changed nothing on screen. Three causes: the geometry
    ignored stored widths, the stretch absorbed a resize to the rightmost
    column, and a held `+` was treated as text (D22, D26).
12. A tall row repeated its text once per line (D23).

**Found while fixing those**

13. `Ctrl+S` and `Ctrl+Q` did nothing while editing a cell (D25).
14. `Cell.IsFormula` was a second, independent check, so a `-=` formula was
    saved as a frozen value (D27).
15. Pressing `↓` on the row header left the header, so consecutive rows could
    not be sized.

**Defects in the test suite itself**

16. The golden comparison could not detect a trailing newline, because an
    out-of-range line and a trailing empty line both compare as `""`. This is
    why defect 10 slipped past 21 golden frames.
17. The tall-row golden fixture set a row tall and left it **empty**, so
    defect 12 was invisible to it.

The last two are the most instructive: in both cases the suite had a blind spot
exactly where the defect was.

### 24.4 Known limitations

Deliberate, all recorded earlier in this document:

- Pasting a formula copies it verbatim; relative references are not translated.
  That belongs with absolute addressing (§18.2).
- Cross-sheet references are lexed, preserved and reported as `#REF!` until the
  dependency graph is workspace-scoped (§18).
- No CSV import or export (D15).
- No system clipboard, no mouse, no configuration file (D14).
- Dates are plain text (§7.3).
- Linux is the only supported platform (D5).

### 24.5 What is not covered by tests

- **No CI.** There is no automated run on push; the commands in §23.1 are run by
  hand.
- **macOS and Windows are compiled, never run.**
- **No terminal-emulator matrix.** Verified under a bare pty, not under GNOME
  Terminal, Konsole, kitty, Alacritty or tmux.
- **The largest workbook measured is 50,000 cells.** A million-row sheet is
  designed for but not measured.
- **No screen-reader or colour-blind review** beyond using shape as well as
  colour.

---

*End of specification. This document describes the delivered program; §3.2 records how it changed along the way.*
