package dtrexp

import (
	"strconv"
	"strings"
)

type domSpec struct {
	minV, maxV, size int
	unbounded        bool
}

func domainSpec(des, scope designator) domSpec {
	switch des {
	case desY:
		return domSpec{unbounded: true}
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

// buildSelector builds an integer-field selector from its value part vp; at is
// the offset of vp in the full source, from which every error position within
// the value part is derived.
func buildSelector(des, scope designator, vp string, at int) (*selector, error) {
	spec := domainSpec(des, scope)
	s := &selector{des: des, scope: scope}

	// Bare '*' -> whole domain.
	if vp == "*" {
		s.terms = []term{{kind: termRange, start: endpoint{star: true}, end: endpoint{star: true}}}
		return s, nil
	}

	// Exclusion: '!' only immediately after the designator.
	if idx := strings.IndexByte(vp, '!'); idx >= 0 {
		if idx != 0 {
			return nil, errAt(at+idx, "exclusion %q allowed only immediately after the designator", "!")
		}
		body := vp[1:]
		if bad := strings.IndexAny(body, "/#"); bad >= 0 {
			return nil, errAt(at+1+bad, "exclusion cannot carry a stride or ordinal")
		}
		terms, err := parseValueList(des, spec, body, at+1)
		if err != nil {
			return nil, err
		}
		s.exclude = true
		s.terms = terms
		return s, nil
	}

	// Ordinal (E only).
	if strings.ContainsRune(vp, '#') {
		t, err := parseOrdinal(des, spec, vp, at)
		if err != nil {
			return nil, err
		}
		s.terms = []term{t}
		return s, nil
	}

	// Stride.
	if idx := strings.IndexByte(vp, '/'); idx >= 0 {
		if strings.ContainsRune(vp, ',') {
			return nil, errAt(at+idx, "stride not allowed on a list")
		}
		t, err := parseStride(des, scope, spec, vp, at)
		if err != nil {
			return nil, err
		}
		s.terms = []term{t}
		return s, nil
	}

	terms, err := parseValueList(des, spec, vp, at)
	if err != nil {
		return nil, err
	}
	s.terms = terms
	return s, nil
}

// parseValueList parses a comma-separated value list; at is the offset of s in
// the full source.
func parseValueList(des designator, spec domSpec, s string, at int) ([]term, error) {
	items := strings.Split(s, ",")
	multi := len(items) > 1
	var terms []term
	off := at
	for _, it := range items {
		if multi && it == "*" {
			return nil, errAt(off, "bare %q in a list — the list is already the whole domain", "*")
		}
		t, err := parseItem(des, spec, it, off)
		if err != nil {
			return nil, err
		}
		terms = append(terms, t)
		off += len(it) + 1 // the item plus its trailing ','
	}
	return terms, nil
}

func parseItem(des designator, spec domSpec, it string, at int) (term, error) {
	if it == "" {
		return term{}, errAt(at, "empty value item")
	}
	if it == "*" {
		return term{kind: termRange, start: endpoint{star: true}, end: endpoint{star: true}}, nil
	}
	if strings.ContainsRune(it, ':') {
		return parseRange(des, spec, it, at)
	}
	ep, err := parseEndpoint(des, spec, it, true, at)
	if err != nil {
		return term{}, err
	}
	return term{kind: termSingle, start: ep}, nil
}

func parseRange(des designator, spec domSpec, it string, at int) (term, error) {
	parts := strings.Split(it, ":")
	if len(parts) != 2 {
		return term{}, errAt(at, "malformed range %q", it)
	}
	start, err := parseEndpoint(des, spec, parts[0], true, at)
	if err != nil {
		return term{}, err
	}
	end, err := parseEndpoint(des, spec, parts[1], false, at+len(parts[0])+1)
	if err != nil {
		return term{}, err
	}
	wrapEligible := !start.star && !end.star && start.val >= 0 && end.val >= 0
	// Y has no edge to wrap around: a backwards literal range is an error.
	if spec.unbounded && wrapEligible && start.val > end.val {
		return term{}, errAt(at, "backwards range on Y — no edge to wrap around")
	}
	return term{kind: termRange, start: start, end: end, wrapEligible: wrapEligible}, nil
}

// parseEndpoint parses a single value/'*' with domain validation; at is the
// offset of s in the full source.
func parseEndpoint(des designator, spec domSpec, s string, atStart bool, at int) (endpoint, error) {
	if s == "*" {
		return endpoint{star: true}, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return endpoint{}, errAt(at, "invalid value %q", s)
	}
	// the sign requires a nonzero integer: '-0' is not a value (spec §3)
	if v == 0 && strings.HasPrefix(s, "-") {
		return endpoint{}, errAt(at, "'-0' is not a value")
	}
	if spec.unbounded { // Y
		if v < 0 {
			return endpoint{}, errAt(at, "negative value on Y — no edge to count back from")
		}
		// Y takes 4-digit ISO years (spec §2): 1-9999.
		if v < 1 || v > 9999 {
			return endpoint{}, errAt(at, "year %d out of domain (1-9999)", v)
		}
		return endpoint{val: v}, nil
	}
	if v < 0 {
		if v < -spec.size {
			return endpoint{}, errAt(at, "negative value %d out of domain for %q", v, string(des))
		}
		return endpoint{val: v}, nil
	}
	if v < spec.minV || v > spec.maxV {
		return endpoint{}, errAt(at, "value %d out of domain for %q", v, string(des))
	}
	return endpoint{val: v}, nil
}

func parseOrdinal(des designator, spec domSpec, vp string, at int) (term, error) {
	if des != desE {
		return term{}, errAt(at+strings.IndexByte(vp, '#'), "ordinal %q only valid on E", "#")
	}
	parts := strings.Split(vp, "#")
	if len(parts) != 2 {
		return term{}, errAt(at, "malformed ordinal %q", vp)
	}
	base, err := parseEndpoint(des, spec, parts[0], true, at)
	if err != nil {
		return term{}, err
	}
	if base.star {
		return term{}, errAt(at, "ordinal weekday must be a value")
	}
	ordPos := at + len(parts[0]) + 1
	ord, err := strconv.Atoi(parts[1])
	if err != nil {
		return term{}, errAt(ordPos, "invalid ordinal %q", parts[1])
	}
	if ord == 0 {
		return term{}, errAt(ordPos, "ordinal zero")
	}
	if ord < -5 || ord > 5 {
		return term{}, errAt(ordPos, "ordinal %d out of range (-5..-1, 1..5)", ord)
	}
	return term{kind: termOrdinal, start: base, ordinal: ord}, nil
}

func parseStride(des, scope designator, spec domSpec, vp string, at int) (term, error) {
	parts := strings.Split(vp, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return term{}, errAt(at, "malformed stride %q", vp)
	}
	base := parts[0]

	var start, end endpoint
	if strings.ContainsRune(base, ':') {
		rp := strings.Split(base, ":")
		if len(rp) != 2 {
			return term{}, errAt(at, "malformed stride range %q", base)
		}
		s0, err := parseEndpoint(des, spec, rp[0], true, at)
		if err != nil {
			return term{}, err
		}
		e0, err := parseEndpoint(des, spec, rp[1], false, at+len(rp[0])+1)
		if err != nil {
			return term{}, err
		}
		start, end = s0, e0
		if !start.star && !end.star && start.val >= 0 && end.val >= 0 && start.val > end.val {
			return term{}, errAt(at, "wrap ranges take no stride")
		}
	} else {
		s0, err := parseEndpoint(des, spec, base, true, at)
		if err != nil {
			return term{}, err
		}
		start = s0
		end = endpoint{star: true} // default end: domain edge
	}

	// The stride anchor (start) must be an explicit non-negative value.
	if start.star {
		return term{}, errAt(at, "anchorless stride — an explicit start is required")
	}
	if start.val < 0 {
		return term{}, errAt(at, "stride start must be non-negative")
	}

	ivalPos := at + len(parts[0]) + 1
	interval, err := strconv.Atoi(parts[1])
	if err != nil {
		return term{}, errAt(ivalPos, "invalid stride interval %q", parts[1])
	}
	if interval < 2 {
		return term{}, errAt(ivalPos, "stride interval must be >= 2")
	}
	if !spec.unbounded && interval > spec.size {
		return term{}, errAt(ivalPos, "stride interval %d exceeds parent domain — use a cadence", interval)
	}

	duration := 1
	if len(parts) == 3 {
		durPos := ivalPos + len(parts[1]) + 1
		duration, err = strconv.Atoi(parts[2])
		if err != nil {
			return term{}, errAt(durPos, "invalid stride duration %q", parts[2])
		}
		if duration < 1 || duration >= interval {
			return term{}, errAt(durPos, "stride duration must satisfy 1 <= duration < interval")
		}
	}
	return term{kind: termStride, start: start, end: end, interval: interval, duration: duration}, nil
}

// --- Time selector ---------------------------------------------------------

// buildTimeSelector builds the T component from its value part vp; at is the
// offset of vp in the full source.
func buildTimeSelector(vp string, at int) (*timeSelector, error) {
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
				return nil, errAt(off, "malformed T range %q", it)
			}
			if parts[0] == "*" {
				return nil, errAt(off, "T takes no %q", "*")
			}
			if parts[1] == "*" {
				return nil, errAt(off+len(parts[0])+1, "T takes no %q", "*")
			}
			startMs, _, err := parseTimeval(parts[0], false, off)
			if err != nil {
				return nil, err
			}
			endMs, _, err := parseTimeval(parts[1], true, off+len(parts[0])+1)
			if err != nil {
				return nil, err
			}
			if startMs == endMs {
				return nil, errAt(off, "T range with equal endpoints covers nothing")
			}
			ts.ranges = append(ts.ranges, msRange{start: startMs, end: endMs, wrap: startMs > endMs})
		} else {
			if it == "*" {
				return nil, errAt(off, "T takes no %q", "*")
			}
			startMs, unit, err := parseTimeval(it, false, off)
			if err != nil {
				return nil, err
			}
			ts.ranges = append(ts.ranges, msRange{start: startMs, end: startMs + unit, wrap: false})
		}
		off += len(it) + 1 // the item plus its trailing ','
	}
	return ts, nil
}

