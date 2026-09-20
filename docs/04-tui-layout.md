# 04 — TUI layout specification

Exact measurements for every part of the screen, derived from your mock and
rendered by `prototype/look`. Nothing here is approximate: the prototype computes
each position from these rules, so what you see in `layout/preview-*.txt` is what
the real program will draw.

---

## 1. The screen, top to bottom

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

| Region | Rows occupied | Notes |
|---|---|---|
| Menu bar | 1 | Menu titles left, filename right. |
| Rule | 1 | Has a tee where the reference box begins. |
| Formula bar | 1 | Reference box, then `fx:` and the formula source. |
| Rule | 1 | Reference box closes (`┴`); column tees open (`┬`). |
| Column headers | 1 | Centred column letters. |
| Rule | 1 | `┼` at every column boundary. |
| Grid | *(remaining)* | See §3. |
| Closing rule | 1 | `┴` at every column boundary. |
| Sheet tabs | 1 | `[ Sheet1* ]  [ Sheet2 ]  [ + ]`. |
| Rule | 1 | |
| Status bar | 1 | Segments separated by `│`. |
| Bottom border | 1 | |

**Fixed overhead: 10 rows.** A 32-row terminal therefore gives 22 rows to the
grid. Minimum supported size is **60 × 16**; below that the program shows a
"Terminal too small" panel that states the required dimensions.

---

## 2. Horizontal measurements

| Element | Width | Rule |
|---|---|---|
| Left/right frame | 1 each | |
| Row-number gutter | **13** | Must fit `r1000000` and the `(height: 99)` marker. |
| Reference box | **6** | Fits `C1` and `AA100`; grows if the sheet needs it. |
| Column interior | **18** | Default. Per-column override 4–80. |
| Column total | **19** | Interior + one border. |

```
 x=0        14                              120
  │         │                                │
  └─────────┴────────────────────────────────┴──
    gutter     column A ...                 right
     (13)                                   frame
```

The first column's left border sits at `x = 1 + gutterWidth = 14`.

**The last visible column stretches.** Remaining horizontal space is given to
the final column rather than left as a gap, which is why column E is wider than
A–D in the preview. This is deliberate and matches your mock.

At 120 columns that yields exactly **5 visible columns** (A–E), with E getting
the 10 leftover characters.

---

## 3. The grid

Each row occupies **its height in lines, plus one separator line**. A row of
height 1 takes 2 screen lines; the tall r5 in your mock (height 2) takes 3.

Row selection is greedy top-down; if a single line would be left over at the
bottom, it is absorbed into the last visible row so that the closing rule lands
exactly on the bottom border of the grid area. The result is always a clean,
closed box — no doubled rules, no ragged edge.

**Only visible rows and columns are drawn.** Cost is proportional to the
terminal size, never to the size of the sheet.

### Vertical placement of text

A cell's text is drawn on one line of its row, chosen by the row's height: line
`height/2 + 1`. So height 1 shows the text on line 1, height 2 on line 2,
height 3 on line 2, height 4 on line 3, and height 5 on line 3 — the middle, or
the lower of the two middle lines when the height is even. Every other line of a
tall row is blank.

### Inside a row

```
│ r5          │                  │                  │
│ (height: 2) │                  │                  │
  ▲             ▲
  │             └─ cells: values padded to 17 columns + 1 trailing space
  └─ row number on the first line, height marker on the last line when height > 1
```

Values are laid out in **width − 1** columns with a single trailing blank. That
one-character right margin is what makes the grid readable and is why
`$50,000.00` sits one space clear of its right border.

### The active cell

The cursor is marked in **three** places at once, so it is findable whichever
part of the screen you are looking at:

```
│             │        A         │        B         │       [C]        │
├─────────────┼──────────────────┼──────────────────┼──────────────────┤
│ [r1]        │       $50,000.00 │        $1,250.00 ║       $51,250.00 ║
   ▲                                                 ▲                ▲
   │                                                 │                │
   │                                    heavy bars ──┘                │
   │                                    (double vertical borders)     │
   └─ the row header is bracketed                        bracketed ────┘
                                                          column header
```

