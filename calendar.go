package dtrexp

import "time"

// fields holds the calendar coordinates of an instant, computed once per
// Covers call in the evaluation zone (spec §9 step 1).
type fields struct {
	calYear  int // calendar year
	weekYear int // ISO week-year
	isoWeek  int // ISO week number 1..53
	quarter  int // 1..4
	month    int // 1..12
	dom      int // day of month 1..31
	doq      int // day of quarter 1..92
	doy      int // day of year 1..366
	weekday  int // ISO 1(Mon)..7(Sun)
	hour     int // 0..23
	minute   int // 0..59
	second   int // 0..59
	msOfDay  int // milliseconds since local midnight, 0..86399999

	daysInMonth     int
	daysInQuarter   int
	daysInYear      int
	weeksInWeekYear int
}

func computeFields(t time.Time, loc *time.Location) fields {
	lt := t.In(loc)
	y := lt.Year()
	mo := int(lt.Month())
	wy, wk := lt.ISOWeek()
	wd := int(lt.Weekday()) // Sunday=0..Saturday=6
	if wd == 0 {
		wd = 7
	}
	q := (mo-1)/3 + 1
	doy := lt.YearDay()

	// Day of quarter: day-of-year minus the day-of-year of the quarter's first
	// day, plus one. Computed with civil dates to stay DST-safe.
	qStartMonth := (q-1)*3 + 1
	qStartDoy := time.Date(y, time.Month(qStartMonth), 1, 0, 0, 0, 0, loc).YearDay()
	doq := doy - qStartDoy + 1

	f := fields{
		calYear:         y,
		weekYear:        wy,
		isoWeek:         wk,
		quarter:         q,
		month:           mo,
		dom:             lt.Day(),
		doq:             doq,
		doy:             doy,
		weekday:         wd,
		hour:            lt.Hour(),
		minute:          lt.Minute(),
		second:          lt.Second(),
		msOfDay:         lt.Hour()*3600000 + lt.Minute()*60000 + lt.Second()*1000 + lt.Nanosecond()/1_000_000,
		daysInMonth:     daysInMonth(y, mo),
		daysInQuarter:   daysInMonth(y, qStartMonth) + daysInMonth(y, qStartMonth+1) + daysInMonth(y, qStartMonth+2),
		daysInYear:      daysInYear(y),
		weeksInWeekYear: weeksInISOYear(wy),
	}
	return f
}

func isLeap(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

func daysInMonth(y, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeap(y) {
			return 29
		}
		return 28
	}
	return 0
}

func daysInYear(y int) int {
	if isLeap(y) {
		return 366
	}
	return 365
}

// weeksInISOYear returns the number of ISO weeks (52 or 53) in an ISO week-year.
// December 28 is always in the last ISO week of its week-year.
func weeksInISOYear(wy int) int {
	_, wk := time.Date(wy, 12, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return wk
}

// validCivilDate reports whether y-m-d is a real calendar date.
func validCivilDate(y, m, d int) bool {
	if m < 1 || m > 12 {
		return false
	}
	return d >= 1 && d <= daysInMonth(y, m)
}
