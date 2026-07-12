package dtrexp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestParseErrors exercises every parse/validation error path not reached by
// the conformance vectors, pinning both the message and the reported 0-based
// character position.
func TestParseErrors(t *testing.T) {
	cases := []struct {
		expr string
		want string // substring expected in the error
		pos  int    // expected ParseError.Pos
	}{
		// boundary killers: index-0 markers, position arithmetic, duration floor
		{"M!/3", "exclusion cannot carry a stride or ordinal", 2},
		{"M!#2", "exclusion cannot carry a stride or ordinal", 2},
		{"M/3", `invalid value ""`, 1},
		{"T/2", "T takes no stride", 1},
		{"T#1", "T takes no ordinal", 1},
		{"T0900!1200", "T takes no exclusion", 5},
		{"T0900:9900", "hour out of range in time value", 6},
		{"H0/4/4", "stride duration must satisfy", 5},
		// designators
		{"a1", "unknown designator", 0},
		{"z1", "unknown designator", 0},
		{"A1", "unknown designator", 0},
		{"Z1", "unknown designator", 0},
		{"M", "requires a value", 0},
		{"M1 M2", "duplicate designator", 3},
		{"M1 %", "unexpected character", 3},
		// union branches
		{"|M1", "empty union branch", 0},
		{"M1 |", "empty union branch", 4},
		{"M1 | M99", "out of domain", 6}, // positions are global, not branch-local
		// date literals & bounds
		{"2020", "8 digits", 0},
		{"19000229", "not a real calendar date", 0}, // 1900 is not a leap year
		{"20200101T2400", "invalid time in date literal", 9},
		{"20200101T2360", "invalid time in date literal", 9},
		{"20200101T235960", "invalid time in date literal", 9},
		{"20201301", "not a real calendar date", 0},
		{"20200001", "not a real calendar date", 0},
		{"20200101 20200202", "more than one bounds", 9},
		{"20200101:2020", "8 digits", 9},
		{"20191231:20190101", "backwards bounds range", 9},
		{"*", "unexpected", 0},
		{"*x", "unexpected", 0},
		{"*:2020", "8 digits", 2},
		{"*:*", "at least one date-literal endpoint", 2},
		{"20200101 *:20220101", "more than one bounds", 9},
		// cadence
		{"20200101/2D/", "cadence missing count", 12},
		{"20200101/D", "cadence missing count", 9},
		{"20200101/2", "cadence missing unit", 10},
		{"20200101/2X", "unknown cadence unit", 10},
		{"20200101/0D", "zero cadence period", 9},
		{"20200101/2D 20200301/2D", "more than one cadence", 12},
		{"20200101/1D", "duration must be < period", 9}, // defaulted duration
		{"20200101/2D/0D", "duration must be >= 1", 12},
		{"20200101/1W/7D", "not conservatively smaller", 12}, // 7D == 1W exactly
		{"20200101/40D/1M", "month/year duration unit", 13},
		// value lists / items
		{"M1!2", "only immediately after the designator", 2},
		{"M!1/2", "stride or ordinal", 3},
		{"M!1#2", "stride or ordinal", 3},
		{"M!99", "out of domain", 2},
		{"M1,", "empty value item", 3},
		{"M*,1", "bare", 1},
		{"M1,*", "bare", 3},
		// ranges
		{"M1:2:3", "malformed range", 1},
		{"M99:2", "out of domain", 1},
		{"M1:99", "out of domain", 3},
		{"Y2024:2020", "backwards range on Y", 1},
		{"Y0", "out of domain", 1}, // zero is out of domain, not "negative"
		{"M-", "invalid value", 1},
		// ordinals
		{"M1#2", "only valid on E", 2},
		{"E1#2#3", "malformed ordinal", 1},
		{"E9#2", "out of domain", 1},
		{"E*#2", "ordinal weekday must be a value", 1},
		{"E1#", "invalid ordinal", 3},
		// strides
		{"M1/2/3/4", "malformed stride", 1},
		{"M1,2/2", "stride not allowed on a list", 4},
		{"M1:2:3/2", "malformed stride range", 1},
		{"M99:5/2", "out of domain", 1},
		{"M1:99/2", "out of domain", 3},
		{"M10:2/2", "wrap ranges take no stride", 1},
		{"H5:0/2", "wrap ranges take no stride", 1}, // zero end is still a wrap
		{"M99/2", "out of domain", 1},
		{"M1/", "invalid stride interval", 3},
		{"M1/3/", "invalid stride duration", 5},
		// T selector
		{"T12#1", "no ordinal", 3},
		{"T!12", "no exclusion", 1},
		{"T12/2", "no stride", 3},
		{"T12,", "empty T item", 4},
		{"T10:12:14", "malformed T range", 1},
		{"T*", `no "*"`, 1},
		{"T*:12", `no "*"`, 1},
		{"T12:*", `no "*"`, 4},
		{"T120000.12", "malformed millisecond", 8},
		{"T.12", "malformed millisecond", 2}, // fraction validated before the empty HH part
		{"T-2", "malformed time value", 1},
		{"T126000", "minute out of range", 1},
		{"T120060", "second out of range", 1},
		{"T123", "malformed time value", 1},
		{"T.500", "malformed time value", 1},
	}
	for _, c := range cases {
		_, err := Parse(c.expr)
		if err == nil {
			t.Errorf("Parse(%q): expected error containing %q, got nil", c.expr, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q): error %q does not contain %q", c.expr, err.Error(), c.want)
		}
		var pe ParseError
		if !errors.As(err, &pe) {
			t.Errorf("Parse(%q): error is %T, want ParseError", c.expr, err)
			continue
		}
		if pe.Pos != c.pos {
			t.Errorf("Parse(%q): error at %d, want %d (%s)", c.expr, pe.Pos, c.pos, pe.Msg)
		}
	}
}

