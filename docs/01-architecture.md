# 01 — Architecture

How cellsheet is built, why each choice was made, and what it costs.

---

## 1. The shape of the program in one picture

```
                    ┌──────────────────────────────────────────┐
   keystrokes  ───▶ │  internal/tui        event loop, layout,  │
   mouse, paste     │                      menus, dialogs       │
                    └───────────────┬──────────────────────────┘
                                    │ commands (edit this cell, paste here…)
                                    ▼
                    ┌──────────────────────────────────────────┐
                    │  internal/app        the conductor: ties │
                    │                      everything together  │
                    └───┬──────────┬──────────┬────────────┬───┘
                        ▼          ▼          ▼            ▼
              ┌──────────────┐ ┌────────┐ ┌─────────┐ ┌──────────────┐
              │ internal/    │ │ calc/  │ │sheetfile│ │ checkpoint/  │
              │ grid (model) │ │ engine │ │ (.cell) │ │ (history)    │
              └──────┬───────┘ └───┬────┘ └────┬────┘ └──────┬───────┘
                     ▼             ▼           ▼             ▼
              ┌──────────────┐ ┌────────────────────────────────────┐
              │ internal/cell│ │ internal/formula  (+ functions/)    │
              │ value + type │ │ parser, evaluator, dependency graph │
              └──────────────┘ └────────────────────────────────────┘
```

The arrows are strict: **a package may only use the packages below it.**
That single rule is what keeps the program testable and expandable. The test
suite enforces it mechanically (see §11).

---

## 2. Requirements → design response

| Your requirement | How it is met |
|---|---|
| Runs on any system from the last 5–6 years | Go, statically linked, `CGO_ENABLED=0`. No libc, no shared objects, no external programs. Linux amd64/arm64, macOS 11+, Windows 10+. |
| "When the user downloads the binary that should be all they need" | Zero runtime files. Optional config is read if present, never required. Dependencies are vendored into the repo, so even *building* works offline. |
| Modular: "adding a feature is as easy as adding a file to a folder" | Every extension point is a Go package that self-registers in `init()`. Adding `median.go` to `internal/formula/functions/` adds `=MEDIAN()`; no other file changes. See [`05-modularity.md`](05-modularity.md). |
| Rapidly scalable | Sparse cell storage, viewport-only rendering, dependency-tracked recalculation (never a full-sheet rebuild), goroutine worker pool. |
| Multithreaded / async processing | A wave-scheduled calc engine: cells with no dependency on each other are evaluated in parallel across `GOMAXPROCS` goroutines. See §6. |
| Reliability | Atomic saves, checksummed file sections, versioned format, panic isolation per formula, terminal state always restored. See §9. |
| Standalone `.cell` with 99 rollbacks | An operation log inside the file, reverse deltas, `archive`-free custom container. See [`03-cell-file-format.md`](03-cell-file-format.md). |

---

## 3. Technology choices

### 3.1 Language: Go 1.24+

Already your choice, and it is the right one here: static binaries, real
concurrency in the language, fast compile times, and a standard library that
covers compression, checksums, JSON and zipping without any third party.

**Verified on this machine:** Go 1.27.1 is installed; a `CGO_ENABLED=0` build of
the full TUI stack succeeds.

### 3.2 TUI library: Bubble Tea + Lip Gloss

| Option | Verdict |
|---|---|
| **Bubble Tea v1.3.x** (chosen) | Mature, actively maintained, handles the genuinely hard terminal problems: raw mode, Alt/Meta keys, bracketed paste, mouse, resize, and — critically — restoring the terminal if the program exits unexpectedly. |
| Hand-rolled ANSI on the standard library | Full control, but re-implementing key decoding across terminal types and getting crash-safe terminal restore right is weeks of work and a well-known source of broken terminals. |
| `tcell` | Fine library, lower-level than we need; we would still hand-roll the widget layer. |

**Bubble Tea's declared minimum is Go 1.24.0** (verified from its `go.mod`), so
our module declares `go 1.24` and the build floor is Go 1.24.

Bubble Tea v2 exists in beta. We deliberately stay on the stable v1 line.

### 3.3 Rendering: our own compositor, not the library's widgets

Bubble Tea's own renderer diffs lines of a string. A spreadsheet needs more than
that: a floating Rollback panel must be drawn *over* the grid, hiding cells
behind it, while the grid keeps its own borders intact.

