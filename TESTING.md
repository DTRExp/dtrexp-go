# Testing

The package is tested at three levels: the conformance vectors (`testdata/vectors.json`, the behavioral contract), unit tests for everything the vectors don't reach (`unit_test.go`, `covers_test.go`, `helpers_test.go`), and mutation testing with [gremlins](https://github.com/go-gremlins/gremlins).

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

## Coverage: 100% statements

A handful of statements are defensive defaults unreachable through `Parse`/`Covers` (e.g. `daysInMonth` on month 0, `selectorInfo` on an unknown designator, `floorDiv` on negative input, `parseBranch` on a blank string that `parse` already rejects). They are pinned by direct white-box tests in `helpers_test.go` rather than removed, because they guard the internal helpers' own contracts.

`helpers_test.go` also builds a synthetic TZif zone (three transitions within 48h — denser than any real IANA zone) to drive the `resolveCompatible` overlap arm where the ±24h offset samples are inverted; real zones cannot reach it, but a caller-supplied `*time.Location` can.

## Mutation testing

Latest full run: **484 mutants — 467 killed, 11 survivors (all equivalent, justified below), 5 reported "not covered" (tool blind spots, manually verified killed), plus timeouts counted as killed**. Efficacy as reported by gremlins (which counts the 11 equivalents as lived): ~97.7%; unjustified survivors: **0**.

### Justified equivalent survivors (11)

gremlins has no inline suppression mechanism, so equivalents are documented here. (One former row — the `eval_time.go` occurrence-probe loop — is gone: the loop itself was removed when cadence arithmetic moved to exact int64 seconds.) Each is a mutant no test can distinguish because the mutated comparison only differs on inputs the parser/validator has already excluded, or on loop iterations that provably never match.

| Mutant | Site | Why equivalent |
| --- | --- | --- |
| CONDITIONALS_BOUNDARY `eval.go:84` | `if wd < 0` → `<= 0` | `wd` is a parsed E value: 1..7 or -7..-1; `0` and `-0` are parse errors, so `wd == 0` is unreachable. |
| CONDITIONALS_BOUNDARY `eval.go:99` | `if t.ordinal > 0` → `>=` | Ordinal `0` is a parse error ("ordinal zero"), so equality is unreachable. |
| CONDITIONALS_BOUNDARY `eval_time.go:105` | `k <= est+2` → `<` | `estimateIndex` never underestimates the true occurrence index (calendar month/year diffs are ≥ elapsed complete periods; day/week division is exact in naive space), so `est+1`/`est+2` never match. Verified empirically as above. |
| CONDITIONALS_BOUNDARY `eval_time.go:191` | `td > dim` → `>=` | When `td == dim` the clamp assigns `td = dim`, a no-op — identical result. |
| CONDITIONALS_BOUNDARY `eval_time.go:199` (×2) | `a < 0` → `<=`, `b < 0` → `<=` | `a == 0` makes `a%b != 0` false first, short-circuiting identically; `b == 0` would already have panicked at `a / b` (and `b` is always 12 in this package). |
| CONDITIONALS_BOUNDARY `parse.go:320` | `ka[i] < kc[i]` → `<=` | Guarded by `ka[i] != kc[i]`; equality never reaches the comparison. |
| CONDITIONALS_BOUNDARY `parse_value.go:157` | `start.val >= 0` → `> 0` | `start.val == 0` (H/m/s only) can never form a wrap: wrap needs `rs > re` with `re >= 0`, impossible from 0; Y rejects 0 at parse, so the Y-backwards check is unaffected too. |
| CONDITIONALS_BOUNDARY `parse_value.go:252` | `start.val >= 0` → `> 0` | Same shape in the stride wrap check: with `start.val == 0` the guarded `start.val > end.val` is false anyway (`end.val >= 0` required by the same clause). |
| CONDITIONALS_BOUNDARY `parse_value.go:340` | `startMs > endMs` → `>=` | Equal T-range endpoints are rejected two lines earlier ("covers nothing"), so equality is unreachable. |
| CONDITIONALS_BOUNDARY `static.go:166` | `t.start.val >= 0` → `> 0` | `singleValueOf` is only queried for Y/Q/M, whose single values are ≥ 1 after parsing (0 is out of every one of those domains). |

### "Not covered" mutants (5) — tool blind spots, manually verified killed

gremlins only mutates positions present in the coverage profile; `switch`/`case` condition expressions and `const` declarations never appear there, so it refuses to run these five even though the code is exercised. Each was applied by hand and the suite failed (killed):

- `parse.go:68` `case c == '*'` negation — killed (every selector expression misroutes).
- `parse.go:251` `case … sc.peek() == '/'` negation — killed (cadences misparse).
- `parse.go:254` `case … sc.peek() == ':'` negation — killed (bounds misparse).
- `eval.go:130` `minInt = -(1 << 62)` sign flip (both INVERT_NEGATIVES and ARITHMETIC_BASE) — killed (`Y*:2024` open ranges stop matching); the sibling `maxInt` flip is killed too.
