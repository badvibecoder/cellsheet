# 02 — Calculation rules

What a cell contains, how types combine, and in what order maths happens.

This document is the specification. Every rule here will have a matching
automated test in Phase 3.

---

## 1. What a cell can hold

A cell holds one of five **kinds**:

| Kind | Meaning | Example display |
|---|---|---|
| **Empty** | Nothing has been entered. | *(blank)* |
| **Number** | A quantity with no currency. | `450`, `12.75`, `-3` |
| **Currency** | A quantity of US dollars. | `$50,000.00` |
| **Text** | Anything else. Never used in maths. | `Operations`, `YES` |
| **Error** | A calculation that could not be performed. | `#DIV/0!` |

Two further properties are tracked for numbers and never shown as a separate
kind, but they decide how results are stored and displayed:

- **Sign** — whether the value is negative.
- **Fractional** — whether the value has a non-zero part after the decimal point.

**This is the key design decision, and it is what makes your
"signed float" requirement automatic:** numbers are never stored in a narrow
type that could lose a sign or a fraction. Every number in the program is stored
as an exact decimal that can always represent a sign and up to 18 decimal
places. Promotion rules decide the *kind* of a result (Number vs Currency) and
whether it displays as a whole number or with decimals — but there is never a
lossy conversion that could drop a minus sign or truncate a fraction.

---

## 2. How numbers are stored: exact decimal, not floating point

Standard floating point (`float64`) cannot represent `0.10` exactly. Add
`$0.10` to `$0.20` in floating point and you get `0.30000000000000004`. Excel
has this behaviour and papers over it by showing only 15 significant digits,
which is why spreadsheets famously disagree with calculators about money.

**We use an exact fixed-point decimal instead:**

```go
// internal/cell/decimal.go  (illustrative)
type Dec struct {
    Mant  int64  // the digits, without a decimal point
    Scale int8   // how many of those digits are after the point
}
// 50000.00  →  Mant: 5000000, Scale: 2
// 12.75     →  Mant: 1275,    Scale: 2
// 3         →  Mant: 3,       Scale: 0
```

| Operation | Rule | Example |
|---|---|---|
| Add / subtract | Line up the scales, add the digits, keep the larger scale | `1.5 + 1.25` → `2.75` |
| Multiply | Multiply digits, **add** scales | `1.5 × 1.25` → `1.875` |
| Divide | Compute to 12 decimal places, then round half away from zero | `1 ÷ 3` → `0.333333333333` |
| Percent | Divide by 100 | `50%` → `0.5` |
| Power | Integer exponents repeat-multiply; others go via division rule | `2^10` → `1024` |

**Range:** about ±9,200,000,000,000,000,000 (±9.2 × 10¹⁸). Exceeding it produces
`#NUM!` rather than a silently wrong answer.

**Why not a decimal library?** Adding one would add a dependency, and money
arithmetic needs only four operations and one rounding rule. Ours is ~250 lines
and fully testable — a good trade for a program that must stand alone.

---

## 3. Deciding a cell's kind when you type into it

When you type text into a cell and press Enter, these rules are applied **in
this exact order**. The first one that matches wins.

| # | Test | Result |
|---|---|---|
| 1 | Starts with `=` | **Formula** — parsed and evaluated; the kind comes from the result (§5). |
| 2 | Text is empty or only spaces | **Empty**. |
| 3 | Matches the currency pattern | **Currency**. |
| 4 | Matches the number pattern | **Number** (integer form if no decimal point, fractional otherwise). |
| 5 | Anything else | **Text**. |

### Currency pattern

```
[+|-] $ [digits with optional thousands commas] [ . digits ]
```

| Input | Parsed as | Displayed as |
|---|---|---|
| `$50000` | Currency 50000 | `$50,000.00` |
| `$50,000` | Currency 50000 | `$50,000.00` |
| `$1,250.00` | Currency 1250 | `$1,250.00` |
| `-$500` or `$-500` | Currency −500 | `-$500.00` |
| `$0.5` | Currency 0.5 | `$0.50` |

Currency is **always displayed with exactly two decimal places and thousands
separators**, US format, as you specified. Display only — the stored value keeps
full precision.

### Number pattern