So we render into a **2-D grid of styled runes** (a "canvas"), composite panels
on top, and emit the result once per frame. This is exactly what
`prototype/look/canvas.go` already does, and why the prototype is worth keeping.

Cost: we own ~400 lines of compositor. Benefit: overlays, exact border
junctions, and correct handling of wide characters — none of which line-diffing
gives us.

### 3.4 Everything else

| Need | Choice | Why |
|---|---|---|
| Styling/colour | Lip Gloss + `termenv` (comes with Bubble Tea) | Correctly downgrades true colour → 256 → 16 colours. |
| Character widths | `go-runewidth` (comes with Bubble Tea) | CJK and emoji occupy two columns; ignoring this breaks every column. |
| Compression | `compress/flate` (stdlib) | No dependency. |
| Checksums | `crypto/sha256` (stdlib) | No dependency. |
| Money arithmetic | **Our own fixed-point decimal**, ~250 lines | See [`02-calculation-rules.md`](02-calculation-rules.md). Exact cents, no floating-point drift, no dependency. |
| Config format | JSON (stdlib) | No dependency, easy to explain. |

**Total third-party dependencies: one library (Bubble Tea) and its tree.**
All vendored into `vendor/` so builds work with no network.

---

## 4. Repository layout

Folders marked ★ are **extension points** — adding a file there adds a feature.

```
cellsheet/
├── go.mod
├── vendor/                     third-party code, committed for offline builds
├── cmd/
│   └── cellsheet/
│       └── main.go             the only main(); parses flags, starts the app
│
├── internal/
│   ├── app/                    composition root: wires everything, owns lifetime
│   │   └── commands/       ★   user-invokable actions (for a future palette)
│   │
│   ├── cell/                   value types, type inference, formatting
│   │   └── formats/        ★   number/currency display formats
│   │
│   ├── grid/                   the model: sheets, rows, columns, sparse cells
│   │
│   ├── formula/                parsing and evaluation
│   │   ├── ast/                syntax tree
│   │   ├── lexer/
│   │   ├── parser/
│   │   ├── eval/               evaluator + dependency graph
│   │   ├── ops/            ★   arithmetic operators and type coercion
│   │   └── functions/      ★   one file per function: sum.go, median.go, …
│   │
│   ├── calc/                   async engine: waves, worker pool, cancellation
│   │
│   ├── tui/                    everything visible
│   │   ├── render/             the canvas compositor
│   │   ├── layout/             geometry: where every line goes
│   │   ├── screens/            grid screen, dialogs
│   │   ├── menu/               menu bar; menu items register themselves
│   │   │   └── items/      ★
│   │   ├── keys/               key bindings; bindings register themselves
│   │   │   └── bindings/   ★
│   │   └── theme/              colour tokens
│   │
│   ├── sheetfile/              the .cell container: read, write, migrate
│   │   └── codec/          ★   alternative encodings
│   │
│   ├── checkpoint/             history ring, diff/patch, the 3-minute timer
│   │
│   ├── journal/                the operation log (what changed, and why)
│   │
│   └── platform/               OS-specific bits, isolated behind one interface
│
├── docs/                       these documents
├── prototype/
│   └── look/                   the Phase 1 look prototype
└── testdata/                   golden files, sample .cell workbooks
```

### Why this shape

- **`internal/`** — Go forbids anything outside the module from importing these.
  It is a compiler-enforced "private" marker. Nothing here is a public API we
  must keep stable.
- **`cmd/cellsheet`** — one binary, one `main`. Deliberately empty of logic so
  that the whole program can be tested without launching a process.
- **`grid` vs `cell` vs `formula`** — three genuinely different concerns:
  *where* data lives, *what* data is, and *how* it is computed. They change for
  different reasons and are tested separately.
- **`platform`** — everything OS-specific (file locking, "open in editor",
  terminal detection) sits behind one small interface, so 95% of the code is
  portable by construction.

---

## 5. The core data model

### 5.1 Sparse, not dense

A spreadsheet is mostly empty. Storing 1,000,000 × 16,384 cells densely would
need ~130 GB. So cells are stored sparsely:

