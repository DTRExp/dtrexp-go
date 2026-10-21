package dtrexp

import "time"

// --- Local wall-clock plumbing ----------------------------------------------

// naiveLocal returns t's wall-clock fields in loc re-stamped as UTC, so that
// comparisons and calendar arithmetic are pure calendar (spec §9.3: calendar-
// period cadence windows and bounds spans are local wall-clock intervals).
func naiveLocal(t time.Time, loc *time.Location) time.Time {
	tl := t.In(loc)
	return time.Date(tl.Year(), tl.Month(), tl.Day(), tl.Hour(), tl.Minute(), tl.Second(), tl.Nanosecond(), time.UTC)
}

// resolveCompatible maps a local wall-clock datetime to a single instant per
// Temporal's `compatible` rule (spec §9.3): a repeated local time is the
// EARLIER occurrence; a gap time resolves FORWARD past the gap. Exactly one
// construct needs this — the anchor of an H/m-period cadence.
func resolveCompatible(y, m, d, hh, mm, ss int, loc *time.Location) time.Time {
	naive := time.Date(y, time.Month(m), d, hh, mm, ss, 0, time.UTC)
	// Sample the zone's offset a day either side of the wall time (reading the
	// naive value as a UTC instant is within 14h of any real mapping, so ±24h
	// brackets the transition, if any).
	_, offBefore := naive.Add(-24 * time.Hour).In(loc).Zone()
	_, offAfter := naive.Add(24 * time.Hour).In(loc).Zone()
	candA := naive.Add(-time.Duration(offBefore) * time.Second)
	if offBefore == offAfter {
		return candA
	}
	candB := naive.Add(-time.Duration(offAfter) * time.Second)
	matchA := naiveLocal(candA, loc).Equal(naive)
	matchB := naiveLocal(candB, loc).Equal(naive)
	switch {
	case matchA && matchB: // overlap — the earlier occurrence
		if candA.Before(candB) {
			return candA
		}
		return candB
	case matchA:
		return candA
	case matchB:
		return candB
	default: // gap — the pre-transition offset lands forward, past the gap
		return candA
	}
}

// --- Cadence ----------------------------------------------------------------

func (c *cadence) anchorNaive() time.Time {
	return time.Date(c.anchor.year, time.Month(c.anchor.month), c.anchor.day,
		c.anchor.hour, c.anchor.minute, c.anchor.second, 0, time.UTC)
}

func (c *cadence) covers(t time.Time, loc *time.Location) bool {
	if c.periodUnit == 'H' || c.periodUnit == 'm' {
		return c.coversAbsolute(t, loc)
	}
	return c.coversCalendar(t, loc)
}

// coversAbsolute: H/m periods are absolute elapsed time from the resolved
// anchor instant (spec §9.3); their durations are absolute as well.
// Arithmetic is in int64 seconds, not time.Duration — a Duration is int64
// nanoseconds and saturates at ~±292 years, well inside the 1–9999 year
// domain. Periods, durations and anchors are all whole seconds, and the
// windows are half-open on whole-second edges, so second math is exact.
func (c *cadence) coversAbsolute(t time.Time, loc *time.Location) bool {
	a := c.anchor
	anchor := resolveCompatible(a.year, a.month, a.day, a.hour, a.minute, a.second, loc)
	if t.Before(anchor) {
		return false
	}
	elapsed := t.Unix() - anchor.Unix()
	return elapsed%absoluteSeconds(c.period, c.periodUnit) < absoluteSeconds(c.duration, c.durationUnit)
}

func absoluteSeconds(n int, unit byte) int64 {
	switch unit {
	case 'm':
		return int64(n) * 60
	case 'H':
		return int64(n) * 3600
	case 'D':
		return int64(n) * 86400
	case 'W':
		return int64(n) * 604800
	}
	return 0
}

// coversCalendar: D/W/M/Y periods use naive local-calendar windows — membership
// compares t's local fields against the naive window; the anchor is never
// resolved to an instant (spec §9.3). All arithmetic runs in UTC-stamped naive
// space, which is pure calendar: no zone, no transitions.
func (c *cadence) coversCalendar(t time.Time, loc *time.Location) bool {
	tn := naiveLocal(t, loc)
	an := c.anchorNaive()
	if tn.Before(an) {
		return false
	}
	// Estimate the occurrence index, then scan a small neighbourhood: windows
	// are ordered and non-overlapping (duration < period), so ±2 is ample.
	est := c.estimateIndex(tn)
	for k := est - 2; k <= est+2; k++ {
		if k < 0 {
			continue
		}
		start := c.occStart(k)
		if tn.Before(start) {
			continue
		}
		if tn.Before(c.occEnd(start)) {
			return true
		}
	}
	return false
}

