package dtrexp

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, expr string) *Expression {
	t.Helper()
	e, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", expr, err)
	}
	return e
}

func mustInstant(t *testing.T, iso string) time.Time {
	t.Helper()
	ti, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		t.Fatalf("bad instant %q: %v", iso, err)
	}
	return ti
}

func assertCovers(t *testing.T, expr, iso, tz string, want bool) {
	t.Helper()
	e := mustParse(t, expr)
	got, err := e.Covers(mustInstant(t, iso), tz)
	if err != nil {
		t.Fatalf("Covers(%q, %q, %q): %v", expr, iso, tz, err)
	}
	if got != want {
		t.Errorf("Covers(%q, %q, %q) = %v, want %v", expr, iso, tz, got, want)
	}
}

// TestCoversExtra exercises evaluation branches the vector suite misses:
// quarter-scoped ordinals, week-period cadences, and every cross-unit
// duration combination.
func TestCoversExtra(t *testing.T) {
	cases := []struct {
		expr, iso, tz string
		want          bool
	}{
		// ordinal E scoped to the quarter (Q co-present, no M)
		{"E5#2 Q1", "2024-01-12T12:00:00Z", "", true},  // 2nd Friday of Q1 2024
		{"E5#2 Q1", "2024-01-05T12:00:00Z", "", false}, // 1st Friday
		{"E5#-1 Q1", "2024-03-29T12:00:00Z", "", true}, // last Friday of Q1 2024
		{"E5#-1 Q1", "2024-03-22T12:00:00Z", "", false},

		// week-period calendar cadence (occStart/estimateIndex 'W')
		{"20200106/2W", "2020-01-06T00:00:00Z", "", true},
		{"20200106/2W", "2020-01-12T23:59:59Z", "", true},
		{"20200106/2W", "2020-01-13T00:00:00Z", "", false},
		{"20200106/2W", "2020-01-20T00:00:00Z", "", true},
		{"20200106/2W", "2020-01-05T23:59:59Z", "", false}, // before anchor

		// week period with day duration (unitMinSeconds 'W')
		{"20200106/2W/1D", "2020-01-06T12:00:00Z", "", true},
		{"20200106/2W/1D", "2020-01-07T00:00:00Z", "", false},
		{"20200106/2W/1D", "2020-01-20T12:00:00Z", "", true},

		// absolute (H-period) cadences with D and W durations (absoluteLen)
		{"20200101T0000/100H/2D", "2020-01-01T01:00:00Z", "", true},
		{"20200101T0000/100H/2D", "2020-01-03T02:00:00Z", "", false},
		{"20200101T0000/100H/2D", "2020-01-05T05:00:00Z", "", true},
		{"20200101T0000/400H/1W", "2020-01-03T00:00:00Z", "", true},
		{"20200101T0000/400H/1W", "2020-01-08T00:00:00Z", "", false},

		// minute duration under a calendar period (occEnd 'm')
		{"20200101/1D/30m", "2020-01-01T00:15:00Z", "", true},
		{"20200101/1D/30m", "2020-01-01T00:45:00Z", "", false},
		{"20200101/1D/30m", "2020-01-02T00:15:00Z", "", true},

		// week duration under a month period (occEnd 'W', unitMaxSeconds 'W')
		{"20200101/1M/1W", "2020-01-03T00:00:00Z", "", true},
		{"20200101/1M/1W", "2020-01-10T00:00:00Z", "", false},
		{"20200101/1M/1W", "2020-02-03T00:00:00Z", "", true},

		// year duration under a month period (occEnd 'Y', unitMaxSeconds 'Y')
		{"20200101/24M/1Y", "2020-06-01T00:00:00Z", "", true},
		{"20200101/24M/1Y", "2021-06-01T00:00:00Z", "", false},
		{"20200101/24M/1Y", "2022-06-01T00:00:00Z", "", true},

		// month duration under a year period (unitMaxSeconds 'M')
		{"20200101/2Y/1M", "2020-01-15T00:00:00Z", "", true},
		{"20200101/2Y/1M", "2020-02-15T00:00:00Z", "", false},
		{"20200101/2Y/1M", "2022-01-15T00:00:00Z", "", true},

		// minute duration under an absolute period (unitMaxSeconds 'm')
		{"20200101T0000/2H/30m", "2020-01-01T00:10:00Z", "", true},
		{"20200101T0000/2H/30m", "2020-01-01T00:40:00Z", "", false},
		{"20200101T0000/2H/30m", "2020-01-01T02:10:00Z", "", true},

		// hour duration under a minute period (unitMinSeconds 'm')
		{"20200101T0000/120m/1H", "2020-01-01T00:30:00Z", "", true},
		{"20200101T0000/120m/1H", "2020-01-01T01:30:00Z", "", false},
		{"20200101T0000/120m/1H", "2020-01-01T02:30:00Z", "", true},

		// long-span December-anchored month cadence: the occurrence-index
		// estimate must be exact within ±2 across 10 years and a month-12 anchor
		{"20201201/1M/1D", "2030-12-01T12:00:00Z", "", true},
		{"20201201/1M/1D", "2030-12-02T12:00:00Z", "", false},

		// ordinal occurrence arithmetic at the 7-day boundary (Jan 7/14 2024 are Sundays)
		{"E7#1", "2024-01-07T12:00:00Z", "", true},
		{"E7#1", "2024-01-14T12:00:00Z", "", false},
		{"E7#2", "2024-01-14T12:00:00Z", "", true},

		// equal-endpoint range is a single value, not a wrap
		{"M3:3", "2024-03-15T00:00:00Z", "", true},
		{"M3:3", "2024-04-15T00:00:00Z", "", false},

		// T millisecond-of-day arithmetic (hour AND minute parts nonzero)
		{"T0930:1730", "2024-01-15T12:00:00Z", "", true},
		{"T0930:1730", "2024-01-15T09:30:00Z", "", true},
		{"T0930:1730", "2024-01-15T05:00:00Z", "", false},
		{"T0930:1730", "2024-01-15T17:30:00Z", "", false},

		// T boundary values at the top of their domains
		{"T23", "2024-01-15T23:30:00Z", "", true},
		{"T23", "2024-01-15T22:59:59Z", "", false},
		{"T2359", "2024-01-15T23:59:30Z", "", true},
		{"T235959", "2024-01-15T23:59:59.5Z", "", true},

		// minute/second-precision bounds spans
		{"20200101T2359", "2020-01-01T23:59:30Z", "", true},
		{"20200101T2359", "2020-01-02T00:00:00Z", "", false},
		{"20200101T235959", "2020-01-01T23:59:59.2Z", "", true},

		// open Y ranges (pin the unbounded-domain sentinels)
		{"Y*:2024", "2020-06-01T00:00:00Z", "", true},
		{"Y*:2024", "2025-06-01T00:00:00Z", "", false},
		{"Y2020:*", "2024-06-01T00:00:00Z", "", true},
		{"Y2020:*", "2019-06-01T00:00:00Z", "", false},
	}
	for _, c := range cases {
		assertCovers(t, c.expr, c.iso, c.tz, c.want)
	}
}

