# 07 — Cross-sheet formulas: the plan for later

**Status: designed, deliberately not built.** v1 calculates within a single
sheet. This document is the complete, ready-to-execute plan for adding
cross-sheet references when you want them. It is written so that a future
session can pick it up without re-deriving anything.

---

## 1. The decision this implements

> "Let's keep calcs to single sheets for now, but add a markdown writeup for how
> we should add it later so when ready we can feed the next session the exact
> idea on how to do this."

So: **v1 is single-sheet.** This document is the "later".

---

## 2. What v1 must do today so that "later" is cheap

This is the part that matters most. Three small rules in v1 keep the door open.
They cost almost nothing now and save a file-format migration later.

| # | v1 rule | Why it matters |
|---|---|---|
| R1 | **The lexer must recognise `Sheet!A1` and `'Sheet name'!A1` as valid tokens**, even though the evaluator cannot resolve them. | Otherwise a user typing `=Sheet2!A1` gets a confusing parse error, and any tool that round-trips formulas might mangle the text. |
| R2 | **A formula is stored in the `.cell` file as its source text, verbatim.** The unresolved reference is preserved exactly. | When cross-sheet support arrives, opening an old file *just works*. No migration, no data loss. |
| R3 | **v1 evaluates an unresolved cross-sheet reference to `#REF!`** and the status bar explains why: `Cross-sheet references are not enabled yet`. | Honest failure. The user sees precisely what is wrong instead of a wrong number. |

R2 is already guaranteed by the file format ([`03-cell-file-format.md`](03-cell-file-format.md) §3.2:
formulas are stored as source and recomputed on load). R1 and R3 are small
lexer/parser tasks that belong in Phase 2.

**Do not skip R1.** It is the difference between "adding a feature" and
"migrating every file".

---

## 3. Reference syntax

### 3.1 What to support

| Form | Meaning |
|---|---|
| `Sheet2!A1` | Cell A1 on the sheet named `Sheet2`. |
| `Sheet2!A1:B5` | The rectangle A1:B5 on `Sheet2`. |
| `'Q3 Budget'!A1` | Sheet names containing spaces or punctuation are single-quoted. |
| `'It''s Q3'!A1` | A literal apostrophe inside a quoted name is doubled. |
| `Sheet1!A1` | Self-reference by name. Legal, and equivalent to `A1`. |

Case-insensitive, like everything else: `sheet2!a1` = `Sheet2!A1`.

### 3.2 What **not** to support (state this explicitly)

| Form | Why not |
|---|---|
| `Sheet1:Sheet3!A1` (3-D references) | Rarely used, complicates the dependency graph into a set of sheets, and interacts badly with sheet insertion. Reject with a clear message. |
| `[Book2.xlsx]Sheet1!A1` (external workbooks) | A different feature entirely. Out of scope permanently for v1.x. |
| `$A$1` absolute references | Not a cross-sheet question, but worth noting: absolute/relative addressing belongs with **copy-and-paste translation**, not here. Add them together or not at all. |

---

## 4. The one genuinely hard decision: name or identity?

If a formula stores the text `Sheet2!A1` and the user renames `Sheet2` to
`Q3 Data`, the formula must not silently break.

| Approach | Verdict |
|---|---|
| **A. Store the name in the formula; rewrite formulas on rename.** | ✅ **Recommended.** Formulas stay readable (`=Sheet2!A1`), the file stays legible, and a rename is one pass over the formula cells producing a single undoable operation. |
| B. Store a stable sheet ID (`=#7!A1`) | Formulas become unreadable and un-editable by hand. Rejected. |
| C. Store the name and also a resolved ID as a hint | Extra state to keep consistent for no real gain over A. Rejected. |

**Chosen: A.** Implementation detail that makes A safe:

- The parser produces `Ref{SheetName string, Row, Col int}` — the name, not an ID.
- The **resolver** converts `SheetName` → `*Sheet` at dependency-graph build time.
- On rename, the workbook walks every formula cell once, parses it, rewrites any
  `Ref` whose `SheetName` matches the old name, and re-serialises. One
  operation, one undo step, one journal entry (`Renamed sheet Sheet2 to Q3 Data`).
- Renaming is refused for a name that already exists, and names are matched
  case-insensitively.

---

## 5. Changes, file by file

This is the implementation checklist. Nothing here is speculative; each item
names the package that changes.

### 5.1 `internal/formula/lexer`

- Add a `SHEETNAME` token (a bare identifier followed by `!`) and a quoted-name
  variant.
