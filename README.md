# cellsheet

A spreadsheet that runs in your terminal. Rows, columns, tabs, formulas and a
99-step rollback history — in one static binary with no runtime dependencies.

```
┌─ File (Alt+F) ── Edit ── Rollback (Alt+R) ── View ────────────────────────────────── cellsheet: Q3_budget.cell [●] ─┐
├──────┬───────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  C1  │ fx: =SUM(A1:B1)                                                                                               │
├──────┴──────┬──────────────────┬──────────────────┬──────────────────┬──────────────────┬────────────────────────────┤
│             │        A         │        B         │       [C]        │        D         │             E              │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ [r1]        │       $50,000.00 │        $1,250.00 ║       $51,250.00 ║                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r2          │       $14,200.50 │          $320.00 │       $14,520.50 │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r3          │Operations        │Equipment         │Subtotal          │Audited?          │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r4          │              450 │            12.75 │          5737.50 │YES               │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r5          │                  │                  │                  │                  │                            │
│ (height: 2) │                  │                  │                  │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r6          │                  │                  │                  │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r7          │                  │                  │                  │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r8          │                  │                  │                  │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r9          │                  │                  │                  │                  │                            │
├─────────────┼──────────────────┼──────────────────┼──────────────────┼──────────────────┼────────────────────────────┤
│ r10         │                  │                  │                  │                  │                            │
│             │                  │                  │                  │                  │                            │
├─────────────┴──────────────────┴──────────────────┴──────────────────┴──────────────────┴────────────────────────────┤
│ [ Sheet1* ]  [ Sheet2 ]  [ + ]                                                                                       │
├──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ READY │ 100R x 26C │ Active: C1 (Currency) │ Checkpoint: 2m 14s [Rev 4/99] │ ^S Save │ ^Q Quit │ +/- Resize Header   │
└──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Install

```bash
git clone https://github.com/badvibecoder/cellsheet
cd cellsheet
make            # builds bin/cellsheet
./bin/cellsheet
```

Or with Go directly:

```bash
go install github.com/badvibecoder/cellsheet/cmd/cellsheet@latest
```

The binary is statically linked and needs nothing else installed. Dependencies
are vendored, so a clone builds with no network.

## Use

```bash
cellsheet                  # a new workbook
cellsheet budget.cell      # open one
cellsheet -color=none      # no escape sequences at all
cellsheet -version
```

Workbooks are `.cell` files. One file holds the workbook *and* its rollback
history, so it is genuinely self-contained — copy it, mail it, or put it in a
repository.

### Keys

| | |
|---|---|
| Arrows | Move the cursor |
| `Shift` + arrows | Extend the selection |
| Typing | Start editing; `Enter` commits and moves down |
| `Enter` on a populated cell | Reopen it for editing |
| `F2` | Edit the active cell |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste |
| `Delete` | Clear the selection |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo, every single change |
| `Ctrl+S` / `Ctrl+Shift+S` / `Ctrl+N` / `Ctrl+O` | Save / Save As / New / Open |
| `Ctrl+Q` | Quit |
| `Ctrl+B` | Checkpoint now |
| `Ctrl+T`, `Ctrl+PgUp` / `Ctrl+PgDn` | Add a sheet, previous / next sheet |
| `Alt+F` `Alt+E` `Alt+R` `Alt+V` | File, Edit, Rollback, View menus |
| `Alt+D` / `Alt+C` | Delete the row / column |
| `+` / `-` on a header | Resize that row or column |

To resize a column, put the cursor on row 1 and press `↑` to reach the column
header; `←`/`→` then walk along the headers. `↑`/`↓` do the same on the row
header, reached with `←` at column A.

### Formulas

```
=SUM(A1:D1)          add a range
=SUM(A1 + D1)        add two cells
=SUM(A1:B1, 10)      ranges, cells and numbers together
=-SUM(A1:B1)         the negative of the sum
-=SUM(A1:B1)         the same thing, spelled the way the brief asked
=A1 * 1.07           arithmetic: + - * / ^ % and comparisons
```

A formula that stops in the middle of a call is closed for you: `=SUM(A1:A4` and
`Enter` gives `=SUM(A1:A4)`.

Copying a formula moves its relative references the way the destination needs:
`=SUM(B1:B5)` copied to column C becomes `=SUM(C1:C5)`. A `$` pins the part it
marks, so `=$B$1` never moves. Cutting a formula leaves its references alone.

### Cell types

`numbers` · `$currency` · `text`. Currency is always shown as `$50,000.00`, and
arithmetic is exact: `$0.10 + $0.20` is `$0.30`, not `0.30000000000000004`.
Currency survives addition, subtraction and scaling by a plain number; it does
not survive multiplication by currency or division by currency, because the units
stop making sense.

## What makes it different

- **Exact decimal money.** No floating-point drift, ever.
- **Rollback done properly.** A checkpoint every three minutes when something has
  changed, plus one on every notable edit, up to 99 of them, all stored in the
  file. Rolling back is itself reversible.
- **It never loses your data.** Saves are atomic (temp file, `fsync`, `rename`),
  every section is checksummed, and the previous version is kept as `.bak`.
- **Add a function by adding a file.** Drop `median.go` into
  `internal/formula/functions/`, rebuild, and `=MEDIAN()` exists. Nothing else
  changes.
- **Fast.** 10,000 formulas recalculate in about 15 ms across a worker pool.

## Development

```bash
make test       # everything, including performance budgets and pty tests
make race       # under the race detector
make short      # skip the slow parts
make fuzz       # a short pass over all eleven fuzz targets
make matrix     # cross-compile every target
make vet        # go vet plus a gofmt check
```

- **[docs/manual.md](docs/manual.md)** — installing, the keyboard, formulas,
  cell types, the file format, and troubleshooting.
- **[docs/deepseekv41-spec.md](docs/deepseekv41-spec.md)** — the complete
  specification: architecture, calculation rules, file format, the measured
  performance budgets, and every defect found in testing.

## Status

Linux only for now. macOS and Windows are kept compiling but are untested.

Not included, deliberately: CSV import or export, cross-sheet formulas (the
design is written and the syntax is preserved, it evaluates to `#REF!`), dates,
a system clipboard, and mouse support.
