# 06 — Open questions and decisions

Everything I need from you, in one place. Each item has my recommendation and my
reasoning, so "yes to all the recommendations" is a valid answer.

**Status column:** 🔴 blocking Phase 2 · 🟡 needed during Phase 2 · 🟢 can wait.

---

## Decisions already made

Recorded here so the reasoning survives; the detailed items below are updated to
match.

| # | Decision | Date |
|---|---|---|
| D1 | **The look is approved**, with one addition: the active cell needs a visible highlight. Added and re-mocked — heavy `║` bars, reverse-video contents, and `[r1]` / `[C]` brackets on the headers. See [`04-tui-layout.md`](04-tui-layout.md) §3. | Phase 1 review |
| D2 | **The cursor is driven by the arrow keys**, with `Shift`+arrows extending a selection. Full key map in [`04-tui-layout.md`](04-tui-layout.md) §11. | Phase 1 review |
| D3 | **Undo/redo is high-resolution**: every single change in a session is tracked, so undo is per-edit and never coarser than one keystroke. Checkpoints remain the coarse, persisted safety net. | Phase 1 review |
| D4 | **Checkpoints happen on the 3-minute timer *and* on notable events** (bulk paste, structural change, rollback), which is what produces the labels in the mock. | Phase 1 review |
| D5 | **Linux is the only target for now.** The code stays portable; macOS and Windows are a later build-matrix decision, not a rewrite. | Phase 1 review |
| D6 | **Clipboard is internal only.** Copy and paste between cells preserve formulas and types; no system-clipboard integration. | Phase 1 review |
| D7 | **Structural edits use `Alt` keys.** `Alt+D` deletes the row, `Alt+C` deletes the column. Rationale in [`04-tui-layout.md`](04-tui-layout.md) §11. | Phase 1 review |
| D8 | ~~Still open: the numeric engine and the module mechanism.~~ Resolved below as D9 and D10. | Phase 1 review |
| D9 | **Numbers are exact fixed-point decimals**, not floating point. `$0.10 + $0.20` is exactly `$0.30`. | Phase 1 review |
| D10 | **Modules are compile-time files.** Add one file to a folder, rebuild, the feature exists. No runtime scripting. | Phase 1 review |
| D11 | **Text is left-aligned and numbers right-aligned** (the Excel convention), not the sketch's all-right alignment. The prototype default has been changed to match. | Phase 1 review |
| D12 | **v1 calculates within a single sheet.** Cross-sheet syntax is still lexed and preserved, and the full plan for enabling it later is written up in [`07-cross-sheet-formulas.md`](07-cross-sheet-formulas.md). | Phase 1 review |
| D13 | **Accepted:** `%` means percent with `MOD(a,b)` for remainder; negative money displays as `-$500.00`; `YES`/`NO` stay text in v1; rollback is reversible; a manual "Create checkpoint now" (Ctrl+B) exists. | Phase 1 review |
| D14 | **Not adopted:** system clipboard, mouse support, crash journal, optional config file. Defaults and consequences recorded in §G below. | Phase 1 review |
| D15 | **No CSV import or export in v1.** Tab-separated *paste* is in v1. | Phase 1 review |
| D16 | **No remote repository.** The Go module is plain `cellsheet`; everything stays local. | Phase 1 review |
| D17 | **`PHASE-1-SPEC.md` at the repo root is the single-file source of truth**, so the project can be recreated from one document. | Phase 1 review |

---

## A. The look

### Q1. ✅ Do you approve the interface?

**Approved** — see D1. The requested active-cell highlight has been added and is
visible in [`layout/preview-main.txt`](layout/preview-main.txt),
[`layout/preview-nav.txt`](layout/preview-nav.txt) (three frames, arrow-key
movement) and [`layout/preview-select.txt`](layout/preview-select.txt).

Section 8 of [`04-tui-layout.md`](04-tui-layout.md) lists the seven places where
the prototype deliberately deviates from your sketch, with reasons.

> **Still worth a look:** the three-frame navigation preview, to confirm the
> highlight reads the way you meant.

### Q2. ✅ Text alignment: Excel convention

**Decision (D11):** text left, numbers right. The prototype now defaults to this;
`-text-right` reproduces the original sketch.

Your mock right-aligns everything, including text (`Operations`, `Audited?`).
Excel left-aligns text and right-aligns numbers.

The sketch right-aligned everything, including text (`Operations`, `Audited?`).
Excel left-aligns text and right-aligns numbers. Both are one line of code:
`go run ./prototype/look -text-right` reproduces the sketch.

