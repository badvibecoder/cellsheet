# cellsheet — manual

How to install, use and troubleshoot the program. For the design and the
reasoning behind it, see [`deepseekv41-spec.md`](deepseekv41-spec.md).

---

## 1. Installing

### From a release

Download the binary for your platform and put it somewhere on your `PATH`:

```bash
chmod +x cellsheet
sudo mv cellsheet /usr/local/bin/
```

It is statically linked and needs nothing else — no libraries, no runtime, no
configuration file.

### With Go

```bash
go install github.com/badvibecoder/cellsheet/cmd/cellsheet@latest
```

Requires Go 1.24 or newer.

### From source

```bash
git clone https://github.com/badvibecoder/cellsheet
cd cellsheet
make            # builds bin/cellsheet
./bin/cellsheet
```

Dependencies are vendored, so this works with no network.

---

## 2. Starting it

```bash
cellsheet                  # a new, unsaved workbook
cellsheet budget.cell      # open one; if it does not exist, it is created on save
cellsheet -color=none      # no escape sequences at all
cellsheet -color=16        # limit the palette
cellsheet -version
cellsheet -h               # the flag list and a key summary
```

---

## 3. The screen

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

Top to bottom:

- **Menu bar** — `File`, `Edit`, `Rollback`, `View`, and the file name on the
  right. `●` means unsaved changes, `○` means everything is saved.
- **Formula bar** — the address of the active cell in the box on the left, then
  `fx:` and the cell's contents. Edit here with `F2`.
- **Column headers** — `A` … `Z`, `AA` … `AZ`, `BA` … and so on.
- **Row headers** — `r1`, `r2`, … `(height: n)` appears when a row is taller
  than one line.
- **Grid** — the active cell has heavy `║` bars and its contents are
  highlighted; its row and column headers are shown in `[brackets]`.
- **Sheet tabs** — `[ Sheet1 ]`, `[ Sheet2 ]`, `[ + ]`. `*` marks unsaved
  changes.
- **Status bar** — the mode, the sheet size or the selection sum, the active
  cell and its type, the time to the next checkpoint, and shortcut hints.

---

## 4. Moving around

| Key | Action |
|---|---|
| `↑` `↓` `←` `→` | Move one cell |
| `Home` / `End` | First / last populated column in this row |
| `Ctrl+Home` / `Ctrl+End` | First cell / last populated cell |
| `PgUp` / `PgDn` | One screenful |
| `Tab` / `Shift+Tab` | Right / left |
| `Ctrl+←` / `Ctrl+→` | Next non-empty cell, skipping blanks |
| `Ctrl+G` | Go to a typed address |

Holding an arrow key scrolls smoothly; the cursor is always kept on screen.

### Selecting a range

| Key | Action |
|---|---|
| `Shift` + arrows | Extend the selection from the active cell |
| `Ctrl+A` | Select the used area; again for the whole sheet |
| `Esc` | Collapse back to one cell |

While more than one cell is selected, the status bar shows the count and the
sum. The active cell stays put and the selection grows away from it.

---

## 5. Entering and editing

Type straight into a cell: the first character opens the editor, and `Enter`
commits and moves down. `Esc` abandons the edit.

| Key | Action |
|---|---|
| Any printable character | Start an edit with that text |
| `Enter` | Commit, move down |
| `Shift+Enter` | Commit, move up |
| `Tab` | Commit, move right |
| `Esc` | Abandon the edit |
| `Backspace` | Delete the last character |
| `Ctrl+U` | Clear what you have typed |
| `F2` | Edit the active cell, starting from what is already there |
| `Enter` on a cell with content | Reopen it for editing, so you can amend rather than retype |
| `Delete` | Clear the selection |

`Ctrl+S`, `Ctrl+Q` and `Ctrl+B` work **while you are editing**, committing first.

### One automatic repair

A formula that stops in the middle of a call is closed for you:

```
=SUM(A1:A4      becomes   =SUM(A1:A4)
=((1+2          becomes   =((1+2))
```

This is the only repair the program makes, because it is the only one whose
intent is unambiguous. Brackets inside `"text"` and `'sheet names'` are not
counted, and a formula that already parses is never touched.

---

## 6. What a cell can hold

Type something and the program decides what it is, in this order:

| Looks like | Becomes | Example |
|---|---|---|
| starts with `=` or `-=` | a formula | `=SUM(A1:B1)` |
| nothing but spaces | empty | |
| `$` followed by a number | currency | `$50,000` → `$50,000.00` |
| digits, sign, decimal point, exponent | a number | `450`, `-3`, `12.75`, `1e3` |
| anything else | text | `Operations`, `YES` |

Plain numbers do **not** get thousands separators (`5737.50`); currency always
does (`$50,000.00`).