```
[+|-] [digits] [ . digits ] [ (e|E) [+|-] digits ]     at least one digit
```

| Input | Parsed as | Displayed as |
|---|---|---|
| `450` | Number 450 | `450` |
| `-3` | Number −3 | `-3` |
| `12.75` | Number 12.75 | `12.75` |
| `50,000` | Number 50000 | `50000` |
| `1e3` | Number 1000 | `1000` |
| `.5` | Number 0.5 | `0.5` |
| `10.00` | Number 10 | `10` (trailing zeros are not significant) |

Note that plain numbers — unlike currency — are **not** given thousands
separators. That matches your mock, where `$50,000.00` has separators but
`5737.50` does not.

### Deliberately *not* recognised in v1

| Input | Treated as | Why |
|---|---|---|
| `TRUE` / `FALSE` | Text | You have not asked for booleans. Your mock shows `YES` as a plain string. |
| `50%` in a **cell** | Text | Percentages need their own display format. `%` does work inside formulas. |
| `(500)` as negative | Text | Ambiguous; avoids silently turning a notation into a negative number. |
| Dates (`3/4/2026`) | Text | Dates are a large feature and not in scope. |

Each of these is flagged in [`06-open-questions.md`](06-open-questions.md) if you
want any of them promoted later.

---

## 4. Combining types in arithmetic

Two questions must be answered every time: **what kind is the answer**, and
**can the operation happen at all**.

### 4.1 First, can it happen?

| Operand | Behaviour |
|---|---|
| **Error** | The error propagates. `=1 + #DIV/0!` is `#DIV/0!`. |
| **Text** | `#VALUE!` — maths is never attempted on text. |
| **Empty** | Treated as `0` in arithmetic. `A1 + 5` where A1 is blank gives `5`. |

These checks happen before any arithmetic, so a text cell can never produce a
nonsense number.

### 4.2 Then, what kind is the answer?

Read the operator, then the operands:

| Operation | Left | Right | Result kind | Example |
|---|---|---|---|---|
| `+` `-` | Number | Number | **Number** (fractional if either is) | `450 + 12.75` → `462.75` |
| `+` `-` | Currency | Currency | **Currency** | `$50,000.00 + $1,250.00` → `$51,250.00` |
| `+` `-` | Currency | Number | **Currency** | `$500.00 - 20` → `$480.00` |
| `+` `-` | Number | Currency | **Currency** | `20 + $500.00` → `$520.00` |
| `×` | Number | Number | **Number** | `450 × 12.75` → `5737.50` |
| `×` | Currency | Number | **Currency** | `$500.00 × 3` → `$1,500.00` |
| `×` | Number | Currency | **Currency** | `3 × $500.00` → `$1,500.00` |
| `×` | Currency | Currency | **Number** | dollars × dollars is not dollars — it is a squared quantity, so it becomes a plain number |
| `÷` | Number | Number | **Number** (always fractional) | `10 ÷ 4` → `2.5` |
| `÷` | Currency | Number | **Currency** | `$100.00 ÷ 4` → `$25.00` |
| `÷` | Currency | Currency | **Number** (a ratio) | `$200.00 ÷ $50.00` → `4` |
| `÷` | Number | Currency | **Number** (1/dollars) | `100 ÷ $50.00` → `2` |
| `÷` by zero | any | zero | **`#DIV/0!`** | |
| `%` (postfix) | Number | — | **Number** | `50%` → `0.5` |
| `%` (postfix) | Currency | — | **Currency** | `$50.00%` → `$0.50` |
| `^` | Number | Number | **Number** | `2^10` → `1024` |
| `^` | Currency | Number | **Currency** | `$100.00^2` → `$10,000.00` |
| `MOD(a,b)` | Currency | Currency | **Currency** | `MOD($100.00, $30.00)` → `$10.00` |
| comparisons | any | any | **Number** (1 = true, 0 = false) | `$5.00 > $3.00` → `1` |

**The rule in one sentence:** currency survives addition, subtraction and
scaling by a plain number; it does not survive multiplication by currency or
division by currency, because the units no longer make sense.

### 4.3 Sign and fraction propagation

- A negative value always keeps its sign through any operation that can produce
  one. Nothing is silently abs()-ed.