// TestCadenceDST pins the Temporal `compatible` anchor resolution for H/m
// cadences around real DST transitions (America/New_York, 2024).
func TestCadenceDST(t *testing.T) {
	const ny = "America/New_York"
	cases := []struct {
		expr, iso string
		want      bool
	}{
		// anchor the day BEFORE spring-forward: only the pre-transition offset
		// maps the wall time (matchA); 12:00 EST = 17:00Z
		{"20240309T1200/2H", "2024-03-09T17:30:00Z", true},
		{"20240309T1200/2H", "2024-03-09T16:30:00Z", false},
		// anchor later ON the spring-forward day: only the post-transition
		// offset maps it (matchB); 12:00 EDT = 16:00Z
		{"20240310T1200/2H", "2024-03-10T16:30:00Z", true},
		{"20240310T1200/2H", "2024-03-10T15:30:00Z", false},
		// anchor in the repeated hour (fall-back): the EARLIER occurrence;
		// 01:30 EDT = 05:30Z, not 01:30 EST = 06:30Z
		{"20241103T0130/24H", "2024-11-03T05:45:00Z", true},
		{"20241103T0130/24H", "2024-11-03T06:45:00Z", false},
		// anchor in the gap: resolves FORWARD past it; 02:30 -> 03:30 EDT = 07:30Z
		{"20240310T0230/24H", "2024-03-10T07:45:00Z", true},
		{"20240310T0230/24H", "2024-03-10T06:45:00Z", false},
	}
	for _, c := range cases {
		assertCovers(t, c.expr, c.iso, ny, c.want)
	}
}

// TestAPIPlumbing covers Source, the tz error path, and the nil-Location
// default of CoversIn.
func TestAPIPlumbing(t *testing.T) {
	e := mustParse(t, "M1")
	if e.Source() != "M1" {
		t.Errorf("Source() = %q, want %q", e.Source(), "M1")
	}
	jan := mustInstant(t, "2024-01-15T12:00:00Z")
	if got, err := e.Covers(jan, "Not/AZone"); err == nil || got {
		t.Errorf("Covers with bad zone: got (%v, %v), want (false, error)", got, err)
	}
	if !e.CoversIn(jan, nil) {
		t.Error("CoversIn(jan, nil) = false, want true (nil means UTC)")
	}
	if mustParse(t, "M2").CoversIn(jan, nil) {
		t.Error("CoversIn(jan, nil) on M2 = true, want false")
	}
	// "" and "UTC" both mean UTC
	for _, tz := range []string{"", "UTC"} {
		got, err := e.Covers(jan, tz)
		if err != nil || !got {
			t.Errorf("Covers(jan, %q) = (%v, %v), want (true, nil)", tz, got, err)
		}
	}
}
