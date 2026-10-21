# Testing

The package is tested at three levels: the conformance vectors (`testdata/vectors.json`, the behavioral contract), unit tests for everything the vectors don't reach (`unit_test.go`, `covers_test.go`, `zone_test.go`, `helpers_test.go`), and mutation testing with [gremlins](https://github.com/go-gremlins/gremlins).

## Commands

```sh
make test      # go test ./...
make cover     # 100% statement coverage, enforced
make mutation  # gremlins mutation testing (requires: make install-gremlins)
```

Raw equivalents:

```sh
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
gremlins unleash --timeout-coefficient 30 --workers 4
```

The `--timeout-coefficient 30` matters: the suite runs in ~0.4s, so gremlins' default per-mutant timeout is smaller than `go test` compilation time and every mutant falsely times out without it.

## Coverage: 100% Statements

A handful of statements are defensive defaults unreachable through `Parse`/`Covers` (e.g. `daysInMonth` on month 0, `selectorInfo` on an unknown designator, `floorDiv` on negative input, `parseBranch` on a blank string that `parse` already rejects). They are pinned by direct white-box tests in `helpers_test.go` rather than removed, because they guard the internal helpers' own contracts.

`helpers_test.go` also builds a synthetic TZif zone (three transitions within 48h, denser than any real IANA zone) to drive the `resolveCompatible` overlap arm where the ±24h offset samples are inverted; real zones cannot reach it, but a caller-supplied `*time.Location` can.

## The Zone List

`zones.go` carries the tz database's zone and link names (release 2026c) that `Covers` and `WithZone` accept. `TestZoneNamesLoad` loads every listed name and compares the list with the Go toolchain's own `$GOROOT/lib/time/zoneinfo.zip`, minus `Factory`; it skips the comparison when no `go` command is on the path. A toolchain whose database adds or drops a name fails the test until `zones.go` follows: regenerate it from the zip's file list (or from a tz release's `tzdata.zi`, its `Z` and `L` lines), leaving `Factory` out.

## Mutation Testing

Latest full run (2026-10-21, draft 3: error positions, the integer limit, the zone-name list, value-set lint narrowing), with `zones.go` rerun on its own after its last test was added: **520 mutants: 497 killed, 9 survivors (all equivalent, justified below), 7 reported "not covered" (tool blind spots, manually verified killed), 7 timeouts counted as killed**. Efficacy as reported by gremlins (which counts the 9 equivalents as lived): 98.22%; unjustified survivors: **0**.

Rerun of `parse.go` and `parse_value.go` (2026-10-21, after the parser began reading on past a rule error so that the earliest error wins, and checking every token as it is read but for D's domain, which waits for the branch): **273 mutants: 251 killed, 1 timeout counted as killed, 2 lived, 19 not covered**. The two that lived are the parser equivalents below; three more lived on the first pass (a D exclusion's and a later D list item's offsets, and a D stride interval equal to its domain) and were killed by new tests. The nineteen not covered are the parser blind spots below, every one applied by hand and killed.

### Justified Equivalent Survivors (8)

gremlins has no inline suppression mechanism, so equivalents are documented here. (Three former rows are gone with their code: the `eval_time.go` occurrence-probe loop, removed when cadence arithmetic moved to exact int64 seconds; `compareLit`, removed when bounds began comparing the first span's start with the last span's end; and `singleValueOf`, removed when the lint began narrowing domains from every selector form.) Each is a mutant no test can distinguish because the mutated comparison only differs on inputs the parser/validator has already excluded, or on loop iterations that provably never match.

| Mutant | Site | Why equivalent |
| --- | --- | --- |
| CONDITIONALS_BOUNDARY `eval.go:84` | `if wd < 0` → `<= 0` | `wd` is a parsed E value: 1..7 or -7..-1; `0` and `-0` are parse errors, so `wd == 0` is unreachable. |
| CONDITIONALS_BOUNDARY `eval.go:99` | `if t.ordinal > 0` → `>=` | Ordinal `0` is a parse error ("ordinal zero"), so equality is unreachable. |
| CONDITIONALS_BOUNDARY `eval_time.go:105` | `k <= est+2` → `<` | `estimateIndex` never underestimates the true occurrence index (calendar month/year diffs are ≥ elapsed complete periods; day/week division is exact in naive space), so `est+1`/`est+2` never match. Verified empirically as above. |
| CONDITIONALS_BOUNDARY `eval_time.go:183` | `td > dim` → `>=` | When `td == dim` the clamp assigns `td = dim`, a no-op; identical result. |
| CONDITIONALS_BOUNDARY `eval_time.go:191` (×2) | `a < 0` → `<=`, `b < 0` → `<=` | `a == 0` makes `a%b != 0` false first, short-circuiting identically; `b == 0` would already have panicked at `a / b` (and `b` is always 12 in this package). |
| CONDITIONALS_BOUNDARY `parse_value.go:116` | `start.val >= 0` → `> 0` | `start.val == 0` (H/m/s only) can never form a wrap: wrap needs `rs > re` with `re >= 0`, impossible from 0, so the term's `wrapEligible` flag never changes what it covers. |
| CONDITIONALS_BOUNDARY `parse_value.go:234` | `startMs > endMs` → `>=` | Equal T-range endpoints are an error two lines earlier ("covers nothing"), and the branch ends with it, so a range built from them is never evaluated. |

### "Not Covered" Mutants (21) — Tool Blind Spots, Manually Verified Killed

gremlins only mutates positions present in the coverage profile; `switch`/`case` condition expressions and `const` declarations never appear there, so it refuses to run these twenty-one even though the code is exercised. Each was applied by hand and the suite failed (killed):

- `parse.go:74` `case c == '*'` negation: killed (every bounds component misroutes).
- `parse.go:78` `case c == '!'` negation: killed (a misplaced `!` loses its message, and every other unexpected character gains it).
- `parse.go:267` `case items > 1 || first.isRange` negation and boundary: killed (`E1,2#1` reaches the ordinal, and every ordinal is rejected).
- `parse.go:269` `case des != desE` negation: killed (`M1#2` is accepted as far as the ordinal, and `E5#5` rejected).
- `parse.go:282` `case items > 1` negation and boundary: killed (`M1,2/2` reaches the stride, and every single-value stride is rejected as a list).
- `parse.go:377` `case first.start.val < 0` negation and boundary: killed (`D-3/2` strides, and `H0/4` is rejected).
- `parse.go:379` the stride wrap case, five mutants (`des != desY` negated, `first.end.val >= 0` negated and narrowed to `> 0`, `first.start.val > first.end.val` negated and widened to `>=`): killed (`M10:2/2` and `H5:0/2` stride a wrap, or `M3:3/2` and `M1:5/2` are rejected).
- `parse.go:487` `maxInteger = 1<<31 - 1` (both INVERT_NEGATIVES and ARITHMETIC_BASE): killed (the limit moves off 2,147,483,647, and the `E7#2147483648`, `Y1:*/2147483648` and `20200101/2147483648D` vectors parse).
- `parse.go:561` `case … sc.peek() == '/'` negation: killed (cadences misparse).
- `parse.go:564` `case … sc.peek() == ':'` negation: killed (bounds misparse).
- `parse_value.go:60` `case vp[0] == '!'` negation: killed (every list is built as an exclusion).
- `eval.go:130` `minInt = -(1 << 62)` sign flip (both INVERT_NEGATIVES and ARITHMETIC_BASE): killed (`Y*:2024` open ranges stop matching); the sibling `maxInt` flip is killed too.