- If **either** operand has a fractional part, the result is stored with full
  fractional precision. `450 + 12.75` keeps `.75`.
- If the result is a whole number, it displays without a decimal point
  (`4` from `$200.00 ÷ $50.00`), but its full precision is retained internally.
- **There is no conversion step that can lose a sign or a fraction.** This is
  the concrete answer to your requirement that a calculation involving a sign or
  a float must yield a signed float.

The **status bar** shows the kind of the active cell (`Active: C1 (Currency)`),
which is how you verify a promotion at a glance.

---

## 5. Writing a formula

### 5.1 Grammar

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
```

### 5.2 Order of operations

Highest first. This is the table to test against.

| Level | Operator | Associativity | Notes |
|---|---|---|---|
| 1 | `( … )` | — | Innermost first. |
| 2 | `x%` | left | Postfix percent. Binds tighter than negation: `-50%` is `-0.5`. |
| 3 | `^` | **right** | `2^3^2` = `2^(3^2)` = `512`, matching Excel. |
| 4 | `-x`, `+x` (unary) | right | `-2^2` = `-4`, matching Excel. |
| 5 | `*`, `/` | left | `8/4/2` = `1`. |
| 6 | `+`, `-` | left | `1-2-3` = `-4`. |
| 7 | `=`, `<>`, `<`, `>`, `<=`, `>=` | left | Lowest. Result `1` or `0`. |

### 5.3 Ranges

`A1:D1` means the rectangle from A1 to D1 inclusive. A range:

- is **only** valid as a function argument — `=A1:D1` on its own is `#VALUE!`;
- may be written in either direction (`D1:A1` is the same rectangle);
- may span rows and columns (`A1:C5` is 15 cells);
- **skips empty cells and text** when summed or averaged — this is Excel's
  behaviour and it is what makes subtotalling a column with a text header work;
- promotes to **Currency** if any cell in it is Currency.

By contrast a **scalar** argument (a single cell or a literal) that is text
produces `#VALUE!`. So `=SUM(A1:A3)` tolerates text in A2, but `=SUM(A2, 1)`
does not. This distinction matters and will be tested explicitly.

### 5.4 Your examples, spelled out

| You write | Parses as | Result |
|---|---|---|
| `=SUM(A1:D1)` | function `SUM`, one range argument | Adds A1+B1+C1+D1, skipping text and blanks. |
| `=SUM(A1 + D1)` | function `SUM`, one expression argument | Evaluates `A1 + D1`, then sums that single value. |
| `-=SUM(A1 + D1)` | unary minus on the call | The negated total. Also accepted as `=-SUM(A1+D1)`; the normal Excel form. |
| `=SUM(A1:D1, F1, 10)` | three arguments | Mixed ranges, cells and literals. |
| `=A1 * 1.07` | arithmetic | Currency × Number → Currency. |

`-=` is not standard Excel. It is accepted as a convenience because you wrote it,
and normalised internally to `-SUM(...)`.

**Decided (D13):** `%` is the postfix percent operator, exactly as in Excel, and
remainder is the `MOD(a, b)` function. There is no infix modulo operator.

### 5.5 Case and whitespace

`=sum(a1:d1)`, `=SUM(A1:D1)` and `= Sum ( A1 : D1 )` are identical. Function
names and cell references are case-insensitive; the canonical upper-case form is
what gets stored and displayed.

---

## 6. Errors

| Error | Meaning | Triggered by |
|---|---|---|
| `#VALUE!` | Wrong kind of value | Text in arithmetic; text as a scalar function argument; a bare range. |
| `#DIV/0!` | Division by zero | `=1/0`, `=A1/B1` where B1 is 0 or empty. |
| `#NUM!` | Result out of range | Overflow beyond ±9.2 × 10¹⁸; `=(-1)^0.5`. |
| `#CIRC!` | Circular reference | `A1 = B1`, `B1 = A1`. |
| `#NAME?` | Unknown function | `=SUMM(A1)` — almost always a typo. |
| `#REF!` | Reference points nowhere | A formula referring to a deleted row or column. |
| `#ERR!` | Internal failure | A function panicked. Isolated to one cell. |

Errors propagate through a calculation and are cleared as soon as the cause is
fixed. A cell showing an error displays it centred, in red.

---

## 7. Functions

