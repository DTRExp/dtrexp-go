// Package dtrexp parses and evaluates DTRExp (Date-Time Range & Recurrence
// Expression) strings, per DTRExp draft 2.8.
//
// A DTRExp denotes a — possibly infinite — set of time intervals. It is not
// enumerated into dates; it is evaluated for coverage: "is this instant inside
// the set?". This package implements parsing/validation (with the spec's §9.1
// warnings) and the covers check. Rendering, description and RRULE export are
// out of scope.
//
// The evaluation model is time-zone agnostic: an expression carries no zone;
// the zone is a parameter of Covers (default UTC).
package dtrexp

import (
	"errors"
	"fmt"
	"time"
)

// ParseError is a positioned syntax error: what went wrong and where. Pos is
// the 0-based character offset of the offending input in the source string
// (DTRExp expressions are ASCII, so byte and character offsets coincide).
// Every error Parse returns for invalid source is a ParseError.
type ParseError struct {
	Pos int
	Msg string
}

// Error implements the error interface, rendering as
// "dtrexp: <msg> (at <pos>)".
func (e ParseError) Error() string { return fmt.Sprintf("dtrexp: %s (at %d)", e.Msg, e.Pos) }

// Warning is a positioned §9.1 warning: a construct that is legal but can
// never match. Pos is the 0-based character offset of the offending component
// in the source string, or -1 where no position is derivable from the parsed
// expression.
type Warning struct {
	Pos     int
	Message string
}

// String returns the warning message (without the position).
func (w Warning) String() string { return w.Message }

// ValidationResult is the outcome of Validate: whether the source parses, the
// positioned syntax errors when it does not, and the §9.1 warnings when it
// does. An expression can be valid and still warned.
type ValidationResult struct {
	Valid    bool
	Errors   []ParseError
	Warnings []Warning
}

// Validate checks a DTRExp string without failing: typo-shaped input comes
// back as data, never as a Go error. Errors is empty when Valid; parsing
// stops at the first syntax error, so it holds at most one entry. Warnings
// carries the same content as Warnings on the parsed expression.
func Validate(s string) ValidationResult {
	e, err := Parse(s)
	if err != nil {
		var pe ParseError
		errors.As(err, &pe)
		return ValidationResult{Errors: []ParseError{pe}}
	}
	return ValidationResult{Valid: true, Warnings: e.Warnings()}
}

// Expression is a parsed DTRExp. It is one or more union branches; an instant
// is covered iff any branch covers it. Expression values are immutable after
// Parse and safe for concurrent use.
type Expression struct {
	source   string
	branches []*branch
	warnings []Warning
}

// Parse parses a DTRExp string. It returns an error for any syntactically or
// statically invalid expression; the error is always a ParseError carrying
// the offending character offset. A successfully parsed expression may still
// carry warnings (see Warnings) for statically unsatisfiable constructs that
// are legal but never match.
func Parse(s string) (*Expression, error) {
	e, err := parse(s)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Warnings returns the §9.1 warnings collected during parsing — statically
// unsatisfiable expressions or branches that are legal but can never match.
// The slice is empty for a clean expression.
func (e *Expression) Warnings() []Warning {
	return e.warnings
}

// Source returns the original expression string.
func (e *Expression) Source() string { return e.source }

// Covers reports whether the absolute instant t falls inside the set denoted by
// the expression, evaluated in IANA time zone tz. An empty tz means UTC. It
// returns an error only if tz is not a loadable IANA zone.
func (e *Expression) Covers(t time.Time, tz string) (bool, error) {
	loc, err := loadZone(tz)
	if err != nil {
		return false, err
	}
	return e.CoversIn(t, loc), nil
}

// CoversIn is Covers with an already-resolved *time.Location (nil means UTC).
func (e *Expression) CoversIn(t time.Time, loc *time.Location) bool {
	if loc == nil {
		loc = time.UTC
	}
	f := computeFields(t, loc)
	for _, b := range e.branches {
		if b.covers(t, f, loc) {
			return true
		}
	}
	return false
}

func loadZone(tz string) (*time.Location, error) {
	if tz == "" || tz == "UTC" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("dtrexp: unknown time zone %q: %w", tz, err)
	}
	return loc, nil
}