Treated as text, deliberately: `TRUE`/`FALSE`, `50%` typed into a cell,
`(500)` for a negative, and dates.

---

## 7. Formulas

### Arithmetic

```
=1+2*3          7          multiplication first
=(1+2)*3        9
=2^3^2          512        powers group to the right
=-2^2           -4         the power binds tighter than the minus
=8/4/2          1
=10%            0.1        percent is postfix
=1<2            1          comparisons give 1 or 0
=A1+B1
=A1 * 1.07
```

Order of operations, tightest first: `( )`, `%`, `^`, unary `-`, `*` `/`,
`+` `-`, comparisons.

`-=SUM(A1:A2)` and `=-SUM(A1:A2)` mean the same thing: the negative of the sum.

### Ranges

```
=SUM(A1:D1)         every cell in the rectangle
=SUM(A1 + D1)       two cells added, then summed
=SUM(A1:D1, F1, 10) ranges, cells and numbers together
=SUM(A1:A3)         text and blanks inside a range are skipped
=SUM(A2, 1)         a single text cell here is an error, not skipped
```

A range on its own — `=A1:D1` — is an error, because a range is only meaningful
as a function argument.

### Functions in this version

`SUM`. The registry is built so that adding one is a single file: see the
specification, §14.

### Errors

| Error | Means |
|---|---|
| `#VALUE!` | text where a number was needed, or a bare range |
| `#DIV/0!` | division by zero |
| `#NUM!` | the result is out of range |
| `#CIRC!` | the formula refers to itself, directly or indirectly |
| `#NAME?` | no such function — usually a typo |
| `#REF!` | the reference points nowhere |

---

## 8. Money is exact

Numbers are stored as exact decimals, not floating point, so:

```
=$0.10 + $0.20      $0.30       not 0.30000000000000004
=$50,000.00 * 2     $100,000.00
```

Currency survives addition, subtraction and scaling by a plain number. It does
**not** survive multiplication by currency or division by currency, because
dollars squared and dollars-per-dollar are not dollars — those give a plain
number.

Currency always displays two decimals. An amount smaller than a cent, such as
`$0.0001`, is stored exactly and displays as `$0.00`; it is not rounded away.

---

## 9. Rows and columns

### Resizing

Put the cursor on the header, then use `+` and `-`:

| To reach | Press |
|---|---|
| the row header | `←` at column A |
| the column header | `↑` at row 1 |

While a header is focused, `↑`/`↓` walk rows and `←`/`→` walk columns, so you can
size several in a row. `→` (from the row header) or `↓` (from the column header)
returns you to the grid, as does `Esc`.

Rows are 1 to 20 lines tall and columns 4 to 80 characters wide; beyond that the
key is ignored rather than wrapping around.

A cell's text is placed in the middle of a tall row — the lower of the two middle
lines when the height is even:

```
height 1   text on line 1
height 2   text on line 2
height 3   text on line 2
height 4   text on line 3
height 5   text on line 3
```

### Inserting and deleting

| Key | Action |
|---|---|
| `Alt+D` | Delete the row the cursor is in |
| `Alt+C` | Delete the column the cursor is in |
| Edit ▸ Insert row above | Insert a row |
| Edit ▸ Insert column left | Insert a column |

Deleting asks for confirmation. All of these can be undone.

### Automatic growth

A sheet starts at 100 rows and 26 columns. Pasting data that reaches past the
last row adds another 100 rows; past the last column, another 26 columns.

---

## 10. Sheets

| Key | Action |
|---|---|
| `Ctrl+T` | Add a sheet |
| `Ctrl+PgDn` / `Ctrl+PgUp` | Next / previous sheet |
| Edit ▸ Rename sheet… | Rename the current sheet |
| Edit ▸ Delete sheet | Delete the current sheet |

Each sheet is independent. Calculations stay within one sheet in this version;
`=Sheet2!A1` is accepted and stored but evaluates to `#REF!`.

---

## 11. Copy, cut and paste

| Key | Action |
|---|---|
| `Ctrl+C` | Copy the selection |
| `Ctrl+X` | Cut |
| `Ctrl+V` | Paste at the cursor |

Formulas and types are preserved, so copying a `$50,000.00` currency cell and
pasting it gives currency, not text.

**Pasting from another application** works too: paste tab-separated data and it
fills the grid from the cursor, growing the sheet if it needs to. That is the
way to get a block of numbers out of a browser or another spreadsheet. Values
are typed on the way in, so `$1,250.00` arrives as currency.

A pasted formula is copied exactly as written; relative references are not
translated, so `=SUM(A1:B1)` pasted one row down still refers to row 1.

---

## 12. Files

