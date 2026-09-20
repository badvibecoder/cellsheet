# 05 — Modularity

Your requirement: *"adding features should be as easy as adding a file to a
folder."* This document explains how that works, what it costs, and the one place
where it cannot mean what it sounds like.

---

## 1. The honest limit, stated first

Go programs are **compiled**. A running binary cannot read a new `.go` file and
grow a new function — that is not a Go limitation we can engineer around, it is
what "compiled language" means.

So "add a file" means:

> Add one file to a folder, run `go build`, and the feature exists.
> **No other file in the repository needs to be edited.**

That is a real and valuable property, and it is what the design delivers. If you
want the program to grow features **without rebuilding** — for example, you drop
a script into a folder while the program is running and `=MEDIAN()` becomes
available — that is a different architecture (an embedded scripting language),
and it is a decision worth making deliberately. See §6.

---

## 2. The extension points

| # | To add… | Drop a file in… | And you get |
|---|---|---|---|
| 1 | A formula function | `internal/formula/functions/` | `=MYFUNC()` usable in any cell. |
| 2 | An arithmetic operator | `internal/formula/ops/` | A new infix or prefix operator. |
| 3 | A number format | `internal/cell/formats/` | A new display style (percent, accounting…). |
| 4 | A menu item | `internal/tui/menu/items/` | An entry under File/Edit/Rollback/View. |
| 5 | A key binding | `internal/tui/keys/bindings/` | A shortcut mapped to a command. |
| 6 | A command | `internal/app/commands/` | An action any binding or menu item can invoke. |
| 7 | A file codec | `internal/sheetfile/codec/` | CSV import/export, or a new format. |
| 8 | A whole screen | `internal/tui/screens/` | A new view (settings, formula wizard…). |

---

## 3. How self-registration works

Each extension point is a **package**, and every file in it runs an `init()`
function when the program starts. Those `init()` functions register themselves
with a registry:

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

Because `sum.go` and `median.go` are in the **same package**, adding a new file
to that folder requires zero changes anywhere else. Go compiles all files in a
directory together, so the new `init()` simply runs. There is no list to keep in
sync — which is exactly the failure mode this design is trying to avoid.

---

## 4. Walkthrough: adding `=MEDIAN()`

**Step 1.** Create `internal/formula/functions/median.go`:

```go
package functions

import (
    "sort"

    "cellsheet/internal/cell"
    "cellsheet/internal/formula/registry"
)

func init() {
    registry.Register(registry.Spec{
        Name:       "MEDIAN",
        Summary:    "Returns the middle value of the numbers supplied.",
        MinArgs:    1,
        MaxArgs:    -1,
        TakesRange: true,
        Fn: func(_ *registry.Ctx, args []cell.Arg) cell.Value {
            nums := make([]cell.Dec, 0, 16)
            for _, a := range args {
                for _, v := range a.Values() {
                    if v.Skip() {
                        continue // empty or text inside a range
                    }
                    nums = append(nums, v.Dec())
                }
            }
            if len(nums) == 0 {
                return cell.Error(cell.ErrNum)
            }
            sort.Slice(nums, func(i, j int) bool { return nums[i].Less(nums[j]) })
            mid := len(nums) / 2
            if len(nums)%2 == 1 {
                return cell.Number(nums[mid])
            }
            return cell.Number(nums[mid-1].Add(nums[mid]).Div(2))
        },
    })
}
```

**Step 2.** `go build`.

**Step 3.** `=MEDIAN(A1:A20)` works. Documentation, argument checking, error
propagation, panic isolation and parallel safety are all handled by the registry.

That is the whole process. No registry file to edit, no switch to extend, no
documentation list to update (the `Summary` string feeds `Help → Functions`).

---

## 5. Rules that keep modules from breaking the program

| Rule | Enforced by |
|---|---|
| A function must be **pure** — no globals, no I/O, no clock | Code review + a test that runs the whole registry concurrently under `-race`. |
| A function must never panic | The evaluator recovers per call and returns `#ERR!` in that one cell. |
| Argument count is the registry's job, not the function's | `MinArgs`/`MaxArgs`. |
| Bad argument kinds are caught before the function runs | The registry. |
| A function must not allocate per call where avoidable | Benchmark in the test suite; reuse the provided scratch buffers. |
| A module must not reach into the UI or the file format | The import rule (§7) — the compiler refuses to build it. |

The result: **one bad module degrades one cell**, not the program.

---

## 6. If you want modules without rebuilding

This is worth deciding now, because it changes the architecture.

| Approach | What you gain | What it costs |
|---|---|---|
| **Compile-time files** *(recommended)* | Simplest, fastest, safest, full access to all internal types, one language. | Requires `go build` after adding a file. |
| Embedded scripting (**Starlark** or **Lua**) | Add a script while the program runs; users can write their own functions without Go. | Adds a dependency and a second language; script functions are 10–100× slower; error messages are worse; a sandbox to maintain. |
| Go plugins (`.so`) | Runtime loading of compiled Go. | Linux/macOS only, requires exact compiler-version matching, effectively unshippable. **Not viable.** |

**My recommendation:** compile-time files for v1. If you later want
user-authored formulas, add an embedded script engine as a *second* extension
point — the registry design already accepts it, because a script function is
just another `Spec` whose implementation happens to be interpreted. Nothing in
the current design would need to be undone.

---

## 7. How the rule is kept honest

`01-architecture.md` states that packages may only import from lower layers.
That is easy to say and easy to break. So a test enforces it:

```go
// internal/arch/imports_test.go
func TestPackageLayers(t *testing.T) {
    // Parses every package's imports and checks them against the allowed
    // layer table. A violation fails the build with the offending import.
}
```

If someone (including me, six months from now) makes `internal/cell` import
`internal/tui`, `go test ./...` fails and says exactly which import broke which
rule. Architecture that is not enforced is architecture that decays.

---

## 8. Guidelines for a module author

Written for whoever adds the tenth function, possibly you:

1. **One file, one function.** `sum.go` holds `SUM`. It makes the folder a
   readable index of what the program can do.
2. **Use the shared helpers.** `cell.Value` arithmetic already handles currency
   promotion, sign and error propagation. Do not reimplement it.
3. **Ignore what should be ignored.** Inside a range, empty and text cells are
   skipped — that is `v.Skip()`.
4. **Return errors, do not panic.** `cell.Error(cell.ErrNum)` is always better
   than a crash, even though a crash is contained.
5. **Write the table test.** For `median.go`, a `median_test.go` beside it with
   the odd/even/empty/text cases. Tests live next to the module they test.
6. **No state.** If you need a cache, it belongs in the engine, not in the
   function.