| Cue | Where | Why |
|---|---|---|
| `║` heavy bars | Both vertical borders of the cell | Visible even when the cell is empty. |
| Reverse video on the contents | Inside the cell | Makes a populated cell unmistakable. |
| `[r1]` brackets | Row-number gutter | Shows the cursor's row. |
| `[C]` brackets | Column header | Shows the cursor's column. |
| Reference box + status bar | Top and bottom | `C1` and `Active: C1 (Currency)`. |

**Brackets rather than colour alone.** Colour alone would be invisible on a
monochrome terminal and hard to read for colour-blind users. The brackets are
plain ASCII, cost no width, and are reinforced by colour (cyan headers, reversed
cell) when colour is available.

The cell's **width never changes** when the cursor moves. The mock in the
original brief drew the selected cell's interior two characters narrower; that
would make every cell in the column appear to shift on each keypress, so it is
deliberately not reproduced (§8).

### 3.1 Moving the cursor

| Key | Action |
|---|---|
| `↑` `↓` `←` `→` | Move one cell. |
| `Home` / `End` | First / last populated column in the row. |
| `Ctrl+Home` / `Ctrl+End` | First cell / last populated cell of the sheet. |
| `PgUp` / `PgDn` | Move one screenful of rows. |
| `Tab` / `Shift+Tab` | Move right / left, wrapping to the next row. |
| `Enter` / `Shift+Enter` | Move down / up (committing an edit first). |
| `Ctrl+←` / `Ctrl+→` | Jump to the next non-empty cell (skips blanks). |
| `Ctrl+G` | Go to a typed address. |

The cursor is scrolled into view automatically. Only the visible window is
redrawn, so holding an arrow key costs the same as pressing it once.

### 3.2 Selecting a range

| Key | Action |
|---|---|
| `Shift` + arrows | Extend the selection from the active cell. |
| `Ctrl+Shift+←` / `→` | Extend to the next non-empty cell. |
| `Ctrl+A` | Select the whole used area; a second press selects the sheet. |
| `Shift+Space` / `Ctrl+Space` | Select the whole row / column. |
| `Esc` | Collapse back to the single active cell. |

The **active cell stays put** while the selection grows away from it — Excel's
behaviour, and the reason the selection has both an anchor and a moving corner.

In colour, the selected block is tinted with a dark slate background. In
monochrome, its perimeter is drawn with dashed rules (`┄` horizontal, `┆`
vertical), so the selection is unmistakable without colour:

```
│             ┌┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┐
│ [r1]        ║       $50,000.00 ║        $1,250.00 ┆
│ r2          ┆       $14,200.50 │          $320.00 ┆
│             └┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┘
   active cell A1, kept in heavy bars
```

While more than one cell is selected the status bar shows the count and the sum:

```
│ READY │ Sum: $65,770.50 · 2R x 2C selected │ Active: A1 (Currency) │ … │
```


### Row and column headers

- Row header: ` r<n>` left-aligned, on the first line of the row.
- Height marker: `(height: n)` centred, on the last line, only when height > 1.
- Column header: the column name centred in the column's interior width.
- Active row/column headers are highlighted in cyan.

---

## 4. The formula bar

```
│  C1  │ fx: =SUM(A1:B1)                                      │
```

- The reference box holds the active cell address, centred.
- `fx:` is followed by one space, then the cell's **source**: the formula text if
  it has one, otherwise the raw entered value.
- Editing a formula happens here. `F2` or typing `=` moves the caret into the
  formula bar; `Esc` restores the previous contents; `Enter` commits and moves
  down.

---

## 5. The status bar

Segments, in order, separated by ` │ `:

| Segment | Example | Notes |
|---|---|---|
| Mode | `READY` | Also `EDIT`, `POINT`, `MENU`, `ROLLBACK`, `RESIZE`. |
| Selection summary | `Sum: $51,250.00 · 100R x 26C` | Sum appears when more than one cell is selected. |
| Active cell | `Active: C1 (Currency)` | Kind comes from the calculation rules. |
| Checkpoint | `Checkpoint: 2m 14s [Rev 4/99]` | Time to the next auto-checkpoint, and history depth. |
| Key hints | `^S Save`, `^Q Quit`, `+/- Resize Header` | Context-sensitive. |

The mode word is green in normal use and changes colour for `EDIT`, `RESIZE` and
`ROLLBACK` so you can tell at a glance what the keyboard will do.

---

## 6. Sheet tabs