// TestParseValid pins boundary-valid expressions that must parse cleanly (the
// counterparts of the boundary errors above).
func TestParseValid(t *testing.T) {
	valid := []string{
		// domain edges
		"Y1", "Y9999", "Y2020:2020",
		"E5#5", "E5#-5",
		"M1/12",  // stride interval == domain size
		"M3:3/2", // equal endpoints are not a wrap
		// calendar edges
		"20000229", // 2000 IS a leap year
		"20200101", "20201231",
		// time-part edges
		"20200101T2359", "20200101T235959",
		"T23", "T2359", "T235959",
	}
	for _, expr := range valid {
		if _, err := Parse(expr); err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", expr, err)
		}
	}
}

// TestWarningBranchPrefix pins the warning text: multi-branch expressions
// prefix warnings with the 1-based branch index; single-branch ones don't.
// Positions are global offsets of the offending selector's designator.
func TestWarningBranchPrefix(t *testing.T) {
	e := mustParse(t, "M1 | D30 M2")
	if w := e.Warnings(); len(w) != 1 || !strings.Contains(w[0].Message, "branch 2:") || w[0].Pos != 5 {
		t.Errorf("multi-branch warnings = %v, want one 'branch 2:' warning at pos 5", e.Warnings())
	}
	e = mustParse(t, "D30 M2")
	w := e.Warnings()
	if len(w) != 1 || strings.Contains(w[0].Message, "branch") || !strings.Contains(w[0].Message, "unsatisfiable") {
		t.Errorf("single-branch warnings = %v, want one unprefixed 'unsatisfiable' warning", w)
	}
	if len(w) == 1 && w[0].Pos != 0 {
		t.Errorf("single-branch warning pos = %d, want 0 (the D selector)", w[0].Pos)
	}
}

// TestWarningPositions pins Warning.Pos for each analysis arm: the
// unsatisfiable selector's designator, and the M selector for M/Q
// disjointness.
func TestWarningPositions(t *testing.T) {
	cases := []struct {
		expr string
		pos  int
	}{
		{"D30 M2", 0},  // the impossible D selector
		{"M2 D30", 3},  // same warning, D later in the source
		{"Q2 D92", 3},  // quarter-scoped D
		{"M3 Q2", 0},   // M/Q disjoint -> the M selector
		{"Q2 M3", 3},   // M/Q disjoint, M later in the source
		{"T09 M!*", 4}, // full-domain exclusion, offset past the T component
	}
	for _, c := range cases {
		e := mustParse(t, c.expr)
		w := e.Warnings()
		if len(w) != 1 {
			t.Errorf("Parse(%q): warnings = %v, want exactly one", c.expr, w)
			continue
		}
		if w[0].Pos != c.pos {
			t.Errorf("Parse(%q): warning pos = %d, want %d (%s)", c.expr, w[0].Pos, c.pos, w[0].Message)
		}
	}
}

// TestParseErrorType pins the ParseError rendering and both recovery routes:
// errors.As and a plain type assertion.
func TestParseErrorType(t *testing.T) {
	_, err := Parse("M99")
	if err == nil {
		t.Fatal("Parse(\"M99\") = nil error, want ParseError")
	}
	pe, ok := err.(ParseError)
	if !ok {
		t.Fatalf("Parse error is %T, want ParseError", err)
	}
	if pe.Pos != 1 || !strings.Contains(pe.Msg, "out of domain") {
		t.Errorf("ParseError = %+v, want out-of-domain at pos 1", pe)
	}
	if got, want := pe.Error(), `dtrexp: value 99 out of domain for "M" (at 1)`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	var as ParseError
	if !errors.As(err, &as) || as != pe {
		t.Errorf("errors.As recovered %+v, want %+v", as, pe)
	}
}