```go
// internal/grid/sheet.go  (illustrative)
type Sheet struct {
    name    string
    cells   map[CellRef]cell.Cell  // only cells that exist
    rows    map[uint32]RowMeta     // only rows that differ from default
    cols    map[uint32]ColMeta     // only columns that differ from default
    rowN    uint32                 // how many rows currently exist
    colN    uint32                 // how many columns currently exist
}

type CellRef struct{ Row, Col uint32 }
```

- A sheet of 100×26 with 8 populated cells stores 8 cells, plus two small maps.
- Lookup is O(1); the renderer only ever asks about visible cells.
- Default row height and column width live in the sheet, not per row/column,
  so `rows`/`cols` stay empty until you actually resize something.

**Limits** (safety rails, not design constraints): 1,048,576 rows × 16,384
columns, matching Excel, so that any future export lands somewhere sane.
Reaching a limit produces a clear message, never a silent truncation.

### 5.2 Auto-growth

- Sheet starts at **100 rows × 26 columns (A–Z)**.
- Pasting data that reaches past the last row adds **100 more rows**.
- Pasting data that reaches past the last column adds **26 more columns**.
- Growth is computed from the *shape of the paste*, in one step, before any
  cell is written — so a 500-cell paste causes one resize, not 500.

Column naming is Excel's bijective base-26: `A…Z, AA…AZ, BA…BZ, … ZZ, AAA…`.
The generator is already implemented and tested in the prototype
(`colName` in `prototype/look/screen.go`), including the `Z → AA` and
`ZZ → AAA` boundaries where naive modulo arithmetic fails.

### 5.3 Row and column resizing

Row height and column width are per-row/per-column integer overrides, exactly as
you specified:

- Cursor on a **row header** + `+`/`-` → that row grows/shrinks by one line.
- Cursor on a **column header** + `+`/`-` → that column grows/shrinks by one
  character.

Bounds: height 1–20 lines, width 4–80 characters. Out-of-range presses are
ignored (no error, no wrap-around) because holding a key should not destroy the
layout.

The geometry keeps **prefix sums** of row heights and column widths, so
"which row is under the cursor at screen line 37?" is answered in O(log n)
rather than by walking every row. This is what keeps scrolling smooth even at
100,000 rows.

---

## 6. The calculation engine

This is the part your "multithreading" requirement mostly lives in.

### 6.1 Dependency tracking, not full recalculation

Every formula cell records what it reads. Editing `A1` recomputes only the cells
that transitively depend on `A1` — not the other 2,599.

When a formula is entered, the parser produces a list of precedents
(`A1`, `B1`, `Sheet2!C4`). The engine stores:

- `dependents[A1] = {C1, F7, …}` — who must be recomputed when A1 changes.
- `precedents[C1] = {A1, B1}` — what C1 must wait for.

Cycles are detected when the graph is built, not at evaluation time, and the
offending cells become `#CIRC!` instead of hanging the program.

### 6.2 Wave scheduling across goroutines

After an edit, the engine computes the dirty set and sorts it into **waves**:

```
Wave 0:  C1 = SUM(A1:B1)          ← A1,B1 changed; no formula deps
Wave 1:  E1 = C1 * 1.07           ← needs C1
Wave 2:  E2 = E1 - D1             ← needs E1
```

Cells **inside** a wave have no dependency on each other, so they are evaluated
concurrently. Cells **between** waves are strictly ordered. This gives full
parallelism with zero risk of reading a half-computed value, and it is
deterministic: the answer never depends on scheduling.

```
        ┌───────────────┐
        │  dirty set    │
        └───────┬───────┘
                ▼
        ┌───────────────┐     wave 0 ─▶ [worker][worker][worker][worker]
        │ wave planner  │     wave 1 ─▶ [worker][worker]
        │ (topological) │     wave 2 ─▶ [worker]
        └───────────────┘
```

### 6.3 The worker pool

- Pool size = `min(runtime.NumCPU(), 8)` by default, user-tunable.
- A single shared pool per running application, not one per recalculation —
  spawning goroutines per edit would cost more than the arithmetic.
- **Threshold:** if the dirty set is smaller than ~64 cells, evaluation runs
  inline. Fanning 3 cells out to 8 workers is slower than just computing them.
  This is why a 50-calculation sheet spins up threads and a 2-cell edit does not.
- **Cancellation:** each recalculation carries a generation number. If you edit
  again mid-flight, the old generation is abandoned and its results are
  discarded. Stale results never reach the screen.
