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
	// an empty expression or branch is an error where a component was
	// required: past its spaces, at the '|' or the end of the input
	if strings.Trim(s, " ") == "" {
		return nil, errAt(len(s), "empty expression")
	}
	parts := strings.Split(s, "|")
	e := &Expression{source: s}
	off := 0
	for _, part := range parts {
		if strings.Trim(part, " ") == "" {
			return nil, errAt(off+len(part), "empty union branch")
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

// rawSelector is one selector as read. A D selector waits for the branch's
// scope (its domain depends on a co-present M/Q/Y), so only its value part is
// kept; every other selector is built as soon as it is read, into sel. desPos
// and vpPos are offsets into the full source of the designator letter and
// value part.
type rawSelector struct {
	des    designator
	vp     string
	desPos int
	vpPos  int
	sel    *selector
}

// parseBranch parses one union branch; base is the branch's offset into the
// full source, folded into every reported position. A rule error does not
// stop it: the scanner notes the earliest and reads on, and a grammar error
// ends the branch with the earlier of the two. Only D's domain waits for the
// complete branch (spec §8).
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
		var err error
		switch {
		case isDigit(c):
			err = sc.parseDateLeading(b)
		case c == '*':
			err = sc.parseBoundsStar(b)
		case isLetter(c):
			err = sc.parseSelector(b, &raws)
		case c == '!':
			err = errAt(sc.at(), "exclusion %q allowed only immediately after the designator", "!")
		default:
			err = errAt(sc.at(), "unexpected character %q", string(c))
		}
		if err != nil {
			return nil, sc.earliest(err)
		}
	}

	if len(raws) == 0 && b.tsel == nil && b.cadence == nil && b.bounds == nil {
		return nil, errAt(base, "empty expression")
	}
	b.hasW = b.present[desW]

	// The branch is complete: build D now that its scope is resolvable.
	scope := resolveScope(b)
	for _, r := range raws {
		if r.des == desD {
			selr, err := buildSelector(desD, scope, r.vp, r.vpPos)
			if err != nil {
				sc.note(err)
				continue
			}
			selr.pos = r.desPos
			r.sel = selr
		}
		if r.sel != nil {
			r.sel.scope = scope
			b.selectors = append(b.selectors, r.sel)
		}
	}
	if sc.pending != nil {
		return nil, sc.pending
	}
	return b, nil
}

// parseSelector reads one selector at the scanner. A selector other than D is
// built and checked at once; T is kept on the branch.
func (sc *scanner) parseSelector(b *branch, raws *[]rawSelector) error {
	desPos := sc.at()
	c := sc.peek()
	des := designator(c)
	if !isDesignator(des) {
		return errAt(desPos, "unknown designator %q", string(c))
	}
	if b.present[des] {
		sc.note(errAt(desPos, "duplicate designator %q in one expression", string(c)))
	}
	b.present[des] = true
	sc.next()
	vpPos := sc.at()
	if sc.eof() || !isValueChar(sc.peek()) {
		return errAt(vpPos, "designator %q requires a value", string(c))
	}
	if des == desT {
		vp := sc.readTimeRun()
		if vp == "" {
			return errAt(vpPos, "designator %q requires a value", string(c))
		}
		ts, err := buildTimeSelector(vp, vpPos, sc.note)
		if err != nil {
			return err
		}
		b.tsel = ts
		return nil
	}
	vp, err := sc.readSelectorValue(des)
	if err != nil {
		return err
	}
	r := rawSelector{des: des, vp: vp, desPos: desPos, vpPos: vpPos}
	if des != desD {
		// every domain but D's is fixed and already checked, so the build cannot
		// fail and the scope passed here is never read
		r.sel, _ = buildSelector(des, desM, vp, vpPos)
		r.sel.pos = desPos
	}
	*raws = append(*raws, r)
	return nil
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
	s       string
	pos     int
	base    int   // offset of s within the full source (union-branch start)
	pending error // the earliest rule error noted in the branch, if any
}

// note keeps err when it is the earliest rule error the branch has found.
func (sc *scanner) note(err error) {
	if sc.pending == nil || err.(ParseError).Pos < sc.pending.(ParseError).Pos {
		sc.pending = err
	}
}

