package dtrexp

// designator is the leading letter of a selector (case-sensitive: 'M' month vs
// 'm' minute). The zero value means "none" (a date-literal component).
type designator byte

const (
	desY   designator = 'Y'
	desQ   designator = 'Q'
	desM   designator = 'M'
	desW   designator = 'W'
	desD   designator = 'D'
	desE   designator = 'E'
	desT   designator = 'T'
	desH   designator = 'H'
	desMin designator = 'm'
	desS   designator = 's'
)

// branch is one union member: components intersected.
type branch struct {
	selectors []*selector   // integer-field selectors (Y,Q,M,W,D,E,H,m,s)
	tsel      *timeSelector // T selector, if present
	cadence   *cadence      // at most one
	bounds    *bounds       // at most one
	hasW      bool          // W present -> Y tests the ISO week-year
	present   map[designator]bool
}

// selector is an integer-field selector (everything but T, cadence, bounds).
type selector struct {
	des     designator
	scope   designator // resolved M/Q/Y scope for D and ordinal E; else 0
	pos     int        // offset of the designator in the full source (for warnings)
	exclude bool
	terms   []term
}

type termKind int

const (
	termSingle termKind = iota
	termRange
	termStride
	termOrdinal
)

// endpoint is one side of a range/value. star means '*' (domain edge); otherwise
// val is the raw integer (possibly negative).
type endpoint struct {
	star bool
	val  int
}

type term struct {
	kind         termKind
	start        endpoint // single value uses start
	end          endpoint
	wrapEligible bool // both endpoints literal, non-negative (only such ranges wrap)
	interval     int  // stride
	duration     int  // stride
	ordinal      int  // E ordinal, signed non-zero
}

// timeSelector is the T component: a set of half-open millisecond-of-day ranges.
type timeSelector struct {
	ranges []msRange
}

type msRange struct {
	start int // inclusive, ms of day
	end   int // exclusive, ms of day (may be < start -> wraps midnight)
	wrap  bool
}

// cadence is an anchored recurrence: from anchor, every period, covering
// duration each occurrence.
type cadence struct {
	anchor       dateLit
	period       int
	periodUnit   byte // 'Y','M','W','D','H','m'
	duration     int
	durationUnit byte
}

// bounds is an absolute clipping window. A missing lower/upper endpoint means
// unbounded on that side.
type bounds struct {
	hasLower bool
	lower    dateLit
	hasUpper bool
	upper    dateLit
}

// datePrecision records the span a bare date literal denotes.
type datePrecision int

const (
	precDay    datePrecision = iota // 8-digit date -> whole day
	precMinute                      // ...Thhmm    -> whole minute
	precSecond                      // ...Thhmmss  -> whole second
)

type dateLit struct {
	year, month, day     int
	hour, minute, second int
	prec                 datePrecision
}