- **Panic isolation:** each function call is wrapped so that a broken formula
  function yields `#ERR!` in one cell rather than killing the program. This also
  protects you from a bad module you add later, which matters a lot once
  functions are drop-in files.

### 6.4 What the UI does while calculating

The event loop never blocks. A recalculation is a Bubble Tea command: the UI
stays responsive, and a `recalcDone` message updates the affected cells. For
typical sheets the entire cycle is sub-millisecond and invisible; for a
pathological sheet the status bar shows a spinner rather than freezing.

---

## 7. The rendering pipeline

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

Three properties matter:

1. **Only visible cells are touched.** Cost is proportional to the terminal
   size (say 120×40 = 4,800 cells), never to the size of the sheet. A 100-row and
   a 1,000,000-row workbook render in the same time.
2. **Panels composite over the base.** The Rollback menu is drawn last and hides
   what is behind it, exactly as in the mock.
3. **One write per frame.** The whole screen is assembled in memory and emitted
   in a single `write` call, which eliminates tearing and flicker.

Each region of the frame is produced by a function that takes the geometry and
returns nothing but draws into the canvas — see [`04-tui-layout.md`](04-tui-layout.md).

---

## 8. Input model

All input becomes a **command** (`EditCell`, `PasteRange`, `ResizeRow`,
`RollbackTo`, …). The event loop maps keys to commands, and commands are applied
to the model. Nothing in the key handler knows how the model works; nothing in
the model knows about keyboards. This is what makes the UI testable without a
terminal — we feed a list of keys to the model and assert the resulting state.

Keys that terminals genuinely disagree about are handled explicitly rather than
assumed. Three cases you should know about, because they affect your spec:

| Key | Reality | Plan |
|---|---|---|
| `Ctrl+Shift+D` | Most terminals send the *same byte* as `Ctrl+D`. It is not reliably distinguishable. | Detect the modern "kitty keyboard protocol" when available; otherwise fall back to a second binding. Needs your decision — see [`06-open-questions.md`](06-open-questions.md). |
| `Ctrl+C` | In raw mode we receive it as a key, so copy works — but then it no longer interrupts. | Copy wins; `Ctrl+Q` quits. |
| `Alt+F` | Sent as `Esc` then `f`, which Bubble Tea reports as `alt+f`. Fine everywhere except terminals with "Meta sends Escape" disabled. | Documented, with a menu-bar click fallback. |

---

## 9. Persistence, checkpoints and reliability

Full detail is in [`03-cell-file-format.md`](03-cell-file-format.md); the
architectural points:

- Every edit is appended to an in-memory **operation log** (`journal`), which
  records *what* changed and *why* ("Row 4 edited", "Bulk Paste (+12 cells)").
- The **checkpoint timer** fires every 3 minutes. It compares a cheap state hash
  and does nothing if nothing changed. When something did change, it converts
  the pending operation log into one **reverse delta** and files it as a
  checkpoint.
- Reverse deltas mean rolling back one step is cheap and rolling back 47 steps
  is 47 cheap steps — no full copies are kept.
- The `.cell` file always contains the current state plus up to 99 reverse
  deltas, so it is genuinely standalone.

**Crash safety, in order of importance:**
1. **Atomic save.** Write to `name.cell.tmp` in the same directory, `fsync`,
   then `rename()` over the original. A crash mid-save leaves the old file
   intact. `rename` within a directory is atomic on every filesystem we target.
2. **Terminal restore.** If the program panics or is killed, the terminal must
   not be left in raw mode. Bubble Tea handles the normal paths; we add a
   `defer` on a top-level recover, plus a written instruction to run `reset` in
   the worst case.
3. **Checksums.** Every section of the file carries a SHA-256. A damaged file is
   detected and reported with the section name, rather than silently
   misinterpreted.
4. **Backup on overwrite.** The previous version is kept as `name.cell.bak`.
5. **Single-writer lock.** A lock file prevents two copies of cellsheet from
   clobbering the same workbook.

**Undo is a separate mechanism, and it is per-edit.** The operation log also
feeds an in-memory undo stack that records every individual change in a session,
with no time granularity at all — one keystroke, one undo step. `Ctrl+Z` walks
that stack; `Ctrl+Y` walks back up it. Checkpoints and undo do different jobs:

