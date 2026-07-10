package dtrexp

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// These white-box tests pin the internal helpers on inputs the public API
// cannot produce (defensive defaults and contract guards), plus the exact
// spec constants used by static cadence validation.

func TestDaysInMonthOutOfRange(t *testing.T) {
	if got := daysInMonth(2020, 0); got != 0 {
		t.Errorf("daysInMonth(2020, 0) = %d, want 0", got)
	}
	if got := daysInMonth(2020, 13); got != 0 {
		t.Errorf("daysInMonth(2020, 13) = %d, want 0", got)
	}
}

func TestSelectorInfoUnknownDesignator(t *testing.T) {
	fv, minV, maxV, unbounded := selectorInfo(designator('X'), 0, fields{}, false)
	if fv != 0 || minV != 0 || maxV != 0 || unbounded {
		t.Errorf("selectorInfo('X') = (%d, %d, %d, %v), want zeros", fv, minV, maxV, unbounded)
	}
}

func TestMemberUnknownKind(t *testing.T) {
	// termOrdinal never reaches member (selector.covers routes it to
	// ordinalCovers); member's contract for it is "no match".
	if (term{kind: termOrdinal}).member(1, 1, 7, false) {
		t.Error("member on termOrdinal = true, want false")
	}
}

func TestDomainSpecUnknownDesignator(t *testing.T) {
	if got := domainSpec(desT, 0); got != (domSpec{}) {
		t.Errorf("domainSpec(desT) = %+v, want zero", got)
	}
}

func TestAbsoluteLenUnknownUnit(t *testing.T) {
	if got := absoluteLen(1, 'Y'); got != 0 {
		t.Errorf("absoluteLen(1, 'Y') = %v, want 0", got)
	}
}

func TestCadenceDefaults(t *testing.T) {
	if got := (&cadence{periodUnit: 'x'}).occStart(0); !got.IsZero() {
		t.Errorf("occStart with unknown unit = %v, want zero time", got)
	}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := (&cadence{durationUnit: 'x'}).occEnd(start); !got.Equal(start) {
		t.Errorf("occEnd with unknown unit = %v, want start", got)
	}
	if got := (&cadence{periodUnit: 'x'}).estimateIndex(start); got != 0 {
		t.Errorf("estimateIndex with unknown unit = %d, want 0", got)
	}
}

func TestFloorDivMod(t *testing.T) {
	cases := []struct{ a, b, q int }{
		{7, 2, 3},
		{-1, 12, -1},
		{-24, 12, -2},
		{-13, 12, -2},
		{13, 12, 1},
		{0, 12, 0},
	}
	for _, c := range cases {
		if got := floorDiv(c.a, c.b); got != c.q {
			t.Errorf("floorDiv(%d, %d) = %d, want %d", c.a, c.b, got, c.q)
		}
	}
	if got := floorMod(-1, 12); got != 11 {
		t.Errorf("floorMod(-1, 12) = %d, want 11", got)
	}
}

func TestUnitSeconds(t *testing.T) {
	maxes := map[byte]int{'Y': 366 * 86400, 'M': 31 * 86400, 'W': 7 * 86400, 'D': 86400, 'H': 3600, 'm': 60, 'x': 0}
	for u, want := range maxes {
		if got := unitMaxSeconds(u); got != want {
			t.Errorf("unitMaxSeconds(%q) = %d, want %d", string(u), got, want)
		}
	}
	mins := map[byte]int{'Y': 365 * 86400, 'M': 28 * 86400, 'W': 7 * 86400, 'D': 86400, 'H': 3600, 'm': 60, 'x': 0}
	for u, want := range mins {
		if got := unitMinSeconds(u); got != want {
			t.Errorf("unitMinSeconds(%q) = %d, want %d", string(u), got, want)
		}
	}
}

func TestParseBranchBlank(t *testing.T) {
	// parse() guards blank branches; parseBranch's own contract still rejects them.
	if _, err := parseBranch("  ", 0); err == nil {
		t.Error("parseBranch(blank) = nil error, want error")
	}
}

func TestSingleValueOfMissingSelector(t *testing.T) {
	// present-but-no-selector cannot arise from parse(); the helper's contract
	// is "not a single value".
	b := &branch{present: map[designator]bool{desY: true}}
	if v, ok := singleValueOf(b, desY); ok || v != 0 {
		t.Errorf("singleValueOf = (%d, %v), want (0, false)", v, ok)
	}
}

// synthLoc builds a TZif v1 zone with three transitions inside 48 hours:
//
//	V1 = 999974800: +00:00 -> +02:00 (gap)
//	V2 = 999996400: +02:00 -> +00:00 (2h overlap)
//	V3 = 1000007200: +00:00 -> +02:00 (gap)
//
// No real IANA zone packs transitions this densely; it exists to exercise
// resolveCompatible's overlap arm where the ±24h offset samples are inverted
// (offBefore < offAfter) and the EARLIER occurrence is candB.
func synthLoc(t *testing.T) *time.Location {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("TZif")
	buf.WriteByte(0)            // version 1
	buf.Write(make([]byte, 15)) // reserved
	// isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt
	for _, v := range []uint32{0, 0, 0, 3, 2, 8} {
		if err := binary.Write(&buf, binary.BigEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, tr := range []int32{999974800, 999996400, 1000007200} {
		if err := binary.Write(&buf, binary.BigEndian, tr); err != nil {
			t.Fatal(err)
		}
	}
	buf.Write([]byte{1, 0, 1}) // transition type indices
	// ttinfo: utoff, isdst, desigidx
	if err := binary.Write(&buf, binary.BigEndian, int32(0)); err != nil {
		t.Fatal(err)
	}
	buf.Write([]byte{0, 0})
	if err := binary.Write(&buf, binary.BigEndian, int32(7200)); err != nil {
		t.Fatal(err)
	}
	buf.Write([]byte{1, 4})
	buf.WriteString("STD\x00DST\x00")
	loc, err := time.LoadLocationFromTZData("Synthetic", buf.Bytes())
	if err != nil {
		t.Fatalf("LoadLocationFromTZData: %v", err)
	}
	return loc
}

// TestResolveCompatibleInvertedOverlap drives the overlap arm where candB is
// the earlier occurrence, both directly and through the public CoversIn.
func TestResolveCompatibleInvertedOverlap(t *testing.T) {
	loc := synthLoc(t)
	// Wall 2001-09-09T01:46:40 sits in the V2 overlap: occurrences at
	// 999992800 (offset +2h, the earlier) and 1000000000 (offset 0).
	got := resolveCompatible(2001, 9, 9, 1, 46, 40, loc)
	want := time.Unix(999992800, 0)
	if !got.Equal(want) {
		t.Errorf("resolveCompatible = %v (unix %d), want %v", got, got.Unix(), want)
	}

	e := mustParse(t, "20010909T014640/2H")
	// 30 min after the earlier occurrence: inside the first 1H window.
	if !e.CoversIn(time.Unix(999992800+1800, 0), loc) {
		t.Error("CoversIn 30min after earlier overlap anchor = false, want true")
	}
	// 30 min before it: before the anchor entirely.
	if e.CoversIn(time.Unix(999992800-1800, 0), loc) {
		t.Error("CoversIn 30min before earlier overlap anchor = true, want false")
	}
}