// occStart returns the naive start of occurrence k (k >= 0), constrained per
// spec §9.2 for month/year periods. Calendar periods only. A count may be as
// large as 2³¹ − 1 (spec §8), so the offsets are int64: whole seconds for
// D/W (naive space has no transitions, so a day is always 86,400 seconds)
// and months for M/Y.
func (c *cadence) occStart(k int) time.Time {
	a := c.anchor
	switch c.periodUnit {
	case 'D', 'W':
		return time.Unix(c.anchorNaive().Unix()+int64(k)*absoluteSeconds(c.period, c.periodUnit), 0).UTC()
	case 'M':
		return constrainAddMonths(a.year, a.month, a.day, a.hour, a.minute, a.second, int64(k)*int64(c.period))
	case 'Y':
		return constrainAddMonths(a.year, a.month, a.day, a.hour, a.minute, a.second, int64(k)*int64(c.period)*12)
	}
	return time.Time{}
}

// occEnd returns the exclusive naive end of an occurrence, measured from the
// (already constrained) start. H/m durations under calendar periods are
// local-clock units — "the same arithmetic" as the window they extend. The
// fixed units add int64 seconds, never a time.Duration, which overflows past
// ~292 years.
func (c *cadence) occEnd(start time.Time) time.Time {
	switch c.durationUnit {
	case 'H', 'm', 'D', 'W':
		return time.Unix(start.Unix()+absoluteSeconds(c.duration, c.durationUnit), 0).UTC()
	case 'M':
		return constrainAddMonths(start.Year(), int(start.Month()), start.Day(),
			start.Hour(), start.Minute(), start.Second(), int64(c.duration))
	case 'Y':
		return constrainAddMonths(start.Year(), int(start.Month()), start.Day(),
			start.Hour(), start.Minute(), start.Second(), int64(c.duration)*12)
	}
	return start
}

func (c *cadence) estimateIndex(tn time.Time) int {
	switch c.periodUnit {
	// D/W count elapsed days in int64 seconds, not time.Sub — a Duration
	// saturates at ~±292 years, and the 1–9999 year domain spans ~10,000:
	// a clamped estimate strands the ±2 occurrence scan far below the
	// true index. Both operands are whole-second naive UTC stamps, so
	// second math is exact (same reasoning as coversAbsolute).
	case 'D', 'W':
		return int((tn.Unix() - c.anchorNaive().Unix()) / absoluteSeconds(c.period, c.periodUnit))
	case 'M':
		months := (tn.Year()-c.anchor.year)*12 + (int(tn.Month()) - c.anchor.month)
		return months / c.period
	case 'Y':
		return (tn.Year() - c.anchor.year) / c.period
	}
	return 0
}

// constrainAddMonths adds months to y-m-d, clamping the day to the target
// month's last valid day (Temporal 'constrain'). months is int64: a count of
// years times 12 outgrows 32 bits.
func constrainAddMonths(y, m, d, hh, mm, ss int, months int64) time.Time {
	total := int64(m-1) + months
	ty := int(int64(y) + floorDiv(total, 12))
	tm := int(floorMod(total, 12)) + 1
	td := d
	if dim := daysInMonth(ty, tm); td > dim {
		td = dim
	}
	return time.Date(ty, time.Month(tm), td, hh, mm, ss, 0, time.UTC)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 {
	return a - floorDiv(a, b)*b
}

// --- Bounds -------------------------------------------------------------------

// Bounds spans are local wall-clock intervals like everything else (spec §9.3):
// membership compares t's local fields; a span whose local minute repeats on a
// fall-back day covers both passes.

func spanStart(d dateLit) time.Time {
	return time.Date(d.year, time.Month(d.month), d.day, d.hour, d.minute, d.second, 0, time.UTC)
}

// spanEnd returns the exclusive end of a date literal's span, per its precision.
func spanEnd(d dateLit) time.Time {
	s := spanStart(d)
	switch d.prec {
	case precMinute:
		return s.Add(time.Minute)
	case precSecond:
		return s.Add(time.Second)
	default: // precDay
		return time.Date(d.year, time.Month(d.month), d.day+1, 0, 0, 0, 0, time.UTC)
	}
}

func (b *bounds) covers(t time.Time, loc *time.Location) bool {
	tn := naiveLocal(t, loc)
	if b.hasLower && tn.Before(spanStart(b.lower)) {
		return false
	}
	if b.hasUpper && !tn.Before(spanEnd(b.upper)) {
		return false
	}
	return true
}