Workbooks are single `.cell` files. One file holds the workbook **and** its
rollback history, so it is genuinely self-contained: copy it, mail it, or commit
it.

| Key | Action |
|---|---|
| `Ctrl+S` | Save (asks for a name the first time) |
| `Ctrl+Shift+S` | Save As |
| `Ctrl+N` | New workbook |
| `Ctrl+O` | Open |

Saving is atomic: the new file is written alongside, flushed to disk, and only
then moved into place. A crash mid-save leaves the old file intact. The previous
version is kept as `name.cell.bak`.

A lock file, `name.cell.lock`, stops two copies of the program writing to the
same workbook at once.

---

## 13. Checkpoints, rollback and undo

There are two safety nets, and they do different jobs.

**Undo** is per keystroke and lasts for the session.

| Key | Action |
|---|---|
| `Ctrl+Z` | Undo the last change |
| `Ctrl+Y` | Redo |

**Checkpoints** are stored in the file and survive closing it. One is taken
automatically every three minutes if anything has changed, and immediately after
a bulk paste, a structural change, or `Ctrl+B`. Up to 99 are kept.

| Key | Action |
|---|---|
| `Alt+R` | Open the rollback menu |
| `Ctrl+B` | Take a checkpoint now |

The menu lists the current state as `0` and the last nine checkpoints. Pick one
and press `Enter` to go back to it. Press `J` to type any index up to the maximum
shown. **A rollback is itself reversible**: the state you left is kept as index
1, so a mis-click costs nothing.

---

## 14. Menus

| Menu | Key |
|---|---|
| File | `Alt+F` |
| Edit | `Alt+E` |
| Rollback | `Alt+R` |
| View | `Alt+V` |

Arrow keys move, `Enter` chooses, `Esc` closes. If `Alt` does not work in your
terminal, see below.

---

## 15. Full keyboard reference

| Key | Action |
|---|---|
| `Ctrl+Q` | Quit |
| `Ctrl+S` / `Ctrl+Shift+S` | Save / Save As |
| `Ctrl+N` / `Ctrl+O` | New / Open |
| `Ctrl+B` | Checkpoint now |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste |
| `Ctrl+A` | Select all |
| `Ctrl+T` | Add a sheet |
| `Ctrl+PgUp` / `Ctrl+PgDn` | Previous / next sheet |
| `Ctrl+Home` / `Ctrl+End` | First cell / last populated cell |
| `Ctrl+←` / `Ctrl+→` | Next non-empty cell |
| `Ctrl+G` | Go to an address |
| `F1` | Help |
| `F2` | Edit the active cell |
| `Alt+D` / `Alt+C` | Delete row / column |
| `Alt+F` `Alt+E` `Alt+R` `Alt+V` | Menus |
| `+` / `-` on a header | Resize the row or column |
| `Delete` | Clear the selection |
| `Esc` | Cancel, or leave a header |

---

## 16. Troubleshooting

**The screen is a mess of junk characters.** Your terminal is not interpreting
escape sequences. Run `reset`, then start with `cellsheet -color=none`.

**Colours look wrong, or the block characters are misaligned.** Force a palette:
`-color=16`. If box characters are still wrong, your font is missing them; any
font with decent Unicode coverage will do.

**`Alt` does nothing.** Some terminals need "Meta sends Escape" enabled, and
macOS Terminal needs "Use Option as Meta key" in its profile settings. Failing
that, the menus are reachable from the keyboard only via `Alt`, so prefer a
terminal that supports it — every mainstream Linux terminal does.

**"Terminal too small".** The program needs at least 60 columns by 16 rows.
Resize, or use a smaller font.

**The terminal is unusable after a crash.** Run `reset` or `stty sane`. The
program restores the terminal on normal exit and on a panic; a `kill -9` cannot
be intercepted by anything.

**A formula shows `#REF!`.** It refers to another sheet. Cross-sheet calculations
are not enabled in this version.

**A number shows `###`.** The column is too narrow to show it. Widen the column,
or it will stay as hashes rather than show a misleading partial number.

---

## 17. Building from source

```bash
make            # build bin/cellsheet
make run        # build and start it
make test       # all tests, including performance budgets
make race       # under the race detector
make short      # skip the slow tests
make fuzz       # a short fuzz pass over every target
make matrix     # cross-compile every target
make vet        # go vet and a gofmt check
make install    # go install
make clean
```

Cross-compiling for a release:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o bin/cellsheet-linux-arm64 ./cmd/cellsheet
```

---

## 18. Known limitations

- Linux only. macOS and Windows are kept compiling but are untested.
- No CSV import or export. Tab-separated **paste** is the way in and out.
- Formulas stay within one sheet.
- Pasting a formula does not translate relative references.
- Dates are plain text.
- No system clipboard, no mouse, no configuration file.
