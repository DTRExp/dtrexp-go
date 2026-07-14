# dtrexp-go

Go implementation of **[DTRExp](https://github.com/DTRExp/dtrexp)** (read: "**DTR Expression**") — a compact string expression for date-time ranges and recurrence, evaluated by **coverage** rather than enumeration.

```
T0900:1800 E1:5          Mon–Fri, 09:00–18:00
E7#-1 M4                 last Sunday of April, every year
20200106/10D             every 10 days from 2020-01-06 (cron can't say this)
M!7                      every month except July
```

Scope: **parsing, validation and coverage evaluation** (the spec's core interface). Rendering, description and RRULE export are out of scope; the [reference implementation][js] has them.

## Install

```sh
go get github.com/DTRExp/dtrexp-go
```

Go 1.26+, stdlib only (`time` for IANA zones).

## Usage

```go
import (
    "time"
    dtrexp "github.com/DTRExp/dtrexp-go"
)

dtr, err := dtrexp.Parse("T0900:1800 E1:5")    // business hours, Mon–Fri
if err != nil { /* a positioned ParseError */ }

ok, err := dtr.Covers(time.Now(), "Europe/Berlin")
// —> true on a weekday, 09:00–18:00 Berlin local time
// The zone is an evaluation parameter, never part of the expression;
// empty string or "UTC" means UTC.

// Preloaded zone; cannot fail:
berlin, _ := time.LoadLocation("Europe/Berlin")
ok = dtr.CoversIn(time.Now(), berlin)
```

Note that you parse **once** (at write/config time) and evaluate **many**; `Expression` values are immutable after `Parse` and safe for concurrent use. `Covers` is a single calendar-field extraction followed by integer comparisons; no occurrence iteration.

## Errors and warnings

Both carry a **position**; the 0-based character offset into the source:

```go
_, err := dtrexp.Parse("Y*/3")     // anchorless stride — a syntax error
var pe dtrexp.ParseError
errors.As(err, &pe)                // pe.Pos points at the offending character

res := dtrexp.Validate("D30 M2")   // never returns a Go error
res.Valid                          // true — it parses
res.Warnings                       // [{Pos: 0, Message: "unsatisfiable …"}] — no February has 30 days
```

- `Parse(s)` returns the expression or a `ParseError` (*Pos* `int`, *Msg* `string`).
- `Validate(s)` never fails; typo-shaped input comes back as data. Returns a `ValidationResult` with *Valid* `bool`, *Errors* (parsing stops at the first syntax error, so at most one) and *Warnings*.
- Warnings are the spec's [§9.1](https://github.com/DTRExp/dtrexp/blob/main/spec.md#91-the-existence-rule) unsatisfiability lint: expressions that parse but can never match. `dtr.Warnings()` and `Validate(s).Warnings` carry the same content.

## Conformance & quality

- The test suite is driven by the shared [`vectors.json`][vectors] from the spec repo (draft 2.8): every coverage, rejection, warning and quiet vector, including the calendar traps (Feb 29 across 2000/2024/**2100**, `W53` existence, DST gap/overlap in `Europe/Berlin`). See [VECTORS.md][vectors-md] for how the suite works.
- 100% statement coverage; mutation-tested with [gremlins][gremlins]. Commands and survivor justifications: [TESTING.md](TESTING.md).
- Zero dependencies.

## Related projects

- [**dtrexp** (spec)][spec] — the DTRExp specification (grammar, semantics, conformance vectors) this package implements.
- [**dtrexp-js**][js] — the reference implementation; adds `intersect`, `next`, `describe`, `toRRule` and canonicalization.
- [**dtrexp-py**][py] · [**dtrexp-swift**][swift] · [**dtrexp-rs**][rs] · [**dtrexp-java**][java] — the other ports; same core interface.

## License

© 2026, Onur Yıldırım. [**MIT**](LICENSE) License.

[spec]: https://github.com/DTRExp/dtrexp
[js]: https://github.com/DTRExp/dtrexp-js
[py]: https://github.com/DTRExp/dtrexp-py
[swift]: https://github.com/DTRExp/dtrexp-swift
[rs]: https://github.com/DTRExp/dtrexp-rs
[java]: https://github.com/DTRExp/dtrexp-java
[vectors]: https://github.com/DTRExp/dtrexp/blob/main/vectors.json
[vectors-md]: https://github.com/DTRExp/dtrexp/blob/main/VECTORS.md
[gremlins]: https://github.com/go-gremlins/gremlins