### Q3. 🟡 Should the active cell keep its width?

Your mock shows the selected cell's interior two characters narrower than its
neighbours. The prototype keeps the width identical and swaps the borders to `║`
plus reverse video.

> **Recommendation:** keep the width. If the interior changes width, every cell
> in the column appears to shift when you move the cursor.

### Q4. 🟢 Colour theme

The prototype uses a dark-terminal palette: grey frame, green currency, white
numbers, cyan highlights, red errors.

> **Recommendation:** ship this as the default and add a light-terminal theme
> plus a config file in Phase 5.

---

## B. Calculation

### Q5. ✅ Exact decimals

**Decision (D9):** numbers are stored as exact fixed-point decimals. Money totals
are trustworthy by construction, and the "signed float" promotion rule in
[`02-calculation-rules.md`](02-calculation-rules.md) §4.3 is satisfied without
any lossy conversion step.

| Option | Consequence |
|---|---|
| **Exact decimal** *(recommended)* | `$0.10 + $0.20` is exactly `$0.30`, always. Every money total is trustworthy. Costs ~250 lines we own. |
| `float64` (Excel's approach) | Matches Excel's quirks exactly, including its rounding artifacts. Less code. |

> **Recommendation:** exact decimal. For a program whose main job is budgets,
> floating-point money errors are a defect, not a compatibility feature.

### Q6. 🟡 What should `%` mean?

You listed `%` among the maths operators. In Excel, `%` is a **postfix percent**
(`50%` = `0.5`) and modulo is the `MOD()` function.

> **Recommendation:** Excel's meaning — `50%` → `0.5`, and `MOD(a,b)` for
> remainder. If you meant modulo as an infix operator, say so and I will support
> both.

### Q7. 🟡 How should a negative amount of money look?

`-$500.00` or `($500.00)` (accounting style)?

> **Recommendation:** `-$500.00`. Universally legible; accounting parentheses
> can be added later as a display format.

### Q8. 🟡 Should `YES`/`NO` be real booleans?

Your mock shows `YES` in a cell. In v1 it is text, so `=IF(...)` has nothing to
work with.

> **Recommendation:** keep them as text in v1, and revisit booleans when `IF()`
> is added. `IF()` can accept text for now.

### Q9. 🟢 Number formatting scope

Plain numbers currently get no thousands separators (`5737.50`), while currency
always does (`$50,000.00`) — matching your mock.

> **Recommendation:** keep as-is; add a per-cell format override in a later
> phase.

### Q10. 🟢 `50%`, `(500)`, dates typed into a cell

Currently all treated as text. See [`02-calculation-rules.md`](02-calculation-rules.md) §3.

> **Recommendation:** leave them as text in v1. Dates in particular are a large
> feature (formats, arithmetic, locale) that deserves its own phase.

---

## C. Keyboard and input

### Q11. ✅ `Ctrl+Shift+D` cannot be delivered reliably — resolved using Alt

**Decision (D7):** `Alt+D` deletes the row, `Alt+C` deletes the column. Alt has
no ambiguity, and the menu bar already depends on it. Full reasoning in
[`04-tui-layout.md`](04-tui-layout.md) §11.

The original analysis, kept for the record:

Most terminals transmit **the same byte** for `Ctrl+D` and `Ctrl+Shift+D`, so the
program cannot tell them apart. This is a terminal limitation, not something I
can code around on every system.

Options:

| Option | Effect |
|---|---|
| **A. `Ctrl+D` = delete row; `Alt+D` = delete column** *(recommended)* | Works everywhere, immediately. |
| B. `Ctrl+D` = row; `Ctrl+Shift+D` where the terminal supports the kitty keyboard protocol, `Alt+D` elsewhere | Honours your spec on modern terminals, degrades gracefully. |
| C. `Ctrl+D` = row; `Ctrl+K` = column | Same as A with a different second key. |

> **Recommendation:** **B** — try your exact shortcut, fall back automatically,
> and show the working shortcut in the status bar so you are never guessing.

### Q12. 🟡 `Ctrl+C` for copy, not interrupt

In a full-screen terminal program, we receive `Ctrl+C` as a key, so copy works.
The side effect is that `Ctrl+C` no longer interrupts the program — `Ctrl+Q`
quits, as you specified.

> **Recommendation:** accept this; it is how every terminal spreadsheet behaves.

### Q13. ✅ Clipboard: internal only

**Decision (D6):** internal clipboard only. Copy and paste between cells always
work, formulas and types are preserved, and no external program is required.

The alternatives considered were:

| Option | Effect |
|---|---|
| **Internal + OSC 52 export** *(recommended)* | Copy/paste between cells always works, over SSH included. Text can also be pushed to the system clipboard using the OSC 52 terminal escape — no external program needed. |
| Shell out to `xclip`/`wl-copy` | Native integration, but requires those programs to be installed, breaking "the binary is all you need". |

Note there are two different pastes and both will work: `Ctrl+V` pastes the
app's internal clipboard (formulas and types preserved), while pasting
tab-separated text from another application uses the terminal's own paste.

> **Recommendation:** internal clipboard as the primary, OSC 52 as a bonus.

### Q14. 🟡 Mouse support?

> **Recommendation:** yes, for click-to-select, drag-to-select a range, wheel
> scroll, and clicking menu items and tabs. No hover — most terminals cannot
> report it, and your `+`/`-` resize gesture works by placing the cursor on the
> header with the keyboard or a click.

---

## D. History and checkpoints

### Q15. ✅ Checkpoints on 3-minute boundaries **and** on notable events

**Decision (D4).** The timer always runs; bulk pastes, structural changes and
rollbacks also create an immediately-labelled checkpoint.

Your mock shows checkpoints labelled `Row 4 edited` at 10:02:15 and
`Bulk Paste (+12 cells)` at 09:59:00 — times that are not 3-minute boundaries.
So it looks like you want **both**.

> **Recommendation:** both. The 3-minute timer always runs; bulk pastes,
> structural changes and rollbacks also create an immediately-labelled
> checkpoint. It costs nothing and the history becomes far more useful.

### Q16. ✅ Add high-resolution undo/redo (`Ctrl+Z` / `Ctrl+Y`)

**Decision (D3).** Every individual change in a session is recorded, so undo is
per-edit and never coarser than a single keystroke. Checkpoints remain as the
coarse, persisted, cross-session safety net — the two mechanisms do different
jobs and neither replaces the other.

Checkpoints are coarse — up to 3 minutes apart. Undo is per-keystroke and lives
in memory; the two are complementary, not redundant.

> **Recommendation:** yes. A spreadsheet without `Ctrl+Z` will feel broken
> within a minute of use, regardless of how good the rollback system is.

### Q17. 🟡 Is a rollback reversible?

If rolling back to index 5 discarded everything after it, a mis-click would
destroy work.

> **Recommendation:** preserve the pre-rollback state as the new index 1,
> labelled `Rolled back from index 5`. A rollback can then itself be rolled back.
> The cost is one history slot. See [`03-cell-file-format.md`](03-cell-file-format.md) §6.

### Q18. 🟢 Should there be a manual "Create checkpoint now"?

> **Recommendation:** yes — a File-menu item and `Ctrl+B`, so you can mark a
> point before doing something risky.

### Q19. 🟢 Crash journal between checkpoints?

If the program is killed between checkpoints, unsaved edits are lost.

> **Recommendation:** yes — a tiny append-only journal beside the file, deleted
> on clean exit. Cheap insurance; offered for recovery on next launch.

---

## E. Scope and packaging

### Q20. ✅ Linux only for now

**Decision (D5).** Linux amd64/arm64 is the only target for the first release.
The code stays portable — no cgo, no OS-specific assumptions outside
`internal/platform` — so adding macOS and Windows later is a build-matrix
decision rather than a rewrite.

| Option | Effect |
|---|---|
| **Linux + macOS + Windows** *(recommended)* | Same code, no extra work beyond the build matrix; costs us a test matrix. |
| Linux only | Slightly less testing, but nothing is actually saved — the code is portable either way. |

> **Recommendation:** build for all three; make Linux the primary target you
> actually use, and document Windows Terminal as the supported Windows terminal.

### Q21. ✅ Multiple sheets, single-sheet calculations

**Decision (D12).** Sheet tabs with add/rename/switch are in v1. Calculations
stay within one sheet. The parser still accepts `Sheet2!A1` and preserves the
text, and v1 reports `#REF!` with a clear message rather than a parse error — so
no file ever needs migrating when the feature lands.

The complete implementation plan, including the dependency-graph change and the
edge-case test table, is [`07-cross-sheet-formulas.md`](07-cross-sheet-formulas.md).
Hand that document to the session that builds it.

### Q22. 🟡 Where should config live?

> **Recommendation:** `~/.config/cellsheet/config.json` on Linux, the platform
> equivalent elsewhere, plus `./cellsheet.json` in the current directory if
> present. Entirely optional — the program runs with no config at all.

### Q23. 🟢 Repository and module path

Currently `cellsheet` (local module, no remote), which must match wherever you push
it for `go install` to work.

> **Recommendation:** confirm or correct this now; it is a one-line change today
> and a mildly annoying one later.

### Q24. 🟢 CSV import/export in v1?

> **Recommendation:** defer to v1.1. Tab-separated **paste** is in v1 (it is how
> data gets in from other applications), but file import/export is a separate
> feature with its own edge cases.

### Q25. 🟡 Size limits

Rows: 1,048,576; columns: 16,384; row height 1–20 lines; column width 4–80
characters; 99 checkpoints.

> **Recommendation:** accept these. They match Excel for rows/columns, so any
> future export is safe, and the resize bounds prevent a key held down from
> destroying the layout.

---

### Q26. ✅ The module mechanism: compile-time files

**Decision (D10).** Add `median.go` to `internal/formula/functions/`, rebuild,
and `=MEDIAN()` exists. No other file changes. Runtime scripting is not adopted;
[`05-modularity.md`](05-modularity.md) §6 records what it would have cost if you
change your mind — the registry design accepts a script-backed `Spec` without
rework.

---

## F. Things I decided without asking

Listed so nothing is hidden. Tell me if you disagree with any of them.

| Decision | Reasoning |
|---|---|
| Column names use Excel's bijective base-26 (`Z` → `AA`, `ZZ` → `AAA`) | That is what you described, and the boundaries are where naive implementations break. |
| The `.cell` container is custom, not ZIP or SQLite | Standard library only; keeps the single-binary promise; every byte explainable. |
| Formulas are stored as source, recomputed on load | A file can never contain a stale answer. |
| The status bar shows the active cell's *kind* | It is how you verify a type promotion at a glance. |
| Errors are shown as `#VALUE!`, `#DIV/0!`, `#CIRC!`, … | Excel vocabulary, so it needs no explanation. |
| The grid's last visible row absorbs a spare line | Keeps the frame closed with no doubled rules. |
| Architecture import rules are enforced by a test | Unenforced architecture decays. |
| Values get one trailing space in their cell | Matches your mock and makes the grid readable. |
| Saving is atomic and keeps one `.bak` | Data loss is the only truly unforgivable bug. |


---

## G. Items you did not adopt (D14)

Recorded with the default I will build, so nothing is quietly assumed. Say the
word and any of these can be added.

| Item | Default I will build | Consequence |
|---|---|---|
| System clipboard | **Internal clipboard only.** Copy/paste between cells preserves formulas and types. | Copying from cells into another application uses your terminal's own selection, as it does today. |
| Mouse | **Keyboard only.** No mouse capture at all. | Fewer terminal-compatibility surprises. Every action already has a key, so nothing becomes unreachable. |
| Crash journal | **None.** Unsaved edits made between checkpoints are lost if the process is killed. | Mitigated by `Ctrl+B` (manual checkpoint, which you did adopt) — press it before anything risky. |
| Config file | **None.** Settings come from command-line flags and the menus. | Stronger "the binary is all you need": the program writes nothing outside your `.cell` files except the `.bak` and the lock file. |
| CSV import/export | **Decided (D15): not in v1.** Tab-separated *paste* is in v1, which is how data gets in from other applications. | Confirmed: no file import or export at all for now. |
| Size limits | **Excel's limits:** 1,048,576 rows, 16,384 columns. Row height 1–20 lines, column width 4–80 characters, 99 checkpoints. | These are named constants, changeable in one place. The resize bounds stop a held-down key from destroying the layout. |
| Repository path | **`cellsheet` (local module, no remote)**, already in `go.mod`. | **Please confirm or correct.** It only matters for `go install` and for publishing; changing it later is one command (`go mod edit -module`). |

---

## H. Phase 1 sign-off

| | |
|---|---|
| Interface approved, including the active-cell highlight | ✅ |
| Design documents reviewed | ✅ |
| Single-file specification written | ✅ [`PHASE-1-SPEC.md`](../PHASE-1-SPEC.md) |
| **Phase 1 complete** | ✅ |
| **Phase 2 (prototype) authorised** | ✅ |

Nothing is open. Every item is either decided (D1–D17) or has a stated default in
§G. Work now proceeds to Phase 2; see [`PHASE-1-SPEC.md`](../PHASE-1-SPEC.md) §20
for the build order.
