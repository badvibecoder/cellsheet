# 03 — The `.cell` file format

A `.cell` file is standalone: it holds the current workbook **and** up to 99
rollback points. No sidecar files, no database, no server.

---

## 1. Requirements this format must satisfy

| Requirement | How |
|---|---|
| Standalone single file | Everything is in one container. |
| Contains the current state | A `STATE` section holds the live workbook. |
| Contains up to 99 rollbacks | A `HISTORY` section holds 99 reverse deltas. |
| Rolling back is fast | Reverse deltas; no full copies of the workbook. |
| A crash never corrupts it | Write to a temporary file, then atomically rename. |
| Corruption is detected, not guessed at | SHA-256 per section, plus a whole-file hash. |
| Tomorrow's version can open today's file | Explicit version numbers and skippable sections. |
| Files stay small | Everything is compressed; deltas are tiny. |

---

## 2. Container layout

```
 byte 0
 ┌─────────────────────────────────────────────────────────┐
 │ HEADER — fixed 64 bytes                                 │
 │   magic "CELLSHEET", format version, minimum reader     │
 │   version, flags, created/modified timestamps,          │
 │   section count, directory offset, header checksum      │
 ├─────────────────────────────────────────────────────────┤
 │ SECTION DIRECTORY — 48 bytes per section                │
 │   section id, offset, compressed length, uncompressed   │
 │   length, SHA-256 (truncated to 24 bytes), flags        │
 ├─────────────────────────────────────────────────────────┤
 │ SECTION PAYLOADS                                         │
 │   ┌──────────┐ ┌──────────┐ ┌──────────┐                │
 │   │ MANIFEST │ │  STATE   │ │ HISTORY  │                │
 │   └──────────┘ └──────────┘ └──────────┘                │
 ├─────────────────────────────────────────────────────────┤
 │ TRAILER — SHA-256 of everything above                   │
 └─────────────────────────────────────────────────────────┘
```

All integers are little-endian. Unknown section ids are preserved verbatim on
rewrite, so a newer version of cellsheet never loses data written by an older
one.

### Why not ZIP, JSON, or SQLite?

| Alternative | Verdict |
|---|---|
| ZIP (like `.xlsx`) | Would work, but gives us no place to put checksums, and the ordering guarantees we want are simpler to own directly. |
| Plain JSON | Human-readable, but 5–10× larger and slow to parse for 100,000 cells. We use JSON *inside* the MANIFEST section, where readability is worth more than bytes. |
| SQLite | Excellent for deltas, but it is a large dependency and it breaks the "one small static binary" promise. |
| **Custom container (chosen)** | ~400 lines, uses only the standard library, and every byte is explainable. |

---

## 3. The sections

### 3.1 `MANIFEST` — JSON, compressed

The only human-readable part. Metadata only; no cell data.

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

`scratch` is a place for future versions to record things old versions should
carry but not interpret.

### 3.2 `STATE` — binary, compressed

The live workbook. Encoded as varint-tagged records, column-major per sheet:

```
SHEET_BEGIN(sheetID, nameRef, rows, cols, defaultH, defaultW)
  ROW_META(row, height)              // only for rows that differ from default
  COL_META(col, width)               // only for columns that differ from default
  CELL(row, col, kind, payload)      // only for cells that are not empty
    kind 0 = Empty      (deletes; only appears in deltas)
    kind 1 = Number     payload = sign, mantissa varint, scale
    kind 2 = Currency   payload = sign, mantissa varint, scale
    kind 3 = Text       payload = UTF-8 bytes
    kind 4 = Formula    payload = UTF-8 source; the value is recomputed on load
    kind 5 = Error      payload = error code
SHEET_END
```

Formulas are stored as **source text**, never as cached results. On load, the
engine recalculates. This means a file can never contain a stale answer, and it
means the formula language can be improved without invalidating old files.

> **Cost, stated honestly:** opening a workbook with 50,000 formulas
> recalculates 50,000 formulas. At the v1 target that is well under a second.
> If it ever becomes a problem, we add an optional cached-value section that is
> always re-verified against the source — never trusted blindly.

### 3.3 `HISTORY` — binary, compressed

Up to 99 entries, newest first. Index 0 is the current state and is not stored
as a delta; entries 1–99 are the reverse deltas.

```
HISTORY(count, headRevisionID)
  ENTRY(
    index,          // 1..99
    revisionID,     // monotonically increasing, never reused
    timestamp,      // unix milliseconds + UTC offset minutes
    kind,           // Auto | BulkPaste | RowEdited | ColEdited | Structural | Rollback | Initial
    label,          // "Auto-Checkpoint", "Bulk Paste (+12 cells)", "Row 4 edited"
    summary,        // counts: cells added/removed/changed, rows/cols added/removed
    payload         // the reverse delta itself
  ) × count-1
```

The reverse delta is a list of the same record types as `STATE`, but expressed
as *undo* operations:

```
UNDO_CELL(sheet, row, col, previousKind, previousPayload)  // or Empty to clear
UNDO_ROW_META(sheet, row, previousHeight)
UNDO_COL_META(sheet, col, previousWidth)
UNDO_ROWS(sheet, at, count, removedCells…)                 // undo of an insert
UNDO_COLS(sheet, at, count, removedCells…)
UNDO_SHEET(id, definition)                                 // undo of a sheet add
```

Applying entry *k* transforms the workbook from state *k−1* into state *k*.
Rolling back to index *k* therefore means applying entries 1 through *k* in
order. Each step is tiny: editing one cell produces a delta of a few dozen
bytes.

