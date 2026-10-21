package dtrexp

import (
	"fmt"
	"strings"
)

// errAt builds a positioned ParseError; every parse-failure site funnels
// through it so callers can always recover the character offset.
func errAt(pos int, format string, args ...any) error {
	return ParseError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// parse is the entry point: split top-level union branches, parse each, then
// run static warning analysis. Branch offsets into the full source are carried
// down so every error and warning is positioned against the original string.
func parse(s string) (*Expression, error) {
	if strings.Trim(s, " ") == "" {
		return nil, errAt(0, "empty expression")
	}
	parts := strings.Split(s, "|")
	e := &Expression{source: s}
	off := 0
	for _, part := range parts {
		if strings.Trim(part, " ") == "" {
			return nil, errAt(off, "empty union branch")
		}
		b, err := parseBranch(part, off)
		if err != nil {
			return nil, err
		}
		e.branches = append(e.branches, b)
		off += len(part) + 1 // the branch text plus its trailing '|'
	}
	e.warnings = analyzeWarnings(e)
	return e, nil
}

// rawSelector defers value-building until the branch's scoping designators are
// known (D and ordinal-E scope depend on co-present M/Q/Y). desPos and vpPos
// are offsets into the full source of the designator letter and value part.
type rawSelector struct {
	des    designator
	vp     string
	desPos int
	vpPos  int
}

// parseBranch parses one union branch; base is the branch's offset into the
// full source, folded into every reported position.
func parseBranch(s string, base int) (*branch, error) {
	sc := &scanner{s: s, base: base}
	b := &branch{present: map[designator]bool{}}
	var raws []rawSelector

	for {
		sc.skipSpace()
		if sc.eof() {
			break
		}
		c := sc.peek()
		switch {
		case isDigit(c):
			if err := sc.parseDateLeading(b); err != nil {
				return nil, err
			}
		case c == '*':
			if err := sc.parseBoundsStar(b); err != nil {
				return nil, err
			}
		case isLetter(c):
			desPos := sc.at()
			des := designator(c)
			if !isDesignator(des) {
				return nil, errAt(desPos, "unknown designator %q", string(c))
			}
			sc.next()
			vpPos := sc.at()
			vp := sc.readValueRun()
			if vp == "" {
				return nil, errAt(vpPos, "designator %q requires a value", string(c))
			}
			if b.present[des] {
				return nil, errAt(desPos, "duplicate designator %q in one expression", string(c))
			}
			b.present[des] = true
			raws = append(raws, rawSelector{des: des, vp: vp, desPos: desPos, vpPos: vpPos})
		default:
			return nil, errAt(sc.at(), "unexpected character %q", string(c))
		}
	}

	if len(raws) == 0 && b.tsel == nil && b.cadence == nil && b.bounds == nil {
		return nil, errAt(base, "empty expression")
	}
	b.hasW = b.present[desW]

	// Phase 2: build selectors now that scope is resolvable.
	for _, r := range raws {
		if r.des == desT {
			ts, err := buildTimeSelector(r.vp, r.vpPos)
			if err != nil {
				return nil, err
			}
			b.tsel = ts
			continue
		}
		scope := resolveScope(b)
		selr, err := buildSelector(r.des, scope, r.vp, r.vpPos)
		if err != nil {
			return nil, err
		}
		selr.pos = r.desPos
		b.selectors = append(b.selectors, selr)
	}
	return b, nil
}

// resolveScope returns the scope for a D or ordinal-E selector: the co-present
// M, else Q, else Y, else month (default).
func resolveScope(b *branch) designator {
	switch {
	case b.present[desM]:
		return desM
	case b.present[desQ]:
		return desQ
	case b.present[desY]:
		return desY
	default:
		return desM
	}
}

// --- scanner ---------------------------------------------------------------

type scanner struct {
	s    string
	pos  int
	base int // offset of s within the full source (union-branch start)
}

func (sc *scanner) eof() bool  { return sc.pos >= len(sc.s) }
func (sc *scanner) peek() byte { return sc.s[sc.pos] }
func (sc *scanner) next() byte { c := sc.s[sc.pos]; sc.pos++; return c }

// at returns the current position as an offset into the full source.
func (sc *scanner) at() int { return sc.base + sc.pos }

func (sc *scanner) skipSpace() {
	for !sc.eof() && isSpace(sc.peek()) {
		sc.pos++
	}
}

// readValueRun consumes a selector's value part: the run of value characters up
// to whitespace, '|', a designator letter, or end.
func (sc *scanner) readValueRun() string {
	start := sc.pos
	for !sc.eof() && isValueChar(sc.peek()) {
		sc.pos++
	}
	return sc.s[start:sc.pos]
}

// digitsValue returns the value of a short run of ASCII digits the scanner has
// already checked (a date literal's or time value's fixed-width fields).
func digitsValue(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func (sc *scanner) readDigits(n int) (string, bool) {
	if sc.pos+n > len(sc.s) {
		return "", false
	}
	for i := 0; i < n; i++ {
		if !isDigit(sc.s[sc.pos+i]) {
			return "", false
		}
	}
	out := sc.s[sc.pos : sc.pos+n]
	sc.pos += n
	return out, true
}

// --- character classes -----------------------------------------------------

func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isSpace(c byte) bool  { return c == ' ' } // spec §1: the ASCII space only
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isDesignator(d designator) bool {
	switch d {
	case desY, desQ, desM, desW, desD, desE, desT, desH, desMin, desS:
		return true
	}
	return false
}

func isValueChar(c byte) bool {
	if isDigit(c) {
		return true
	}
	switch c {
	case ',', ':', '*', '!', '/', '#', '.', '-':
		return true
	}
	return false
}

// maxInteger is the largest value an integer may have (spec §8): 2³¹ − 1,
// however many leading zeros it carries.
const maxInteger = 1<<31 - 1

// parseInteger reads s as an integer with an optional leading '-'. ok is false
// when s is not a sign and a run of ASCII digits; over is true when the
// integer's value exceeds maxInteger. Leading zeros are unlimited.
func parseInteger(s string) (v int, ok, over bool) {
	digits := strings.TrimPrefix(s, "-")
	if !allDigits(digits) {
		return 0, false, false
	}
	var n int64
	for i := 0; i < len(digits); i++ {
		n = n*10 + int64(digits[i]-'0')
		if n > maxInteger {
			return 0, true, true
		}
	}
	if len(digits) < len(s) {
		n = -n
	}
	return int(n), true, false
}

// --- date-leading components (cadence or bounds) ---------------------------

func (sc *scanner) parseDateLit() (dateLit, error) {
	start := sc.at()
	digits, ok := sc.readDigits(8)
	if !ok {
		return dateLit{}, errAt(start, "invalid date literal (need 8 digits)")
	}
	lit := dateLit{year: digitsValue(digits[0:4]), month: digitsValue(digits[4:6]), day: digitsValue(digits[6:8]), prec: precDay}

	// Unconditional T-glue: a T after 8 digits belongs to the literal. Every
	// error in the literal, its time part included, is positioned at its start.
	if !sc.eof() && sc.peek() == 'T' {
		sc.next()
		hhmm, ok := sc.readDigits(4)
		if !ok {
			return dateLit{}, errAt(start, "malformed time-part in date literal")
		}
		lit.hour, lit.minute = digitsValue(hhmm[0:2]), digitsValue(hhmm[2:4])
		lit.prec = precMinute
		if ss, ok := sc.readDigits(2); ok {
			lit.second = digitsValue(ss)
			lit.prec = precSecond
		}
	}
	// a date literal is exactly 8 digits (and its time part 4 or 6): a further
	// digit makes the whole run malformed, never a second literal
	if !sc.eof() && isDigit(sc.peek()) {
		return dateLit{}, errAt(start, "malformed date literal: too many digits")
	}
	if !validCivilDate(lit.year, lit.month, lit.day) {
		return dateLit{}, errAt(start, "%04d%02d%02d is not a real calendar date", lit.year, lit.month, lit.day)
	}
	if lit.hour > 23 || lit.minute > 59 || lit.second > 59 {
		return dateLit{}, errAt(start, "invalid time in date literal")
	}
	return lit, nil
}

func (sc *scanner) parseDateLeading(b *branch) error {
	start := sc.at()
	d1, err := sc.parseDateLit()
	if err != nil {
		return err
	}
	switch {
	case !sc.eof() && sc.peek() == '/':
		sc.next()
		return sc.parseCadence(b, d1, start)
	case !sc.eof() && sc.peek() == ':':
		sc.next()
		return sc.parseBoundsUpper(b, d1, start)
	default:
		// Single-date bounds: the whole span of the literal.
		if b.bounds != nil {
			return errAt(start, "more than one bounds component per expression")
		}
		b.bounds = &bounds{hasLower: true, lower: d1, hasUpper: true, upper: d1}
		return nil
	}
}

// parseBoundsUpper parses the upper endpoint after "lower:"; start is the
// offset of the lower endpoint (the bounds component's first character).
func (sc *scanner) parseBoundsUpper(b *branch, lower dateLit, start int) error {
	if b.bounds != nil {
		return errAt(start, "more than one bounds component per expression")
	}
	bd := &bounds{hasLower: true, lower: lower}
	if !sc.eof() && sc.peek() == '*' {
		sc.next()
		// lower:* -> open upper
	} else {
		upperStart := sc.at()
		upper, err := sc.parseDateLit()
		if err != nil {
			return err
		}
		// backwards when the first span begins at or after the last span's
		// end; mixed precision is valid in either order (spec §6)
		if !spanStart(lower).Before(spanEnd(upper)) {
			return errAt(upperStart, "backwards bounds range")
		}
		bd.hasUpper = true
		bd.upper = upper
	}
	b.bounds = bd
	return nil
}

func (sc *scanner) parseBoundsStar(b *branch) error {
	start := sc.at()
	sc.next() // '*'
	if sc.eof() || sc.peek() != ':' {
		return errAt(sc.at(), "expected ':' after a bounds %q", "*")
	}
	sc.next() // ':'
	if !sc.eof() && sc.peek() == '*' {
		return errAt(sc.at(), "bounds require at least one date-literal endpoint")
	}
	upper, err := sc.parseDateLit()
	if err != nil {
		return err
	}
	if b.bounds != nil {
		return errAt(start, "more than one bounds component per expression")
	}
	b.bounds = &bounds{hasUpper: true, upper: upper}
	return nil
}

var cadenceUnits = map[byte]bool{'Y': true, 'M': true, 'W': true, 'D': true, 'H': true, 'm': true}

// parseCadence parses "period[/duration]" after the anchor's '/'; start is the
// offset of the anchor (the cadence component's first character).
func (sc *scanner) parseCadence(b *branch, anchor dateLit, start int) error {
	if b.cadence != nil {
		return errAt(start, "more than one cadence per expression")
	}
	ppos := sc.at()
	period, punit, err := sc.readCadencePart()
	if err != nil {
		return err
	}
	c := &cadence{anchor: anchor, period: period, periodUnit: punit}
	if period == 0 {
		return errAt(ppos, "zero cadence period")
	}
	dpos := ppos
	if !sc.eof() && sc.peek() == '/' {
		sc.next()
		dpos = sc.at()
		dur, dunit, err := sc.readCadencePart()
		if err != nil {
			return err
		}
		c.duration = dur
		c.durationUnit = dunit
	} else {
		// Default duration: one of the period's unit.
		c.duration = 1
		c.durationUnit = punit
	}
	if err := validateCadence(c, dpos); err != nil {
		return err
	}
	b.cadence = c
	return nil
}

func (sc *scanner) readCadencePart() (int, byte, error) {
	start := sc.pos
	for !sc.eof() && isDigit(sc.peek()) {
		sc.pos++
	}
	if sc.pos == start {
		return 0, 0, errAt(sc.at(), "cadence missing count")
	}
	n, _, over := parseInteger(sc.s[start:sc.pos])
	if over {
		return 0, 0, errAt(sc.base+start, "integer above %d", maxInteger)
	}
	if sc.eof() {
		return 0, 0, errAt(sc.at(), "cadence missing unit")
	}
	upos := sc.at()
	u := sc.next()
	if !cadenceUnits[u] {
		return 0, 0, errAt(upos, "unknown cadence unit %q", string(u))
	}
	return n, u, nil
}

// unit lengths in seconds for the conservative duration<period check (§5.2).
func unitMaxSeconds(u byte) int64 {
	switch u {
	case 'Y':
		return 366 * 86400
	case 'M':
		return 31 * 86400
	case 'W':
		return 7 * 86400
	case 'D':
		return 86400
	case 'H':
		return 3600
	case 'm':
		return 60
	}
	return 0
}

func unitMinSeconds(u byte) int64 {
	switch u {
	case 'Y':
		return 365 * 86400
	case 'M':
		return 28 * 86400
	case 'W':
		return 7 * 86400
	case 'D':
		return 86400
	case 'H':
		return 3600
	case 'm':
		return 60
	}
	return 0
}

// validateCadence checks the static duration/period rules; pos is the offset
// of the duration part (or the period part when the duration is defaulted).
func validateCadence(c *cadence, pos int) error {
	// Month/year duration unit requires a month/year period.
	if (c.durationUnit == 'M' || c.durationUnit == 'Y') &&
		!(c.periodUnit == 'M' || c.periodUnit == 'Y') {
		return errAt(pos, "month/year duration unit requires a month/year period")
	}
	if c.duration < 1 {
		return errAt(pos, "cadence duration must be >= 1")
	}
	// duration < period, conservatively with fixed unit lengths.
	if c.durationUnit == c.periodUnit {
		if c.duration >= c.period {
			return errAt(pos, "cadence duration must be < period")
		}
		return nil
	}
	// int64: a count may reach maxInteger, and a year of seconds times that
	// overflows a 32-bit int
	if int64(c.duration)*unitMaxSeconds(c.durationUnit) >= int64(c.period)*unitMinSeconds(c.periodUnit) {
		return errAt(pos, "cadence duration not conservatively smaller than period")
	}
	return nil
}