```
│ [ Sheet1* ]  [ Sheet2 ]  [ + ]                                   │
```

- The active tab is cyan and bold; inactive tabs are dim.
- `*` marks unsaved changes, so `[ Sheet1* ]` is the active sheet with pending
  edits.
- `[ + ]` adds a sheet.
- Double-clicking a tab renames it; middle-click or a menu item closes it.

---

## 7. Overlays

Overlays are composited **on top of** the base frame by the canvas, so they
genuinely hide what is behind them. This is the reason the renderer is a
compositor rather than a line-differ.

### 7.1 File menu (`Alt+F`)

Hangs from the menu bar beneath the word `File`. Items with their shortcuts
right-aligned; separators as full-width rules; the highlighted item drawn in
reverse video.

```
┌─ File (Alt+F) ── Edit ── Rollback (Alt+R) ── View ──── cellsheet: Q3_budget.cell [●] ─┐
├─┌───────────────────────────┐──────────────────────────────────────────────────────────┤
│ │  New                Ctrl+N│                                                          │
├─│  Open…              Ctrl+O│──────────────────────────────────────────────────────────┤
│ │───────────────────────────│
│ │  Save               Ctrl+S│
│ │  Save As…     Ctrl+Shift+S│
│ │───────────────────────────│
│ │  Export CSV…              │
│ │  Reload from disk         │
│ │───────────────────────────│
│ │  Exit               Ctrl+Q│
│ └───────────────────────────┘
```

Navigation: `↑`/`↓`, `Enter` to choose, `Esc` to close, or type the underlined
letter. `Alt+F` again closes it.

### 7.2 Rollback menu (`Alt+R`)

Centred horizontally, opening just below the formula bar. Index 0 is the current
state and is highlighted.

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

- Shows the **last 9 plus current**, exactly as you specified.
- `J` (or selecting the last line) opens the number prompt, which states the
  maximum available index:

```
     ┌────────────────────────────────────────┐
     │ Rollback                               │
     │ Rollback to index (1-47): 4█           │
     │ Enter = go   Esc = cancel              │
     └────────────────────────────────────────┘
```

- The maximum is validated before anything changes; an out-of-range number is
  refused with the valid range restated.

### 7.3 Resize mode

Pressing `+`/`-` with the cursor on a **row header** or **column header** resizes
that one row or column by exactly one line or character. The status bar switches
to `RESIZE ROW r5 (height 2)  +/-`, and the header being resized is highlighted.
`Esc` leaves the mode. Limits are 1–20 lines for rows and 4–80 characters for
columns; further presses are ignored rather than wrapping around.

---

## 8. Where I deviated from your mock, and why

Every difference below is deliberate. Tell me if you would rather have the mock's
behaviour.

| # | Mock | Prototype | Why |
|---|---|---|---|
| 1 | The active cell's interior appears 2 characters narrower (`║    $51,250.00  ║`) | Interior width unchanged; borders swapped to `║`, contents reversed, headers bracketed | Cells must not shift when the cursor moves. In a real grid this is instantly noticeable and looks broken. |
| 2 | No active-cell indicator beyond the double bars | Added `[r1]` / `[C]` header brackets | A highlight that exists only as colour disappears on monochrome terminals and is weak for colour-blind users. |
| 3 | All text right-aligned | **Text left-aligned, numbers right-aligned** (decision D11) | Excel's convention, and easier to scan down a column of labels. `-text-right` still reproduces the sketch. |
| 4 | Rollback panel's internal divider drawn with `\|----\|` | Proper box-drawing rule `├────┤` | Consistent with the rest of the frame; the mock's version was hand-drawn. |
| 5 | Values sit flush against the right border | One trailing space | The mock shows this too; called out because it is a real rule, not an accident. |
| 6 | No bottom border visible under the status bar | `└────┘` bottom border | The mock implies it; the frame must close. |
| 7 | `●` in the title | Same, but `○` when saved | Ambiguous-width character; see §9. |

---

## 9. Terminal compatibility notes