- Emit `BANG` for `!`.
- Handle the doubled-apostrophe escape inside quoted names.
- **v1 already ships this** (rule R1 in §2), so this step is likely already done.

### 5.2 `internal/formula/ast`

```go
type Ref struct {
    Sheet string // "" means "this sheet"
    Row   int
    Col   int
}

type Range struct {
    From, To Ref
}
```

Parsing rules:

- `A1:B5` → `Range{From: Ref{Sheet: "", 0, 0}, To: Ref{Sheet: "", 4, 1}}`
- `Sheet2!A1:B5` → both endpoints take the sheet `Sheet2`.
- `Sheet2!A1:Sheet3!B5` → **error**. A range must lie within one sheet.
- A bare `Sheet2!A1` in a formula that expects a scalar is a scalar.

### 5.3 `internal/formula/parser`

Already handles ranges as function arguments. Add:

- A `Ref` may be prefixed by `Sheet!` or `'Sheet'!`.
- When a range endpoint omits the sheet, it inherits the range's other endpoint's
  sheet. `Sheet2!A1:B5` means `Sheet2!A1:Sheet2!B5`.
- Reject `Sheet2!A1:Sheet3!B5` with `#VALUE!` and a clear message.

### 5.4 `internal/grid` (the model)

```go
type Workbook struct {
    sheets []*Sheet
    index  map[string]*Sheet // lower-cased name → sheet
}
```

- Add `SheetByName(name string) (*Sheet, bool)`.
- Add `RenameSheet(i int, newName string) error`, which also performs the
  formula rewrite from §4.

### 5.5 `internal/formula/eval` (the dependency graph)

This is the real work.

**Today:** the graph is per-sheet. Node identity is `(row, col)`.
**After:** the graph is workbook-scoped. Node identity becomes `(sheetID, row, col)`.

```go
type NodeKey struct {
    Sheet uint32
    Row   uint32
    Col   uint32
}
```

Consequences, each of which needs a test:

| Concern | Change |
|---|---|
| Precedent extraction | `Ref{Sheet: ""}` resolves to the owning sheet; `Ref{Sheet: "X"}` resolves via `Workbook.SheetByName`. |
| Dirty propagation | Unchanged algorithm, but the dirty set can now contain nodes from several sheets. |
| Wave planning | Unchanged. Waves are already computed from the graph, so cross-sheet edges simply appear as extra levels. **No new scheduling code.** |
| Parallelism | Unchanged, and a bonus: cells on different sheets are trivially parallel. |
| Cycle detection | Must now span sheets. `A1 = Sheet2!A1` and `Sheet2!A1 = A1` is a cycle. The existing detector works unchanged **once node identity includes the sheet**. This is the single most important change. |
| Recalculation trigger | Editing a sheet invalidates dependents on *all* sheets, including sheets not currently visible. |
| Memory | One graph per workbook, not per sheet. |

**The pleasant part:** because the graph already keys on a node rather than a
cell object, and because wave planning is derived from the graph rather than
written by hand, this is a change of the key type plus resolution, not a rewrite
of the engine.

### 5.6 `internal/calc`

- The engine becomes workbook-scoped (`calc.New(wb *grid.Workbook)`).
- Dirty-set computation crosses sheets.
- The worker pool is unchanged.
- Add a **cross-sheet recalculation test**: edit `Sheet1!A1`, assert
  `Sheet2!C3` updates, and assert the result is identical whether evaluated
  sequentially or in parallel.

### 5.7 `internal/cell` / `internal/formula/functions`

- **No change.** Functions receive already-expanded values. A range from another
  sheet expands to the same `[]cell.Value` as a local one. This is the payoff of
  the argument-expansion contract in [`05-modularity.md`](05-modularity.md).

### 5.8 `internal/tui`

- The formula bar shows the source text unchanged: `fx: =SUM(Sheet2!A1:B5)`.
- **Pointing mode:** when editing a formula and the user switches sheet tabs
  mid-entry, insert `Sheet2!` in front of the reference automatically. This is
  what makes cross-sheet entry pleasant, and it is the reason to schedule this
  work *after* a basic formula-editing mode exists.
- Highlight the referenced range on the other sheet when the reference is
  selected in the formula bar (optional, nice to have).
- **Sheet rename must warn**: "3 formulas on Sheet1 refer to this sheet. Update
  them?" with Update / Cancel.
- **Sheet delete must warn** the same way, and offer to convert the references
  to `#REF!` instead of updating (deleting is destructive, so the default is to
  cancel).

### 5.9 `internal/sheetfile`