// earliest is the branch's error at a grammar error err: the earlier of err
// and the rule errors noted before it (spec §8).
func (sc *scanner) earliest(err error) error {
	sc.note(err)
	return sc.pending
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

// readTimeRun consumes the T component's value part: the run of characters a
// time list can hold, or that buildTimeSelector rejects in place, up to
// whitespace, '|', a designator letter, '-' or end.
func (sc *scanner) readTimeRun() string {
	start := sc.pos
	for !sc.eof() && isValueChar(sc.peek()) && sc.peek() != '-' {
		sc.pos++
	}
	return sc.s[start:sc.pos]
}

// readSelectorValue consumes an integer selector's value part by the grammar
// (spec §8) and stops before the first character that cannot continue it,
// which the branch loop then reads as the next component or rejects. A token
// the grammar does not allow inside the value part is an error at that token.
// Each token is checked by the rules as it is read, and a rule error is noted;
// D's domain waits for the branch's scope, and buildSelector checks it.
func (sc *scanner) readSelectorValue(des designator) (string, error) {
	start := sc.pos
	exclude := sc.accept('!')
	var first item
	items := 0
	for {
		itemPos := sc.at()
		it, err := sc.readItem(des)
		if err != nil {
			return "", err
		}
		if items == 0 {
			first = it
		}
		items++
		bare := it.start.star && !it.isRange
		// a bare '*' is the whole domain on its own, never a list item or an
		// exclusion's value; '*:*' is a range and may stand in either (spec §3)
		if bare && (exclude || items > 1) {
			return "", errAt(itemPos, "bare %q is not a list item — write the designator with %q for the whole domain", "*", "*")
		}
		if sc.eof() || sc.peek() != ',' {
			break
		}
		if bare {
			return "", errAt(sc.at(), "a list cannot follow %q — it is already the whole domain", "*")
		}
		sc.next()
	}
	if sc.eof() {
		return sc.s[start:sc.pos], nil
	}
	switch sc.peek() {
	case '#':
		switch {
		case exclude:
			return "", errAt(sc.at(), "exclusion cannot carry a stride or ordinal")
		case items > 1 || first.isRange:
			return "", errAt(sc.at(), "an ordinal takes a single value, never a list or range")
		case des != desE:
			return "", errAt(sc.at(), "ordinal %q only valid on E", "#")
		case first.start.star:
			return "", errAt(sc.at(), "ordinal weekday must be a value") // 'E*' is whole; the '#' cannot follow
		}
		sc.next()
		if err := sc.readOrdinal(); err != nil {
			return "", err
		}
	case '/':
		switch {
		case exclude:
			return "", errAt(sc.at(), "exclusion cannot carry a stride or ordinal")
		case items > 1:
			return "", errAt(sc.at(), "stride not allowed on a list")
		}
		if err := sc.readStride(des, first); err != nil {
			return "", err
		}
	}
	return sc.s[start:sc.pos], nil
}

// item is one list item as read: a value, '*', or a range of the two (a
// single value is its own end).
type item struct {
	start, end endpoint
	isRange    bool
}

// readItem consumes one list item.
func (sc *scanner) readItem(des designator) (item, error) {
	start, err := sc.readEndpoint(des)
	if err != nil {
		return item{}, err
	}
	if !sc.accept(':') {
		return item{start: start, end: start}, nil
	}
	endPos := sc.at()
	end, err := sc.readEndpoint(des)
	if err != nil {
		return item{}, err
	}
	// Y has no edge to wrap around: a range end before its start is wrong at
	// the end (spec §3)
	if des == desY && !start.star && !end.star && start.val > end.val {
		sc.note(errAt(endPos, "backwards range on Y — no edge to wrap around"))
	}
	return item{start: start, end: end, isRange: true}, nil
}

// readEndpoint consumes '*' or a value, an optional '-' and an integer, and
// checks the value: '-0', the integer limit and, but for D, its domain.
func (sc *scanner) readEndpoint(des designator) (endpoint, error) {
	if sc.accept('*') {
		return endpoint{star: true}, nil
	}
	pos := sc.at()
	start := sc.pos
	what := "a value"
	if sc.accept('-') {
		what = "an integer after the sign"
	}
	if err := sc.readCount(what); err != nil {
		return endpoint{}, err
	}
	text := sc.s[start:sc.pos]
	v := sc.integer(text, pos)
	// the sign requires a nonzero integer: '-0' is not a value (spec §3)
	if v == 0 && text[0] == '-' {
		sc.note(errAt(pos, "'-0' is not a value"))
	}
	if des != desD {
		if err := checkDomain(des, domainSpec(des, desM), v, pos); err != nil {
			sc.note(err)
		}
	}
	return endpoint{val: v}, nil
}

// readOrdinal consumes and checks the count after an ordinal's '#'.
func (sc *scanner) readOrdinal() error {
	pos := sc.at()
	start := sc.pos
	sc.accept('-')
	if err := sc.readCount("an ordinal"); err != nil {
		return err
	}
	ord := sc.integer(sc.s[start:sc.pos], pos)
	if ord == 0 {
		sc.note(errAt(pos, "ordinal zero"))
	}
	if ord < -5 || ord > 5 {
		sc.note(errAt(pos, "ordinal %d out of range (-5..-1, 1..5)", ord))
	}
	return nil
}

// readStride consumes and checks a stride after its first item: the '/', the
// interval, and the duration when one follows. A stride its start or range
// does not allow is an error at the '/'.
func (sc *scanner) readStride(des designator, first item) error {
	slash := sc.at()
	sc.next()
	switch {
	case first.start.star:
		sc.note(errAt(slash, "anchorless stride — an explicit start is required"))
	case first.start.val < 0:
		sc.note(errAt(slash, "stride start must be non-negative"))
	case first.isRange && des != desY && !first.end.star && first.end.val >= 0 && first.start.val > first.end.val:
		// Y's backwards range is already an error, at its end
		sc.note(errAt(slash, "wrap ranges take no stride"))
	}
	ivalPos := sc.at()
	interval, err := sc.readCountValue("a stride interval")
	if err != nil {
		return err
	}
	if interval < 2 {
		sc.note(errAt(ivalPos, "stride interval must be >= 2"))
	}
	// Y has no parent domain, and D's waits for its scope
	if des != desY && des != desD && interval > domainSpec(des, desM).size {
		sc.note(errAt(ivalPos, "stride interval %d exceeds parent domain — use a cadence", interval))
	}
	if !sc.accept('/') {
		return nil
	}
	durPos := sc.at()
	duration, err := sc.readCountValue("a stride duration")
	if err != nil {
		return err
	}
	if duration < 1 || duration >= interval {
		sc.note(errAt(durPos, "stride duration must satisfy 1 <= duration < interval"))
	}
	return nil
}

// readCountValue consumes an unsigned integer and returns its value.
func (sc *scanner) readCountValue(what string) (int, error) {
	pos := sc.at()
	start := sc.pos
	if err := sc.readCount(what); err != nil {
		return 0, err
	}
	return sc.integer(sc.s[start:sc.pos], pos), nil
}

// integer is the value of the integer token text at pos; one above
// maxInteger is noted there.
func (sc *scanner) integer(text string, pos int) int {
	v, over := parseInteger(text)
	if over {
		sc.note(errAt(pos, "integer above %d", maxInteger))
	}
	return v
}

// readCount consumes an integer's digits; none is an error at the character
// (or the end) where the integer was required, what naming it.
func (sc *scanner) readCount(what string) error {
	if sc.eof() || !isDigit(sc.peek()) {
		return errAt(sc.at(), "expected %s", what)
	}
	for !sc.eof() && isDigit(sc.peek()) {
		sc.pos++
	}
	return nil
}

// accept consumes c when it is the next character.
func (sc *scanner) accept(c byte) bool {
	if sc.eof() || sc.peek() != c {
		return false
	}
	sc.pos++
	return true
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

// parseInteger reads s, an optional leading '-' and a run of ASCII digits the
// scanner has already read, as an integer; over is true when the integer's
// value exceeds maxInteger. Leading zeros are unlimited.
func parseInteger(s string) (v int, over bool) {
	digits := strings.TrimPrefix(s, "-")
	var n int64
	for i := 0; i < len(digits); i++ {
		n = n*10 + int64(digits[i]-'0')
		if n > maxInteger {
			return 0, true
		}
	}
	if len(digits) < len(s) {
		n = -n
	}
	return int(n), false
}

// --- date-leading components (cadence or bounds) ---------------------------

// parseDateLit reads a date literal at the scanner, which must start with a
// digit. A literal the rules do not allow is noted and read past, its digit
// runs and T part whole, so the branch reads on (spec §8).
func (sc *scanner) parseDateLit() dateLit {
	start := sc.at()
	digits := sc.readRun()
	if len(digits) != 8 {
		sc.note(errAt(start, "invalid date literal (exactly 8 digits)"))
		digits = "00010101"
	}
	lit := dateLit{year: digitsValue(digits[0:4]), month: digitsValue(digits[4:6]), day: digitsValue(digits[6:8]), prec: precDay}

	// Unconditional T-glue: a T after 8 digits belongs to the literal. Every
	// error in the literal, its time part included, is positioned at its start.
	if !sc.eof() && sc.peek() == 'T' {
		sc.next()
		hms := sc.readRun()
		// the time part is exactly 4 or 6 digits: a further digit makes it
		// malformed, never the start of a second literal
		if len(hms) != 4 && len(hms) != 6 {
			sc.note(errAt(start, "malformed time-part in date literal"))
			hms = "0000"
		}
		lit.hour, lit.minute = digitsValue(hms[0:2]), digitsValue(hms[2:4])
		lit.prec = precMinute
		if len(hms) == 6 {
			lit.second = digitsValue(hms[4:6])
			lit.prec = precSecond
		}
	}
	if !validCivilDate(lit.year, lit.month, lit.day) {
		sc.note(errAt(start, "%04d%02d%02d is not a real calendar date", lit.year, lit.month, lit.day))
	}
	if lit.hour > 23 || lit.minute > 59 || lit.second > 59 {
		sc.note(errAt(start, "invalid time in date literal"))
	}
	return lit
}

// readRun consumes a run of ASCII digits, possibly empty.
func (sc *scanner) readRun() string {
	start := sc.pos
	for !sc.eof() && isDigit(sc.peek()) {
		sc.pos++
	}
	return sc.s[start:sc.pos]
}

func (sc *scanner) parseDateLeading(b *branch) error {
	start := sc.at()
	d1 := sc.parseDateLit()
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
			sc.note(errAt(start, "more than one bounds component per expression"))
		}
		b.bounds = &bounds{hasLower: true, lower: d1, hasUpper: true, upper: d1}
		return nil
	}
}

