package dtrexp

import "fmt"

// analyzeWarnings implements the §9.1 required-minimum static analysis:
//   - per-selector satisfiability against the domain sizes implied by co-present
//     selectors (catches D30 M2, D31 M4:6/2, W53 Y2021:2025, statically-empty
//     non-wrap ranges like D-1:5 and M-2:2, and full-domain exclusions like
//     M!1:12 and Y!1:9999),
//   - M ∩ Q disjointness (M-1 Q1).
//
// Warnings from every union branch surface on the whole expression, each
// positioned at the offending selector's designator.
func analyzeWarnings(e *Expression) []Warning {
	var out []Warning
	for i, b := range e.branches {
		if w := branchUnsatisfiable(b); w != nil {
			if len(e.branches) > 1 {
				w.Message = fmt.Sprintf("branch %d: %s", i+1, w.Message)
			}
			out = append(out, *w)
		}
	}
	return out
}

// branchUnsatisfiable returns the warning positioned at the offending
// selector, or nil when the branch is satisfiable.
func branchUnsatisfiable(b *branch) *Warning {
	for _, s := range b.selectors {
		// ordinals need occurrence context; a Y selector selects at least one
		// year unless it is an exclusion
		if hasOrdinal(s) || (s.des == desY && !s.exclude) {
			continue
		}
		sizes := possibleSizes(b, s)
		if !selectorSatisfiable(s, sizes) {
			return &Warning{Pos: s.pos, Message: fmt.Sprintf("unsatisfiable — selector %q matches nothing in its domain", string(s.des))}
		}
	}
	return monthQuarterDisjoint(b)
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
// given co-present scoping selectors: the sizes of every parent instance the
// scoping selector can select, whatever its form (a value, a list, a closed
// span or a stride).
func possibleSizes(b *branch, s *selector) []int {
	switch s.des {
	case desW:
		return scopedSizes(b, desY, []int{52, 53}, func(y int) []int { return []int{weeksInISOYear(y)} })
	case desD:
		switch s.scope {
		case desY:
			// with W present, Y is the ISO week-year while day-of-year stays
			// calendar (spec §2) — cross-selector territory, stays quiet (§9.1)
			if b.present[desW] {
				return []int{365, 366}
			}
			return scopedSizes(b, desY, []int{365, 366}, func(y int) []int { return []int{daysInYear(y)} })
		case desQ:
			return scopedSizes(b, desQ, []int{90, 91, 92}, func(q int) []int {
				return []int{quarterDays(2001, q), quarterDays(2000, q)}
			})
		default: // month
			return scopedSizes(b, desM, []int{28, 29, 30, 31}, func(m int) []int {
				return []int{daysInMonth(2001, m), daysInMonth(2000, m)}
			})
		}
	case desY:
		return []int{9999}
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
// sets; the warning is positioned at the M selector. It returns nil when
// there is nothing to warn about.
func monthQuarterDisjoint(b *branch) *Warning {
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
		return nil
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
				return nil // some month lies in a selected quarter
			}
		}
	}
	return &Warning{Pos: mSel.pos, Message: "unsatisfiable — M and Q select disjoint months"}
}

// scopedSizes returns the domain sizes of the parent instances the branch's
// des selector selects (sizesOf gives one instance's sizes), or all when the
// branch has no such selector or it selects nothing (that selector draws its
// own warning). all lists every size an instance can have, so the walk stops
// once it has found them all.
func scopedSizes(b *branch, des designator, all []int, sizesOf func(int) []int) []int {
	var sel *selector
	for _, s := range b.selectors {
		if s.des == des {
			sel = s
		}
	}
	if sel == nil {
		return all
	}
	spec := domainSpec(des, 0)
	seen := map[int]bool{}
	var out []int
	for v := spec.minV; v <= spec.maxV; v++ {
		if !matchesValue(sel, v, spec.minV, spec.maxV) {
			continue
		}
		for _, n := range sizesOf(v) {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
		if len(out) == len(all) {
			break
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

func quarterDays(y, q int) int {
	m := (q-1)*3 + 1
	return daysInMonth(y, m) + daysInMonth(y, m+1) + daysInMonth(y, m+2)
}