// parseTimeval parses HH | HHMM | HHMMSS | HHMMSS.sss and returns the start
// millisecond-of-day and the unit width (for single values). allowEnd permits
// the special whole-day end value 2400; at is the offset of s in the full
// source.
func parseTimeval(s string, allowEnd bool, at int) (ms int, unit int, err error) {
	frac := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		frac = s[dot+1:]
		s = s[:dot]
		if len(frac) != 3 || !allDigits(frac) {
			return 0, 0, errAt(at+dot+1, "malformed millisecond in time value")
		}
	}
	if !allDigits(s) {
		return 0, 0, errAt(at, "malformed time value")
	}

	if allowEnd && s == "2400" && frac == "" {
		return 86400000, 0, nil
	}

	switch len(s) {
	case 2: // HH
		hh, _ := strconv.Atoi(s)
		if hh > 23 {
			return 0, 0, errAt(at, "hour out of range in time value")
		}
		return hh * 3600000, 3600000, nil
	case 4: // HHMM
		hh, _ := strconv.Atoi(s[0:2])
		mm, _ := strconv.Atoi(s[2:4])
		if hh > 23 {
			return 0, 0, errAt(at, "hour out of range in time value")
		}
		if mm > 59 {
			return 0, 0, errAt(at, "minute out of range in time value")
		}
		return hh*3600000 + mm*60000, 60000, nil
	case 6: // HHMMSS[.sss]
		hh, _ := strconv.Atoi(s[0:2])
		mm, _ := strconv.Atoi(s[2:4])
		ssv, _ := strconv.Atoi(s[4:6])
		if hh > 23 {
			return 0, 0, errAt(at, "hour out of range in time value")
		}
		if mm > 59 {
			return 0, 0, errAt(at, "minute out of range in time value")
		}
		if ssv > 59 {
			return 0, 0, errAt(at, "second out of range in time value")
		}
		base := hh*3600000 + mm*60000 + ssv*1000
		if frac != "" {
			f, _ := strconv.Atoi(frac)
			return base + f, 1, nil
		}
		return base, 1000, nil
	}
	return 0, 0, errAt(at, "malformed time value %q", s)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