**No format change.** Formula source text is already stored verbatim
([`03-cell-file-format.md`](03-cell-file-format.md) §3.2), so a file written by a
cross-sheet-capable version is readable by a v1 binary — v1 will simply show
`#REF!` for those cells (rule R3).

If you want old binaries to refuse rather than partially evaluate, raise
`minReaderVersion`. **Recommendation: do not.** Partial evaluation with a clear
message is more useful than a refusal.

---

## 6. Error and edge-case behaviour (the test table)

| Situation | Result |
|---|---|
| `=Sheet2!A1` where A1 is empty | `0` in arithmetic, blank in display. |
| `=Sheet2!A1` where A1 is text | `#VALUE!` (same rule as local text). |
| `=SUM(Sheet2!A1:B5)` containing text | Text skipped, exactly like a local range. |
| `=Sheet2!A1` and Sheet2 does not exist | `#REF!`, and the status bar says `No sheet named "Sheet2"`. |
| Sheet2 existed and was renamed | Formula rewritten on rename; no error. |
| Sheet2 existed and was deleted | `#REF!` in every formula that referenced it. |
| `A1 = Sheet2!B1`, `Sheet2!B1 = A1` | Both `#CIRC!`. Cycle detection spans sheets. |
| `=Sheet2!A1:Sheet3!B5` | `#VALUE!` — a range cannot span sheets. |
| `=Sheet1:Sheet3!A1` | `#VALUE!` — 3-D references unsupported (message says so). |
| `=sheet2!a1` | Same as `=Sheet2!A1`. |
| `='Q3 Budget'!A1` | Works; sheet names with spaces are quoted. |
| `='It''s Q3'!A1` | Works; doubled apostrophe is an escape. |
| Sheet name is `Sheet2` and the user types `=Sheet2!A1` while **on** Sheet2 | Legal self-reference. |
| `=SUM(Sheet2!A1:B5, Sheet3!A1:B5, 10)` | Legal; mixed sheets in one call. |

---

## 7. Suggested order of work

Each step is independently testable and leaves the program working.

| Step | Work | Test that proves it |
|---|---|---|
| 1 | Lexer + parser accept `Sheet!Ref` and quoted names (R1). | Parser table test for every row of §3.1 and §3.2. |
| 2 | Evaluator returns `#REF!` for unresolved cross-sheet refs (R3). | A formula with `Sheet2!A1` errors cleanly; no crash; source preserved. |
| 3 | Node identity becomes `(sheetID, row, col)`; graph becomes workbook-scoped. | Existing single-sheet tests still pass; `-race` clean. |
| 4 | Resolution: `SheetByName` + `Ref` → node. | Cross-sheet reference reads a value. |
| 5 | Dirty propagation + waves across sheets. | Edit Sheet1!A1 → Sheet2!C3 updates; sequential == parallel. |
| 6 | Cross-sheet cycle detection. | Two-sheet cycle → both `#CIRC!`. |
| 7 | Sheet rename rewrites formulas, as one undoable operation. | Rename, assert every reference updated and one undo restores. |
| 8 | Sheet delete → `#REF!` with a warning dialog. | Delete, assert formulas error with the right message. |
| 9 | TUI pointing mode across tabs. | Feed keys to the model; assert `Sheet2!` is inserted. |
| 10 | Documentation and the function-list help. | Manual. |

Steps 1–2 belong in Phase 2 (they are the cheap insurance). Steps 3–10 are the
feature itself and can be scheduled whenever you want it.

---

## 8. Effort, honestly

| Scope | Estimate |
|---|---|
| Steps 1–2 (the insurance) | A few hours, inside Phase 2. |
| Steps 3–6 (the engine) | The bulk of the work. The graph change is mechanical but touches the most-tested part of the program. |
| Steps 7–9 (the UX) | Meaningful: rename/delete warnings and pointing mode are the fiddly parts. |
| Step 10 | Small. |

**Nothing here requires a file-format change, a data migration, or a rewrite of
the calculation engine.** That is the direct result of two v1 decisions: storing
formulas as source, and keying the dependency graph on nodes rather than objects.

---

## 9. Summary for a future session

> Cross-sheet references are additive. Lex and parse them in v1 and fail with
> `#REF!`; the file format already preserves the source text, so no migration is
> ever needed. To enable them: change the dependency graph's node key from
> `(row, col)` to `(sheetID, row, col)`, resolve `Ref.Sheet` through
> `Workbook.SheetByName`, and the existing wave planner, worker pool and cycle
> detector work unchanged. Reference sheets **by name** in formula text and
> rewrite those formulas on rename — do not introduce sheet IDs into the source.
> The error table in §6 is the test list.