| Issue | Handling |
|---|---|
| `●` (U+25CF) is "ambiguous width" — some terminals draw it 2 columns wide | The title is laid out left-to-right and the remaining space is filled with `─`, so a mis-measured `●` shifts only the fill, never the frame. A config option selects `*` as an alternative marker. |
| CJK and emoji occupy 2 columns | All padding and truncation is width-aware (`go-runewidth`). Wide text too long for a cell is truncated with `…`. |
| Colour depth | True colour → 256 → 16 → none, detected at startup and overridable with `-color`. No colour is required for any information to be legible. |
| No mouse | Every mouse action has a keyboard equivalent. Mouse is a convenience, never a requirement. |
| Terminals without Alt | Menu bar items can also be reached with `F10` and the arrow keys. |
| Very narrow terminals | Below 60 × 16, a clear "too small" panel instead of a garbled screen. |

---

## 10. The previews

Generated by `go run ./prototype/look -scene=all -out=docs/layout`:

| File | Shows |
|---|---|
| [`layout/preview-main.txt`](layout/preview-main.txt) | The main screen with your sample data and the active cell in C1. |
| [`layout/preview-nav.txt`](layout/preview-nav.txt) | **Three frames** showing the cursor moved with the arrow keys. |
| [`layout/preview-select.txt`](layout/preview-select.txt) | A selected range (A1:B2) with the sum in the status bar. |
| [`layout/preview-menu.txt`](layout/preview-menu.txt) | The File menu open. |
| [`layout/preview-rollback.txt`](layout/preview-rollback.txt) | The Rollback menu open. |
| [`layout/preview-jump.txt`](layout/preview-jump.txt) | The custom rollback index prompt. |
| [`layout/preview-small.txt`](layout/preview-small.txt) | The too-small-terminal panel. |

Regenerate them at any size, with or without colour:

```bash
go run ./prototype/look -scene=all -w=140 -h=40
go run ./prototype/look -scene=main -color=256      # run this one in a real terminal
go run ./prototype/look -scene=nav  -w=112 -h=18
```

---

## 11. Keyboard reference

The complete proposed key map. Items marked ⚑ changed from the original brief,
with the reason given.

### Always available

| Key | Action |
|---|---|
| `Ctrl+Q` | Quit |
| `Ctrl+S` | Save |
| `Ctrl+Shift+S` | Save As |
| `Ctrl+N` | New workbook |
| `Ctrl+O` | Open |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo (per-edit, in memory) |
| `Alt+F` / `Alt+E` / `Alt+R` / `Alt+V` | File / Edit / Rollback / View menus |
| `F1` | Help and the function list |
| `F2` | Edit the active cell |
| `Esc` | Cancel / close the current menu or mode |

### Grid editing

| Key | Action |
|---|---|
| Arrows, `Home`, `End`, `PgUp`, `PgDn` | Move the cursor (§3.1) |
| `Shift` + arrows | Extend the selection (§3.2) |
| `Ctrl+C` | Copy the selection |
| `Ctrl+V` | Paste at the cursor |
| `Ctrl+X` | Cut |
| `Delete` | Clear the contents of the selection |
| `Ctrl+Shift++` | Insert a row or column |
| `Alt+D` | ⚑ Delete the current row |
| `Alt+C` | ⚑ Delete the current column |
| `+` / `-` (on a header) | Resize that row or column by one unit |

### ⚑ Why `Alt+D` and `Alt+C` instead of `Ctrl+D` / `Ctrl+Shift+D`

You asked whether Alt-only would be simpler. **Yes, and it is also the only
option that works identically everywhere.**

A terminal sends `Ctrl+D` and `Ctrl+Shift+D` as the *same byte*. There is no
information in the byte stream to tell them apart, so `Ctrl+Shift+D` can only
work on terminals that implement a newer keyboard-reporting protocol (kitty,
wezterm, foot) — and fails silently on GNOME Terminal, Konsole, xterm and
Alacritty.

`Alt` has none of that ambiguity, **and the design already depends on Alt** for
the menu bar (`Alt+F`, `Alt+R`). So using Alt for structural edits adds no new
dependency and removes a whole class of "the key does nothing on my machine"
bug reports.

`Alt+R` is taken by the Rollback menu, so the row/column pair is `Alt+D`
(**D**elete row — the common case) and `Alt+C` (**C**olumn).

If you would rather keep something close to the original, the fallback is:
`Ctrl+Shift+D` on terminals that support the extended protocol, `Alt+D`
everywhere else, with the status bar always showing which key is live. That is
strictly more code for strictly less predictability, which is why I recommend
against it.