// TestValidate pins the Validate contract: never a Go error; invalid input
// comes back as positioned data, and warnings match Expression.Warnings.
func TestValidate(t *testing.T) {
	// clean and valid
	r := Validate("M1")
	if !r.Valid || len(r.Errors) != 0 || len(r.Warnings) != 0 {
		t.Errorf("Validate(\"M1\") = %+v, want valid with no errors or warnings", r)
	}

	// valid but warned — same content as the instance's Warnings
	r = Validate("D30 M2")
	if !r.Valid || len(r.Errors) != 0 || len(r.Warnings) != 1 {
		t.Fatalf("Validate(\"D30 M2\") = %+v, want valid with one warning", r)
	}
	e := mustParse(t, "D30 M2")
	if ew := e.Warnings(); len(ew) != 1 || ew[0] != r.Warnings[0] {
		t.Errorf("Validate warnings %v != Expression warnings %v", r.Warnings, ew)
	}
	if r.Warnings[0].String() != r.Warnings[0].Message {
		t.Errorf("Warning.String() = %q, want the message %q", r.Warnings[0].String(), r.Warnings[0].Message)
	}

	// invalid — one positioned error, no warnings
	r = Validate("M99")
	if r.Valid || len(r.Warnings) != 0 {
		t.Fatalf("Validate(\"M99\") = %+v, want invalid with no warnings", r)
	}
	if len(r.Errors) != 1 {
		t.Fatalf("Validate(\"M99\").Errors = %v, want exactly one", r.Errors)
	}
	if got := r.Errors[0]; got.Pos != 1 || !strings.Contains(got.Msg, "out of domain") {
		t.Errorf("Validate(\"M99\").Errors[0] = %+v, want out-of-domain at pos 1", got)
	}
}

// TestStaticAnalysisExtra exercises §9.1 analysis branches beyond the vectors:
// quarter-scoped day domains, full-domain exclusion, and the non-disjoint
// month/quarter quiet path.
func TestStaticAnalysisExtra(t *testing.T) {
	warn := []string{
		"Q2 D92", // Q2 is always 91 days
		"M!*",    // excluding the whole domain matches nothing
		"M3 Q2",  // March is not in Q2
		"M7 Q2",  // July is not in Q2
	}
	for _, expr := range warn {
		e, err := Parse(expr)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", expr, err)
			continue
		}
		if len(e.Warnings()) == 0 {
			t.Errorf("Parse(%q): expected a warning, got none", expr)
		}
	}
	quiet := []string{
		"M1 Q1",          // M and Q agree — no disjointness warning
		"M4 Q2",          // first month of Q2
		"M5 Q2",          // middle month of Q2
		"M6 Q2",          // last month of Q2
		"M12 Q4",         // December, last quarter
		"D29 M2",         // February CAN have 29 days
		"D30 M4",         // April has exactly 30
		"Q1 D91",         // leap-year Q1 has 91 days
		"Q4 D92",         // Q4 always has 92 days
		"D31 M!2",        // excluded M is not a single value — sizes stay open
		"W53 Y2020,2021", // multi-value Y — week domain stays {52,53}
		"W53 Y2020:2021", // range Y — same
		"D31 M1,3",       // multi-value M — day domain stays open
		"Q3,4 D92",       // multi-value Q — day domain stays {90,91,92}
	}
	for _, expr := range quiet {
		e, err := Parse(expr)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", expr, err)
			continue
		}
		if w := e.Warnings(); len(w) > 0 {
			t.Errorf("Parse(%q): expected no warnings, got %v", expr, w)
		}
	}
}

// An H/m-period cadence must stay exact across the full 1–9999 year domain.
// time.Duration (int64 ns) saturates at ~±292 years; the evaluator therefore
// works in int64 seconds. Anchor in year 1, instants in year 9999.
func TestAbsoluteCadenceFarHorizon(t *testing.T) {
	e, err := Parse("00010101/2H")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	anchor := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	t1 := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	for _, tc := range []time.Time{t1, t2} {
		want := (tc.Unix()-anchor)%7200 < 3600
		got, cerr := e.Covers(tc, "UTC")
		if cerr != nil {
			t.Fatalf("covers: %v", cerr)
		}
		if got != want {
			t.Errorf("Covers(%v) = %v, want %v", tc, got, want)
		}
	}
	a, _ := e.Covers(t1, "UTC")
	b, _ := e.Covers(t2, "UTC")
	if a == b {
		t.Errorf("adjacent hours must differ under a 2H/1H cadence: both %v", a)
	}
}

// A stride duration of exactly 1 sits on the 1 <= duration < interval floor
// and must parse (the spec's own lower bound).
func TestStrideDurationFloor(t *testing.T) {
	e, err := Parse("H0/4/1")
	if err != nil {
		t.Fatalf("H0/4/1 must parse: %v", err)
	}
	if len(e.Warnings()) != 0 {
		t.Fatalf("H0/4/1 must be quiet, got %v", e.Warnings())
	}
	ok, _ := e.Covers(time.Date(2024, 1, 1, 4, 30, 0, 0, time.UTC), "UTC") // hour 4, inside
	if !ok {
		t.Error("hour 4 must be covered by H0/4/1")
	}
	ok, _ = e.Covers(time.Date(2024, 1, 1, 5, 30, 0, 0, time.UTC), "UTC") // hour 5, outside
	if ok {
		t.Error("hour 5 must not be covered by H0/4/1")
	}
}
