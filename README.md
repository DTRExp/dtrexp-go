<p align="center"><img src="./.github/logo.svg" width="200" alt="dtrexp-go" /></p>

# dtrexp-go

Go implementation of **[DTRExp](https://github.com/DTRExp/dtrexp)** (read: "**DTR Expression**") — a compact string expression for date-time ranges and recurrence, evaluated by **coverage** rather than enumeration.

```
T0900:1800 E1:5          Mon–Fri, 09:00–18:00
E7#-1 M4                 last Sunday of April, every year
20200106/10D             every 10 days from 2020-01-06 (cron can't say this)
M!7                      every month except July
```

It implements spec draft 3. Scope: **parsing, validation and coverage evaluation** (the spec's core interface). Rendering, description and RRULE export are out of scope; the [reference implementation][js] has them.

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
// an empty string means the bound zone (below), else UTC.

// Preloaded zone; cannot fail:
berlin, _ := time.LoadLocation("Europe/Berlin")
ok = dtr.CoversIn(time.Now(), berlin)

// Bound zone; the no-zone calls evaluate in it:
hours, err := dtr.WithZone("Europe/Berlin")
ok, err = hours.Covers(time.Now(), "")      // Berlin
ok = hours.CoversIn(time.Now(), nil)        // Berlin
ok, err = hours.Covers(time.Now(), "UTC")   // a zone passed to the call wins
```

Note that you parse **once** (at write/config time) and evaluate **many**; `Expression` values are immutable after `Parse` and safe for concurrent use. `Covers` is a single calendar-field extraction followed by integer comparisons; no occurrence iteration.

- `WithZone(zone)` returns a copy whose evaluation defaults to `zone`, readable as `Zone()` in the tz database's spelling (`""` when unbound); the original is left as it is. The copy shares the parsed expression, so its `Source()` and `Warnings()` are the original's: the zone belongs to evaluation, never to the expression. The zone is loaded once, at bind, so an unknown name fails there; so do `""` and `"Local"`, since binding never quietly means UTC or the host's zone.
- `Covers(t, tz)`: an empty `tz` means the bound zone, else UTC; any other `tz`, `"UTC"` included, wins over the bound zone. It returns an error only for a zone it does not know ([Time Zones](#time-zones)) or cannot load.
- `CoversIn(t, loc)`: a nil `loc` means the bound zone, else UTC; a non-nil `loc` wins.

### Time Zones

A zone is named by a tz database zone or link name (`Europe/Berlin`, `US/Eastern`, `UTC`, `Etc/GMT+5`), matched without regard to case: `europe/berlin` binds as `Europe/Berlin`. Anything else is an unknown zone, including a fixed offset (`+02:00`, `UTC+2`), `Z`, `Local`, a path, and a file in the zoneinfo folder that names no zone (`posixrules`, `Factory`, `localtime`). The names are checked against the list the package carries (tz 2026c, the list the Go toolchain bundles), so the answer is the same on every file system; the rules come from wherever `time.LoadLocation` finds them: the host's zoneinfo folder, `$ZONEINFO`, the toolchain's `zoneinfo.zip`, or `time/tzdata` when the program imports it. Import `time/tzdata` to pin the release a binary evaluates with.

**Which to use:** bind the zone with `WithZone` when it belongs to what the expression describes (a store's hours, a venue's schedule), and pass it per call when it belongs to whoever is asking (a rule shown to each user in their own zone). A bound copy shares its `Source()` with the original, so a cache of parsed expressions keys on `Source()` and `Zone()` together.

## Errors and Warnings

Both carry a **position**: the 0-based offset, in Unicode code points, of the first character of the token at fault (spec §8). The grammar is ASCII and parsing stops at the first character outside it, so the offset is also a byte offset into the source:

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
- An integer is at most 2,147,483,647, however many leading zeros it carries (`M0000000003` is `M3`); a larger one is a syntax error at that integer. Domains bound most values far lower; the limit is what bounds `Y` stride intervals and cadence periods and durations.

## Conformance & Quality

- The test suite is driven by the shared [`vectors.json`][vectors] from the spec repo (the draft 3 file): every coverage, rejection, warning and quiet vector, including the calendar traps (Feb 29 across 2000/2024/**2100**, `W53` existence, DST gap/overlap in `Europe/Berlin`); a rejection passes only at the position the vector names. See [VECTORS.md][vectors-md] for how the suite works.
- 100% statement coverage; mutation-tested with [gremlins][gremlins]. Commands and survivor justifications: [TESTING.md](TESTING.md).
- Zero dependencies.

## Related Projects

- [**dtrexp** (spec)][spec] — the DTRExp specification (grammar, semantics, conformance vectors) this package implements.
- [**dtrexp-js**][js] — the reference implementation; adds `intersect`, `next`, `describe`, `toRRule` and canonicalization.
- [**dtrexp-py**][py] · [**dtrexp-swift**][swift] · [**dtrexp-rs**][rs] · [**dtrexp-java**][java] — the other ports; same core interface.
- [**dtrexp-wasm**][wasm] — the Rust core compiled to WebAssembly for JS hosts.

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
[wasm]: https://github.com/DTRExp/dtrexp-wasm