// parseBoundsUpper parses the upper endpoint after "lower:"; start is the
// offset of the lower endpoint (the bounds component's first character).
func (sc *scanner) parseBoundsUpper(b *branch, lower dateLit, start int) error {
	if b.bounds != nil {
		sc.note(errAt(start, "more than one bounds component per expression"))
	}
	bd := &bounds{hasLower: true, lower: lower}
	if !sc.eof() && sc.peek() == '*' {
		sc.next()
		// lower:* -> open upper
	} else {
		upperStart := sc.at()
		if sc.eof() || !isDigit(sc.peek()) {
			return errAt(upperStart, "invalid date literal (need 8 digits)")
		}
		upper := sc.parseDateLit()
		// backwards when the first span begins at or after the last span's
		// end; mixed precision is valid in either order (spec §6)
		if !spanStart(lower).Before(spanEnd(upper)) {
			sc.note(errAt(upperStart, "backwards bounds range"))
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
	if sc.eof() || !isDigit(sc.peek()) {
		return errAt(sc.at(), "invalid date literal (need 8 digits)")
	}
	upper := sc.parseDateLit()
	if b.bounds != nil {
		sc.note(errAt(start, "more than one bounds component per expression"))
	}
	b.bounds = &bounds{hasUpper: true, upper: upper}
	return nil
}

var cadenceUnits = map[byte]bool{'Y': true, 'M': true, 'W': true, 'D': true, 'H': true, 'm': true}

// parseCadence parses "period[/duration]" after the anchor's '/'; start is the
// offset of the anchor (the cadence component's first character).
func (sc *scanner) parseCadence(b *branch, anchor dateLit, start int) error {
	if b.cadence != nil {
		sc.note(errAt(start, "more than one cadence per expression"))
	}
	ppos := sc.at()
	period, punit, err := sc.readCadencePart()
	if err != nil {
		return err
	}
	c := &cadence{anchor: anchor, period: period, periodUnit: punit}
	if period == 0 {
		sc.note(errAt(ppos, "zero cadence period"))
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
		sc.note(err)
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
	n, over := parseInteger(sc.s[start:sc.pos])
	if over {
		sc.note(errAt(sc.base+start, "integer above %d", maxInteger))
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
