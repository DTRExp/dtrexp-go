package dtrexp

import "time"

func (b *branch) covers(t time.Time, f fields, loc *time.Location) bool {
	for _, s := range b.selectors {
		if !s.covers(f, b.hasW) {
			return false
		}
	}
	if b.tsel != nil && !b.tsel.covers(f) {
		return false
	}
	if b.cadence != nil && !b.cadence.covers(t, loc) {
		return false
	}
	if b.bounds != nil && !b.bounds.covers(t, loc) {
		return false
	}
	return true
}

// selectorInfo returns the tested field value and the domain edges for a
// designator in a given instance. unbounded is true only for Y.
func selectorInfo(des, scope designator, f fields, hasW bool) (fieldVal, minVal, maxVal int, unbounded bool) {
	switch des {
	case desY:
		if hasW {
			return f.weekYear, 0, 0, true
		}
		return f.calYear, 0, 0, true
	case desQ:
		return f.quarter, 1, 4, false
	case desM:
		return f.month, 1, 12, false
	case desW:
		return f.isoWeek, 1, f.weeksInWeekYear, false
	case desD:
		switch scope {
		case desQ:
			return f.doq, 1, f.daysInQuarter, false
		case desY:
			return f.doy, 1, f.daysInYear, false
		default:
			return f.dom, 1, f.daysInMonth, false
		}
	case desE:
		return f.weekday, 1, 7, false
	case desH:
		return f.hour, 0, 23, false
	case desMin:
		return f.minute, 0, 59, false
	case desS:
		return f.second, 0, 59, false
	}
	return 0, 0, 0, false
}

func (s *selector) covers(f fields, hasW bool) bool {
	// Ordinal E is evaluated against occurrence-in-scope, not a numeric field.
	if s.des == desE && len(s.terms) == 1 && s.terms[0].kind == termOrdinal {
		return s.ordinalCovers(f)
	}

	fv, minVal, maxVal, unbounded := selectorInfo(s.des, s.scope, f, hasW)

	member := false
	for i := range s.terms {
		if s.terms[i].member(fv, minVal, maxVal, unbounded) {
			member = true
			break
		}
	}
	if s.exclude {
		return !member
	}
	return member
}

func (s *selector) ordinalCovers(f fields) bool {
	t := s.terms[0]
	// weekday value (may be negative, resolves against 1..7).
	wd := t.start.val
	if wd < 0 {
		wd = 7 + 1 + wd
	}
	if f.weekday != wd {
		return false
	}
	var dayOfScope, daysInScope int
	switch s.scope {
	case desQ:
		dayOfScope, daysInScope = f.doq, f.daysInQuarter
	case desY:
		dayOfScope, daysInScope = f.doy, f.daysInYear
	default:
		dayOfScope, daysInScope = f.dom, f.daysInMonth
	}
	if t.ordinal > 0 {
		occ := (dayOfScope-1)/7 + 1
		return occ == t.ordinal
	}
	fromEnd := (daysInScope-dayOfScope)/7 + 1
	return fromEnd == -t.ordinal
}

// resolve turns an endpoint into a concrete integer given the domain edges.
// atStart selects which edge '*' means (min for start, max for end).
func (ep endpoint) resolve(atStart bool, minVal, maxVal int, unbounded bool) int {
	if ep.star {
		if unbounded {
			if atStart {
				return minInt
			}
			return maxInt
		}
		if atStart {
			return minVal
		}
		return maxVal
	}
	if ep.val < 0 {
		// Negative resolves against the instance's actual maximum.
		return maxVal + 1 + ep.val
	}
	return ep.val
}

const (
	minInt = -(1 << 62)
	maxInt = 1 << 62
)

func (t term) member(fv, minVal, maxVal int, unbounded bool) bool {
	switch t.kind {
	case termSingle:
		v := t.start.resolve(true, minVal, maxVal, unbounded)
		if !unbounded && (v < minVal || v > maxVal) {
			return false // negative resolved outside this instance's domain
		}
		return fv == v
	case termRange:
		rs := t.start.resolve(true, minVal, maxVal, unbounded)
		re := t.end.resolve(false, minVal, maxVal, unbounded)
		if t.wrapEligible && rs > re {
			return fv >= rs || fv <= re
		}
		return rs <= fv && fv <= re
	case termStride:
		rs := t.start.resolve(true, minVal, maxVal, unbounded)
		re := t.end.resolve(false, minVal, maxVal, unbounded)
		if fv < rs || fv > re {
			return false
		}
		return (fv-rs)%t.interval < t.duration
	}
	return false
}

func (ts *timeSelector) covers(f fields) bool {
	ms := f.msOfDay
	for _, r := range ts.ranges {
		if r.wrap {
			if ms >= r.start || ms < r.end {
				return true
			}
		} else if ms >= r.start && ms < r.end {
			return true
		}
	}
	return false
}