---

## 4. The labels you see in the Rollback menu

Every checkpoint carries a label, which is why the menu reads the way it does in
your mock:

| Kind | Label shown | Created when |
|---|---|---|
| `Initial` | `Initial file load` | A workbook is opened. |
| `Sheet initialized` | `Sheet initialized` | A new workbook, or a sheet is added. |
| `Auto` | `Auto-Checkpoint` | The 3-minute timer fires **and** something changed. |
| `RowEdited` | `Row 4 edited` | A row-level edit completes. |
| `CellEdited` | `Cell C1 edited` | A single-cell edit worth marking. |
| `BulkPaste` | `Bulk Paste (+12 cells)` | A paste of more than one cell. |
| `Structural` | `Row 4 deleted`, `Column C inserted` | Structural change. |
| `Rollback` | `Rolled back to index 5` | A rollback occurred. |

Labels are generated from the operation log — the program knows *why* something
changed because every edit arrives as a named operation, not as a raw write.

---

## 5. Checkpoint policy

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

Rules:

1. The timer runs **every 3 minutes**, configurable in the config file.
2. It fires **only if the workbook changed** since the last checkpoint. Change is
   detected with a running hash maintained as edits are applied — not by
   re-serialising the workbook, which would be wasteful.
3. Marked events (bulk paste, structural change, rollback) create a checkpoint
   immediately, because they are the points you would actually want to return
   to. **This is an inference from your mock**, where checkpoints appear at
   10:02:15 and 09:59:00 rather than on 3-minute boundaries — please confirm in
   [`06-open-questions.md`](06-open-questions.md).
4. The ring holds **99** entries. Adding number 100 discards the oldest, which
   is why the "max available" number grows as you use the program and then
   settles.
5. Checkpoints are written to the file on save. Until you save, they exist only
   in memory and are labelled `(Uncommitted)` — exactly as index 0 shows.

---

## 6. Rollback semantics

- **Index 0** is always the current state. It is never rolled *to*; it is where
  you are.
- **Index *k*** is *k* steps in the past. "Immediate revert" is index 1.
- Rolling back to *k* applies reverse deltas 1…*k*.
- **The current state is always preserved first.** Rolling back creates a new
  checkpoint numbered 1 labelled `Rolled back from index k`, so a rollback can
  itself be undone by rolling back to 1. Losing work to a mis-click is not
  acceptable; this costs one extra history slot and buys complete safety.
- The custom-jump prompt accepts 1 through the maximum currently available and
  rejects anything else *before* changing state.
- Rolling back while unsaved changes exist prompts first, because the
  pre-rollback state only exists in memory at that moment.

Worked example — you are at index 0 and roll back to 5:

```
Before                          After
 0: Current State                0: Rolled-back state (was 5)
 1: Auto-Checkpoint              1: Rolled back from index 5   ← the old state 0
 2: Auto-Checkpoint              2: old 6
 3: Auto-Checkpoint              3: old 7
 4: Row 4 edited                 4: old 8
 5: Bulk Paste (+12 cells)       5: old 9
 6: Auto-Checkpoint              …
```

---

## 7. Integrity and crash safety

| Protection | Mechanism |
|---|---|
| Detect a damaged file | SHA-256 per section + whole-file trailer hash. |
| Detect a truncated file | Header records the exact payload length; a short read is an error, not a partial load. |
| Never corrupt on save | Write `name.cell.tmp` → `fsync` → `rename()` over `name.cell`. Atomic on every filesystem we target. |
| Recover from a bad save | The previous version is kept as `name.cell.bak`. |
| Know which section failed | The error names it: `HISTORY section failed its checksum (expected a1b2…, got 9f8e…)`. |
| Survive a partial rollback | Applying a rollback is transactional: a failed delta leaves the workbook untouched. |
| Stop two writers | `name.cell.lock` with the owning PID. A second instance opens read-only and says so. A lock whose PID is gone is treated as stale and reclaimed. |

If `MANIFEST` or `STATE` fails its checksum, the file is not loaded; we offer the
`.bak`. If only `HISTORY` fails, the workbook opens with a warning and without
rollback points — a damaged history must never cost you your data.

---

## 8. Versioning

Two numbers are stored, and they mean different things:

- **`formatVersion`** — bumped when the byte layout changes.
- **`minReaderVersion`** — the oldest reader that can still make sense of this
  file. A reader older than this refuses to open it and says so plainly.

A reader that is *newer* than `formatVersion` reads the sections it understands
and preserves the rest untouched.

---

## 9. How big is a `.cell` file?

Estimates for a realistic workbook — a 100 × 26 budget sheet with a few hundred
populated cells:

| Part | Uncompressed | Compressed |
|---|---|---|
| HEADER + directory | ~200 bytes | — |
| MANIFEST | ~600 bytes | ~400 bytes |
| STATE | ~12 KB | ~3 KB |
| HISTORY (99 small deltas) | ~40 KB | ~8 KB |
| **Total** | | **≈ 12 KB** |

A workbook with 50,000 populated cells lands around 1–2 MB. A 99-checkpoint
history of a single large paste stays small because the deltas describe changes,
not whole copies.

---

## 10. File naming and defaults

- Extension: **`.cell`** (case-insensitive on read).
- New workbooks default to `untitled.cell` until saved.
- The window title shows `cellsheet: Q3_budget.cell [●]`, where `●` means
  unsaved changes and `○` means everything is saved.
- Saving over an existing file keeps one previous copy as `.cell.bak`.
