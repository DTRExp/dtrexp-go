package dtrexp

import "strings"

type domSpec struct {
	minV, maxV, size int
	unbounded        bool
}

func domainSpec(des, scope designator) domSpec {
	switch des {
	case desY:
		// the year domain is 1–9999 (spec §3.1); unbounded marks that Y has
		// no parent instance to count back from, wrap around or bound a stride
		return domSpec{minV: 1, maxV: 9999, size: 9999, unbounded: true}
	case desQ:
		return domSpec{minV: 1, maxV: 4, size: 4}
	case desM:
		return domSpec{minV: 1, maxV: 12, size: 12}
	case desW:
		return domSpec{minV: 1, maxV: 53, size: 53}
	case desD:
		switch scope {
		case desQ:
			return domSpec{minV: 1, maxV: 92, size: 92}
		case desY:
			return domSpec{minV: 1, maxV: 366, size: 366}
		default:
			return domSpec{minV: 1, maxV: 31, size: 31}
		}
	case desE:
		return domSpec{minV: 1, maxV: 7, size: 7}
	case desH:
		return domSpec{minV: 0, maxV: 23, size: 24}
	case desMin:
		return domSpec{minV: 0, maxV: 59, size: 60}
	case desS:
		return domSpec{minV: 0, maxV: 59, size: 60}
	}
	return domSpec{}
}

