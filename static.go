package dtrexp

import "fmt"

// analyzeWarnings implements the §9.1 required-minimum static analysis:
//   - per-selector satisfiability against the domain sizes implied by co-present
//     selectors (catches D30 M2, W53 Y2021, statically-empty non-wrap ranges
//     like D-1:5 and M-2:2, and full-domain exclusions like M!1:12),
//   - M ∩ Q disjointness (M-1 Q1).
//
// Warnings from every union branch surface on the whole expression, each
// positioned at the offending selector's designator.
func analyzeWarnings(e *Expression) []Warning {
	var out []Warning
	for i, b := range e.branches {
		if reason, pos := branchUnsatisfiable(b); reason != "" {
			if len(e.branches) > 1 {
				reason = fmt.Sprintf("branch %d: %s", i+1, reason)
			}
			out = append(out, Warning{Pos: pos, Message: reason})
		}
	}
	return out
}

// branchUnsatisfiable returns the warning message and the source offset of the
// offending selector, or ("", -1) when the branch is satisfiable.
func branchUnsatisfiable(b *branch) (string, int) {
	for _, s := range b.selectors {
		if hasOrdinal(s) || s.des == desY {
			continue // ordinals need occurrence context; Y is unbounded
		}
		sizes := possibleSizes(b, s)
		if !selectorSatisfiable(s, sizes) {
			return fmt.Sprintf("unsatisfiable — selector %q matches nothing in its domain", string(s.des)), s.pos
		}
	}
	if reason, pos := monthQuarterDisjoint(b); reason != "" {
		return reason, pos
	}
	return "", -1
}

func hasOrdinal(s *selector) bool {
	for _, t := range s.terms {
		if t.kind == termOrdinal {
			return true
		}
	}
	return false
}

// possibleSizes returns the candidate maximum domain values for a selector,
// given co-present scoping selectors.
func possibleSizes(b *branch, s *selector) []int {
	switch s.des {
	case desW:
		if y, ok := singleValueOf(b, desY); ok {
			return []int{weeksInISOYear(y)}
		}
		return []int{52, 53}
	case desD:
		switch s.scope {
		case desY:
			// with W present, Y is the ISO week-year while day-of-year stays
			// calendar (spec §2) — cross-selector territory, stays quiet (§9.1)
			if !b.present[desW] {
				if y, ok := singleValueOf(b, desY); ok {
					return []int{daysInYear(y)}
				}
			}
			return []int{365, 366}
		case desQ:
			if q, ok := singleValueOf(b, desQ); ok {
				return dedupe([]int{quarterDays(2001, q), quarterDays(2000, q)})
			}
			return []int{90, 91, 92}
		default: // month
			if m, ok := singleValueOf(b, desM); ok {
				if m == 2 {
					return []int{28, 29}
				}
				return []int{daysInMonth(2001, m)}
			}
			return []int{28, 29, 30, 31}
		}
	default:
		return []int{domainSpec(s.des, s.scope).maxV}
	}
}

// selectorSatisfiable reports whether the selector matches at least one value in
// at least one of the candidate domains.
func selectorSatisfiable(s *selector, sizes []int) bool {
	spec := domainSpec(s.des, s.scope)
	for _, size := range sizes {
		for v := spec.minV; v <= size; v++ {
			if matchesValue(s, v, spec.minV, size) {
				return true
			}
		}
	}
	return false
}

func matchesValue(s *selector, v, minVal, maxVal int) bool {
	member := false
	for i := range s.terms {
		if s.terms[i].member(v, minVal, maxVal, false) {
			member = true
			break
		}
	}
	if s.exclude {
		return !member
	}
	return member
}

// monthQuarterDisjoint warns when co-present M and Q select disjoint month
// sets; the warning is positioned at the M selector. It returns ("", -1) when
// there is nothing to warn about.
func monthQuarterDisjoint(b *branch) (string, int) {
	var mSel, qSel *selector
	for _, s := range b.selectors {
		switch s.des {
		case desM:
			mSel = s
		case desQ:
			qSel = s
		}
	}
	if mSel == nil || qSel == nil {
		return "", -1
	}
	months := map[int]bool{}
	for m := 1; m <= 12; m++ {
		if matchesValue(mSel, m, 1, 12) {
			months[m] = true
		}
	}
	for q := 1; q <= 4; q++ {
		if !matchesValue(qSel, q, 1, 4) {
			continue
		}
		for _, m := range []int{(q-1)*3 + 1, (q-1)*3 + 2, (q-1)*3 + 3} {
			if months[m] {
				return "", -1 // some month lies in a selected quarter
			}
		}
	}
	return "unsatisfiable — M and Q select disjoint months", mSel.pos
}

// singleValueOf returns the single positive value a designator selects, if the
// selector is exactly one plain value (used to pin down domain sizes).
func singleValueOf(b *branch, des designator) (int, bool) {
	if !b.present[des] {
		return 0, false
	}
	for _, s := range b.selectors {
		if s.des != des {
			continue
		}
		if s.exclude || len(s.terms) != 1 {
			return 0, false
		}
		t := s.terms[0]
		if t.kind == termSingle && !t.start.star && t.start.val >= 0 {
			return t.start.val, true
		}
		return 0, false
	}
	return 0, false
}

func quarterDays(y, q int) int {
	m := (q-1)*3 + 1
	return daysInMonth(y, m) + daysInMonth(y, m+1) + daysInMonth(y, m+2)
}

func dedupe(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