| | Undo (`Ctrl+Z`) | Rollback |
|---|---|---|
| Granularity | One change | One checkpoint (up to 3 minutes) |
| Lifetime | The current session | Persisted in the file |
| Cost | A few bytes in memory | A reverse delta in the `.cell` file |
| Purpose | "I made a mistake just now" | "Take me back to how this looked" |

Both are built from the same operation log, so neither is extra bookkeeping.

---

## 10. Performance budgets

Numbers we will hold ourselves to, and test in Phase 3. Measured on a modest
4-core machine.

| Operation | Budget |
|---|---|
| Cold start to first frame | < 100 ms |
| Keystroke to screen update | < 16 ms (one frame at 60 Hz) |
| Scroll one row/column | < 8 ms |
| Load 1 MB / 50,000-cell workbook | < 300 ms |
| Save 1 MB / 50,000-cell workbook | < 200 ms |
| Full recalculation of 10,000 formulas | < 500 ms |
| Recalculation after a single edit | < 2 ms |
| Memory, 100,000 populated cells | < 60 MB |

If a budget is missed, it is a bug with a test that fails — not a footnote.

---

## 11. Reliability engineering, beyond the happy path

| Failure | Response |
|---|---|
| Formula function panics | That cell becomes `#ERR!`; program continues. |
| Circular reference | `#CIRC!` on the cycle, detected at graph-build time. |
| Corrupt `.cell` file | Report which section failed its checksum; offer the `.bak`. |
| Disk full during save | Save aborts, original untouched, clear message. |
| Terminal resized to 40×10 | "Terminal too small" screen with the required size. |
| Wide characters (CJK, emoji) in cells | Width-aware truncation; columns stay aligned. |
| Column narrower than its number | `####` like Excel, rather than a misleading partial number. |
| Interrupted mid-paste | Paste is applied as one atomic transaction; either all of it or none. |
| Two instances, one file | Second instance refuses to write; opens read-only with a warning. |
| Illegal import (layer violation) | Test suite fails the build. |

---

## 12. Portability

| Target | Status |
|---|---|
| **Linux amd64 / arm64** | **The only supported target for the first release.** Static binary, no glibc dependency. |
| macOS 11+ amd64 / arm64 | Code is portable and kept compiling in the test matrix, but untested and unsupported until you ask for it. |
| Windows 10+ amd64 | Same: kept compiling, unsupported. `cmd.exe` has weak Alt-key and Unicode support regardless. |

The build matrix (`GOOS × GOARCH` must all compile) stays in the test suite from
day one, because keeping the code portable is nearly free while porting it later
is not. Terminal *feature* detection (colour depth, Unicode support, mouse) is
runtime, not build-time.

---

## 13. Risks, stated plainly

| Risk | Severity | Mitigation |
|---|---|---|
| `Ctrl+Shift+D` is not deliverable on most terminals | Medium — it is in your spec | Fallback binding + terminal-protocol detection. Needs your call. |
| 99 rollbacks × large sheets makes files big | Medium | Reverse deltas + compression; a checkpoint of a 2-cell edit costs bytes, not megabytes. |
| Bolt-on function modules could be slow if each call allocates | Low | Function signature takes reusable buffers; benchmarked in Phase 3. |
| Bubble Tea v2 migration later | Low | TUI is isolated behind `internal/tui`; a migration touches one layer. |
| "Add a file to add a feature" cannot mean *runtime* loading | Medium — expectation setting | Compile-time registration, described honestly in [`05-modularity.md`](05-modularity.md). Needs your confirmation. |
| Very large pastes (100k+ cells) could freeze the UI | Low | Paste is chunked and applied in a background command with progress. |

---

## 14. What Phase 2 will build

In order, each step independently runnable and testable:

1. Canvas compositor + layout engine (port from the prototype).
2. Grid model with sparse storage, auto-growth, row/column resizing.
3. Keyboard navigation and cell editing.
4. Type inference and currency formatting.
5. Formula parser and `=SUM()`, evaluated synchronously.
6. Calc engine with waves and the worker pool.
7. Menu bar, File menu, Save / Save As / New / Exit.
8. `.cell` read/write.
9. Operation log, 3-minute checkpoints, Rollback menu.
10. Clipboard copy/paste, including multi-cell and tab-separated paste.
11. Sheet tabs: add, rename, switch.
12. Delete row / delete column.

Steps 1–5 are the ones worth using early; the rest build on them.