// buildSelector builds an integer-field selector from its value part vp, which
// readSelectorValue has read by the grammar and checked by every rule but D's
// domain. That depends on scope, known once the branch is complete, so D's
// values and stride interval are checked here, the first error returned; at is
// the offset of vp in the full source.
func buildSelector(des, scope designator, vp string, at int) (*selector, error) {
	s := &selector{des: des, scope: scope}
	b := termBuilder{des: des, spec: domainSpec(des, scope)}

	// Bare '*' -> whole domain.
	if vp == "*" {
		s.terms = []term{{kind: termRange, start: endpoint{star: true}, end: endpoint{star: true}}}
		return s, nil
	}

	var err error
	switch {
	case vp[0] == '!': // exclusion: '!' immediately after the designator, before a value list
		s.exclude = true
		s.terms, err = b.list(vp[1:], at+1)
	case strings.ContainsRune(vp, '#'): // ordinal (E only)
		head, tail, _ := strings.Cut(vp, "#")
		ord, _ := parseInteger(tail)
		s.terms = []term{{kind: termOrdinal, start: b.endpoint(head), ordinal: ord}}
	case strings.ContainsRune(vp, '/'): // stride
		var t term
		t, err = b.stride(vp, at)
		s.terms = []term{t}
	default:
		s.terms, err = b.list(vp, at)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// termBuilder builds one selector's terms; spec is its domain.
type termBuilder struct {
	des  designator
	spec domSpec
}

// list builds a comma-separated value list; at is the offset of s in the full
// source.
func (b termBuilder) list(s string, at int) ([]term, error) {
	var terms []term
	off := at
	for _, it := range strings.Split(s, ",") {
		t, err := b.item(it, off)
		if err != nil {
			return nil, err
		}
		terms = append(terms, t)
		off += len(it) + 1 // the item plus its trailing ','
	}
	return terms, nil
}

// item builds a value or a range; at is the offset of it in the full source.
func (b termBuilder) item(it string, at int) (term, error) {
	head, tail, isRange := strings.Cut(it, ":")
	if err := b.checkDay(head, at); err != nil {
		return term{}, err
	}
	start := b.endpoint(head)
	if !isRange {
		return term{kind: termSingle, start: start}, nil
	}
	if err := b.checkDay(tail, at+len(head)+1); err != nil {
		return term{}, err
	}
	end := b.endpoint(tail)
	wrapEligible := !start.star && !end.star && start.val >= 0 && end.val >= 0
	return term{kind: termRange, start: start, end: end, wrapEligible: wrapEligible}, nil
}

// stride builds a stride term; at is the offset of vp in the full source.
func (b termBuilder) stride(vp string, at int) (term, error) {
	parts := strings.Split(vp, "/")
	base := parts[0]
	head, tail, isRange := strings.Cut(base, ":")
	if err := b.checkDay(head, at); err != nil {
		return term{}, err
	}
	start := b.endpoint(head)
	end := endpoint{star: true} // default end: domain edge
	if isRange {
		if err := b.checkDay(tail, at+len(head)+1); err != nil {
			return term{}, err
		}
		end = b.endpoint(tail)
	}
	interval, _ := parseInteger(parts[1])
	if b.des == desD && interval > b.spec.size {
		return term{}, errAt(at+len(base)+1, "stride interval %d exceeds parent domain — use a cadence", interval)
	}
	duration := 1
	if len(parts) == 3 {
		duration, _ = parseInteger(parts[2])
	}
	return term{kind: termStride, start: start, end: end, interval: interval, duration: duration}, nil
}

// endpoint is the value or '*' of the token s.
func (b termBuilder) endpoint(s string) endpoint {
	if s == "*" {
		return endpoint{star: true}
	}
	v, _ := parseInteger(s)
	return endpoint{val: v}
}

// checkDay checks a D value's domain, which its scope sets; the token s is at
// at. Every other domain is checked as the value is read.
func (b termBuilder) checkDay(s string, at int) error {
	if b.des != desD || s == "*" {
		return nil
	}
	v, _ := parseInteger(s)
	return checkDomain(b.des, b.spec, v, at)
}

// checkDomain reports a value v outside the domain spec of des, at at.
func checkDomain(des designator, spec domSpec, v int, at int) error {
	if spec.unbounded { // Y
		if v < 0 {
			return errAt(at, "negative value on Y — no edge to count back from")
		}
		// Y takes 4-digit ISO years (spec §2): 1-9999.
		if v < 1 || v > 9999 {
			return errAt(at, "year %d out of domain (1-9999)", v)
		}
		return nil
	}
	if v < 0 {
		if v < -spec.size {
			return errAt(at, "negative value %d out of domain for %q", v, string(des))
		}
		return nil
	}
	if v < spec.minV || v > spec.maxV {
		return errAt(at, "value %d out of domain for %q", v, string(des))
	}
	return nil
}

// --- Time selector ---------------------------------------------------------

// buildTimeSelector builds the T component from its value part vp; at is the
// offset of vp in the full source. A token the grammar does not allow is the
// error; a value the rules do not allow goes to note, and the build reads on.
func buildTimeSelector(vp string, at int, note func(error)) (*timeSelector, error) {
	if idx := strings.IndexByte(vp, '!'); idx >= 0 {
		return nil, errAt(at+idx, "T takes no exclusion — write the complement explicitly")
	}
	if idx := strings.IndexByte(vp, '/'); idx >= 0 {
		return nil, errAt(at+idx, "T takes no stride")
	}
	if idx := strings.IndexByte(vp, '#'); idx >= 0 {
		return nil, errAt(at+idx, "T takes no ordinal")
	}
	ts := &timeSelector{}
	off := at
	for _, it := range strings.Split(vp, ",") {
		if it == "" {
			return nil, errAt(off, "empty T item")
		}
		if strings.ContainsRune(it, ':') {
			parts := strings.Split(it, ":")
			if len(parts) != 2 {
				return nil, errAt(off+len(parts[0])+1+len(parts[1]), "malformed T range %q", it) // the second ':'
			}
			if parts[0] == "*" {
				return nil, errAt(off, "T takes no %q", "*")
			}
			if parts[1] == "*" {
				return nil, errAt(off+len(parts[0])+1, "T takes no %q", "*")
			}
			endPos := off + len(parts[0]) + 1
			if err := expectTimeval(parts[0], off); err != nil {
				return nil, err
			}
			if err := expectTimeval(parts[1], endPos); err != nil {
				return nil, err
			}
			startMs, _ := parseTimeval(parts[0], false, off, note)
			endMs, _ := parseTimeval(parts[1], true, endPos, note)
			if startMs == endMs {
				note(errAt(endPos, "T range with equal endpoints covers nothing"))
			}
			ts.ranges = append(ts.ranges, msRange{start: startMs, end: endMs, wrap: startMs > endMs})
		} else {
			if it == "*" {
				return nil, errAt(off, "T takes no %q", "*")
			}
			if err := expectTimeval(it, off); err != nil {
				return nil, err
			}
			startMs, unit := parseTimeval(it, false, off, note)
			ts.ranges = append(ts.ranges, msRange{start: startMs, end: startMs + unit, wrap: false})
		}
		off += len(it) + 1 // the item plus its trailing ','
	}
	return ts, nil
}

// expectTimeval is the grammar error for a time value missing at at: an empty
// item or range end, or a '.' with no digits before it.
func expectTimeval(s string, at int) error {
	if s == "" || s[0] == '.' {
		return errAt(at, "expected a time value")
	}
	return nil
}

// parseTimeval parses HH | HHMM | HHMMSS | HHMMSS.sss and returns the start
// millisecond-of-day and the unit width (for single values). allowEnd permits
// the special whole-day end value 2400; at is the offset of s in the full
// source. A value the rules do not allow goes to note, positioned at the value,
// or at its '.' when the fraction is at fault: a fraction follows only six
// digits and is exactly three (spec §8).
func parseTimeval(s string, allowEnd bool, at int, note func(error)) (ms int, unit int) {
	head, frac, hasFrac := strings.Cut(s, ".")
	s = head
	if hasFrac && (len(s) != 6 || len(frac) != 3 || !allDigits(frac)) {
		note(errAt(at+len(s), "milliseconds follow only a 6-digit hhmmss time value"))
		frac = ""
	}
	if allowEnd && s == "2400" && frac == "" {
		return 86400000, 0
	}

	switch len(s) {
	case 2: // HH
		hh := digitsValue(s)
		if hh > 23 {
			note(errAt(at, "hour out of range in time value"))
		}
		return hh * 3600000, 3600000
	case 4: // HHMM
		hh, mm := digitsValue(s[0:2]), digitsValue(s[2:4])
		if hh > 23 {
			note(errAt(at, "hour out of range in time value"))
		}
		if mm > 59 {
			note(errAt(at, "minute out of range in time value"))
		}
		return hh*3600000 + mm*60000, 60000
	case 6: // HHMMSS[.sss]
		hh, mm, ssv := digitsValue(s[0:2]), digitsValue(s[2:4]), digitsValue(s[4:6])
		if hh > 23 {
			note(errAt(at, "hour out of range in time value"))
		}
		if mm > 59 {
			note(errAt(at, "minute out of range in time value"))
		}
		if ssv > 59 {
			note(errAt(at, "second out of range in time value"))
		}
		base := hh*3600000 + mm*60000 + ssv*1000
		if frac != "" {
			return base + digitsValue(frac), 1
		}
		return base, 1000
	}
	note(errAt(at, "malformed time value %q", s))
	return 0, 0
}

// allDigits reports whether every character of s is an ASCII digit.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
