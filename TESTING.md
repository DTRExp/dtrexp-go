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

### Justified Equivalent Survivors (9)

gremlins has no inline suppression mechanism, so equivalents are documented here. (Three former rows are gone with their code: the `eval_time.go` occurrence-probe loop, removed when cadence arithmetic moved to exact int64 seconds; `compareLit`, removed when bounds began comparing the first span's start with the last span's end; and `singleValueOf`, removed when the lint began narrowing domains from every selector form.) Each is a mutant no test can distinguish because the mutated comparison only differs on inputs the parser/validator has already excluded, or on loop iterations that provably never match.

| Mutant | Site | Why equivalent |
| --- | --- | --- |
| CONDITIONALS_BOUNDARY `eval.go:84` | `if wd < 0` → `<= 0` | `wd` is a parsed E value: 1..7 or -7..-1; `0` and `-0` are parse errors, so `wd == 0` is unreachable. |
| CONDITIONALS_BOUNDARY `eval.go:99` | `if t.ordinal > 0` → `>=` | Ordinal `0` is a parse error ("ordinal zero"), so equality is unreachable. |
| CONDITIONALS_BOUNDARY `eval_time.go:105` | `k <= est+2` → `<` | `estimateIndex` never underestimates the true occurrence index (calendar month/year diffs are ≥ elapsed complete periods; day/week division is exact in naive space), so `est+1`/`est+2` never match. Verified empirically as above. |
| CONDITIONALS_BOUNDARY `eval_time.go:183` | `td > dim` → `>=` | When `td == dim` the clamp assigns `td = dim`, a no-op; identical result. |
| CONDITIONALS_BOUNDARY `eval_time.go:191` (×2) | `a < 0` → `<=`, `b < 0` → `<=` | `a == 0` makes `a%b != 0` false first, short-circuiting identically; `b == 0` would already have panicked at `a / b` (and `b` is always 12 in this package). |
| CONDITIONALS_BOUNDARY `parse_value.go:155` | `start.val >= 0` → `> 0` | `start.val == 0` (H/m/s only) can never form a wrap: wrap needs `rs > re` with `re >= 0`, impossible from 0; Y rejects 0 at parse, so the Y-backwards check is unaffected too. |
| CONDITIONALS_BOUNDARY `parse_value.go:254` | `start.val >= 0` → `> 0` | Same shape in the stride wrap check: with `start.val == 0` the guarded `start.val > end.val` is false anyway (`end.val >= 0` required by the same clause). |
| CONDITIONALS_BOUNDARY `parse_value.go:343` | `startMs > endMs` → `>=` | Equal T-range endpoints are rejected two lines earlier ("covers nothing"), so equality is unreachable. |

### "Not Covered" Mutants (7) — Tool Blind Spots, Manually Verified Killed

gremlins only mutates positions present in the coverage profile; `switch`/`case` condition expressions and `const` declarations never appear there, so it refuses to run these seven even though the code is exercised. Each was applied by hand and the suite failed (killed):

- `parse.go:67` `case c == '*'` negation: killed (every selector expression misroutes).
- `parse.go:216` `maxInteger = 1<<31 - 1` (both INVERT_NEGATIVES and ARITHMETIC_BASE): killed (the limit moves off 2,147,483,647, and the `E7#2147483648`, `Y1:*/2147483648` and `20200101/2147483648D` vectors parse).
- `parse.go:285` `case … sc.peek() == '/'` negation: killed (cadences misparse).
- `parse.go:288` `case … sc.peek() == ':'` negation: killed (bounds misparse).
- `eval.go:130` `minInt = -(1 << 62)` sign flip (both INVERT_NEGATIVES and ARITHMETIC_BASE): killed (`Y*:2024` open ranges stop matching); the sibling `maxInt` flip is killed too.