### 7.1 The contract

Every function is a small, pure, self-registering module:

```go
// internal/formula/functions/sum.go
package functions

import (
    "cellsheet/internal/cell"
    "cellsheet/internal/formula/registry"
)

func init() {
    registry.Register(registry.Spec{
        Name:    "SUM",
        Summary: "Adds all the numbers in a range or list of values.",
        MinArgs: 1,
        MaxArgs: -1, // variadic
        TakesRange: true,
        Fn: func(_ *registry.Ctx, args []cell.Arg) cell.Value {
            total := cell.ZeroNumber()
            for _, a := range args {
                for _, v := range a.Values() { // ranges expand here
                    if v.Skip() {           // empty or text inside a range
                        continue
                    }
                    total = total.Add(v)    // currency promotion happens here
                }
            }
            return total
        },
    })
}
```

That is the **entire** integration. Dropping this file into
`internal/formula/functions/` and rebuilding adds `=SUM()` to the program. No
registry list to update, no switch statement to extend. The mechanism is
explained in [`05-modularity.md`](05-modularity.md).

### 7.2 Guarantees every function gets for free

| Guarantee | Provided by |
|---|---|
| Argument count checked | The registry, against `MinArgs`/`MaxArgs`. |
| `#VALUE!` on bad argument kinds | The registry, before your code runs. |
| Errors propagate from arguments | The registry. |
| Panics become `#ERR!` instead of crashing | The evaluator's recover, per call. |
| Safe to run on any goroutine | By contract: functions must be pure. |
| Result kind promotion | The shared `cell.Value` arithmetic helpers. |

### 7.3 v1 function set

The bar for v1 is `=SUM()`, done correctly. The registry is built so that the
rest are trivial additions:

| Wave | Functions |
|---|---|
| **v1 (Phase 2)** | `SUM` |
| v1.1 | `AVERAGE`, `MIN`, `MAX`, `COUNT`, `COUNTA`, `PRODUCT`, `ABS`, `ROUND`, `MOD` |
| v1.2 | `IF`, `AND`, `OR`, `NOT` (needs boolean values to be settled) |
| v1.3 | `VLOOKUP`, `INDEX`, `MATCH`, `CONCAT`, `LEN`, `UPPER`, `LOWER` |
| Later | `PMT`, `IRR`, `NPV` and friends — the reason exact decimals matter |

Volatile functions (`NOW`, `TODAY`, `RAND`) declare `Volatile: true` and force a
full recalculation whenever anything changes. None are in v1.

---

## 8. Recalculation

- Editing a cell marks it and everything that transitively depends on it as
  **dirty**.
- Dirty cells are sorted into **waves** (see
  [`01-architecture.md`](01-architecture.md) §6). Cells in the same wave are
  independent and run in parallel; waves run in order.
- A cycle is detected when the dependency graph is built and the cells involved
  become `#CIRC!` — the program never hangs.
- Recalculation is **deterministic**: parallel and sequential evaluation must
  produce identical results, and a test asserts it.
- Stale recalculations are abandoned when you keep typing, so results never
  flicker backwards.

---

## 9. Worked examples

Using the data from your mock:

| Cell | Content | Kind | Displayed |
|---|---|---|---|
| A1 | `$50,000.00` | Currency | `$50,000.00` |
| B1 | `$1,250.00` | Currency | `$1,250.00` |
| C1 | `=SUM(A1:B1)` | Currency | `$51,250.00` |
| A2 | `$14,200.50` | Currency | `$14,200.50` |
| B2 | `$320.00` | Currency | `$320.00` |
| C2 | `=$14,520` + `$0.50` | Currency | `$14,520.50` |
| A3 | `Operations` | Text | `Operations` |
| A4 | `450` | Number (integer) | `450` |
| B4 | `12.75` | Number (fractional) | `12.75` |
| C4 | `=A4*B4` | Number | `5737.50` |
| D4 | `YES` | Text | `YES` |
| E1 | `=C1/12` | Currency | `$4,270.83` |
| E2 | `=C1/A1` | Number | `1.025` |
| E3 | `=A3+1` | Error | `#VALUE!` |
| E4 | `=B2/0` | Error | `#DIV/0!` |

Every one of these rows becomes a test case.
