package dtrexp

import (
	"archive/zip"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tuesday 07:30 UTC is 09:30 in Berlin (CEST): inside 09:00–18:00 there,
// outside it in UTC.
const zoneHours = "T0900:1800 E1:5"

func mustWithZone(t *testing.T, e *Expression, zone string) *Expression {
	t.Helper()
	b, err := e.WithZone(zone)
	if err != nil {
		t.Fatalf("WithZone(%q): %v", zone, err)
	}
	return b
}

func mustCovers(t *testing.T, e *Expression, at time.Time, tz string) bool {
	t.Helper()
	got, err := e.Covers(at, tz)
	if err != nil {
		t.Fatalf("Covers(%v, %q): %v", at, tz, err)
	}
	return got
}

// TestWithZoneCopy pins that WithZone returns a new expression and leaves the
// original unbound.
func TestWithZoneCopy(t *testing.T) {
	e := mustParse(t, zoneHours)
	berlin := mustWithZone(t, e, "Europe/Berlin")
	if berlin == e {
		t.Fatal("WithZone returned the receiver, want a copy")
	}
	if berlin.Zone() != "Europe/Berlin" {
		t.Errorf("Zone() = %q, want %q", berlin.Zone(), "Europe/Berlin")
	}
	if e.Zone() != "" {
		t.Errorf("original Zone() = %q, want \"\"", e.Zone())
	}
	at := mustInstant(t, "2026-07-07T07:30:00Z")
	if mustCovers(t, e, at, "") || e.CoversIn(at, nil) {
		t.Error("original covers 07:30Z, want UTC evaluation (not covered)")
	}
}

// TestWithZoneEvaluates pins that the no-zone calls evaluate in the bound zone.
func TestWithZoneEvaluates(t *testing.T) {
	berlin := mustWithZone(t, mustParse(t, zoneHours), "Europe/Berlin")
	at := mustInstant(t, "2026-07-07T07:30:00Z")
	if !mustCovers(t, berlin, at, "") {
		t.Error(`Covers(07:30Z, "") = false, want true (09:30 Berlin)`)
	}
	if !berlin.CoversIn(at, nil) {
		t.Error("CoversIn(07:30Z, nil) = false, want true (09:30 Berlin)")
	}
	if mustCovers(t, berlin, at.Add(-time.Hour), "") {
		t.Error(`Covers(06:30Z, "") = true, want false (08:30 Berlin)`)
	}
}

// TestWithZoneCallWins pins that a zone passed to a call, "UTC" included, wins
// over the bound zone.
func TestWithZoneCallWins(t *testing.T) {
	e := mustParse(t, zoneHours)
	at := mustInstant(t, "2026-07-07T07:30:00Z")
	berlinLoc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}

	berlin := mustWithZone(t, e, "Europe/Berlin")
	if mustCovers(t, berlin, at, "UTC") || berlin.CoversIn(at, time.UTC) {
		t.Error("an explicit UTC lost to the bound Berlin zone")
	}
	utc := mustWithZone(t, e, "UTC")
	if mustCovers(t, utc, at, "") {
		t.Error(`bound UTC: Covers(07:30Z, "") = true, want false`)
	}
	if !mustCovers(t, utc, at, "Europe/Berlin") || !utc.CoversIn(at, berlinLoc) {
		t.Error("an explicit Berlin zone lost to the bound UTC zone")
	}
	if got, err := berlin.Covers(at, "Mars/Olympus"); err == nil || got {
		t.Errorf("bound copy, unknown call zone: got (%v, %v), want (false, error)", got, err)
	}
}

// TestWithZoneRebind pins that a bound copy rebinds and leaves the first copy
// as it was.
func TestWithZoneRebind(t *testing.T) {
	berlin := mustWithZone(t, mustParse(t, zoneHours), "Europe/Berlin")
	tokyo := mustWithZone(t, berlin, "Asia/Tokyo")
	if tokyo.Zone() != "Asia/Tokyo" || berlin.Zone() != "Europe/Berlin" {
		t.Errorf("Zone() = %q, %q, want Asia/Tokyo, Europe/Berlin", tokyo.Zone(), berlin.Zone())
	}
	at := mustInstant(t, "2026-07-07T09:30:00Z")
	if mustCovers(t, tokyo, at, "") {
		t.Error("Tokyo covers 09:30Z, want false (18:30 Tokyo)")
	}
	if !mustCovers(t, berlin, at, "") {
		t.Error("Berlin does not cover 09:30Z, want true (11:30 Berlin)")
	}
}

// TestWithZoneRefuses pins that an unknown zone fails at bind, and that a
// missing zone or the host's zone is refused instead of binding UTC or Local.
func TestWithZoneRefuses(t *testing.T) {
	e := mustParse(t, zoneHours)
	cases := map[string]string{
		"Mars/Olympus":    `dtrexp: unknown time zone "Mars/Olympus"`,
		"":                `dtrexp: unknown time zone "": a time zone is required to bind`,
		"Local":           `dtrexp: unknown time zone "Local"`,
		"localtime":       `dtrexp: unknown time zone "localtime"`,
		"./localtime":     `dtrexp: unknown time zone "./localtime"`,
		"posix/localtime": `dtrexp: unknown time zone "posix/localtime"`,
		"Europe//Berlin":  `dtrexp: unknown time zone "Europe//Berlin"`,
	}
	for zone, msg := range cases {
		b, err := e.WithZone(zone)
		if b != nil || err == nil {
			t.Errorf("WithZone(%q) = (%v, %v), want (nil, error)", zone, b, err)
			continue
		}
		if err.Error() != msg {
			t.Errorf("WithZone(%q) error = %q, want %q", zone, err, msg)
		}
	}
}

// TestWithZoneKeepsExpression pins that the zone belongs to evaluation: the
// copy's source and warnings are the original's.
func TestWithZoneKeepsExpression(t *testing.T) {
	for _, src := range []string{zoneHours, "D30 M2"} {
		e := mustParse(t, src)
		b := mustWithZone(t, e, "Europe/Berlin")
		if b.Source() != e.Source() {
			t.Errorf("Source() = %q, want %q", b.Source(), e.Source())
		}
		bw, ew := b.Warnings(), e.Warnings()
		if len(bw) != len(ew) || (len(ew) > 0 && &bw[0] != &ew[0]) {
			t.Errorf("%q: Warnings() = %v, want the original's %v", src, bw, ew)
		}
	}
	if len(mustParse(t, "D30 M2").Warnings()) == 0 {
		t.Fatal("D30 M2 should warn")
	}
}

// TestZoneIdentifiers pins the accepted identifiers (API.md): the tz
// database's zone and link names, in any case, bound in the database's
// spelling; anything else is an unknown zone, whatever the host's zoneinfo
// folder holds.
func TestZoneIdentifiers(t *testing.T) {
	e := mustParse(t, zoneHours)
	at := mustInstant(t, "2026-07-07T07:30:00Z") // 09:30 in Berlin
	for given, canonical := range map[string]string{
		"Europe/Berlin": "Europe/Berlin",
		"europe/berlin": "Europe/Berlin",
		"EUROPE/BERLIN": "Europe/Berlin",
		"utc":           "UTC",
		"US/Eastern":    "US/Eastern", // a link name stays the name given
		"etc/gmt+5":     "Etc/GMT+5",
		"EUROPE/ZURICH": "Europe/Zurich",
		"europe/zurich": "Europe/Zurich", // both ends of A–Z fold in the list too
		"america/aruba": "America/Aruba",
	} {
		b := mustWithZone(t, e, given)
		if b.Zone() != canonical {
			t.Errorf("WithZone(%q).Zone() = %q, want %q", given, b.Zone(), canonical)
		}
	}
	if !mustCovers(t, e, at, "europe/berlin") {
		t.Error(`Covers(07:30Z, "europe/berlin") = false, want true (09:30 Berlin)`)
	}
	for _, zone := range []string{
		"Z", "+02:00", "UTC+2", "GMT+2", "posixrules", "Factory", "factory",
		"SystemV/AST4", "localtime", "Local", "posix/Europe/Berlin",
		"right/UTC", "/usr/share/zoneinfo/UTC", "Europe/Berlin ", " UTC",
		"Asia/\u212Aolkata", // the Kelvin sign lowercases to 'k' outside ASCII
	} {
		if got, err := e.Covers(at, zone); err == nil || got {
			t.Errorf("Covers(%q) = (%v, %v), want (false, error)", zone, got, err)
		}
		if b, err := e.WithZone(zone); err == nil || b != nil {
			t.Errorf("WithZone(%q) = (%v, %v), want (nil, error)", zone, b, err)
		}
	}
}

// TestZoneLoadFails pins the error for a listed name the host cannot load.
func TestZoneLoadFails(t *testing.T) {
	defer func(f func(string) (*time.Location, error)) { loadLocation = f }(loadLocation)
	loadLocation = func(name string) (*time.Location, error) {
		return nil, errors.New("no zone file for " + name)
	}
	_, err := mustParse(t, "M1").Covers(time.Now(), "asia/tokyo")
	if err == nil || err.Error() != `dtrexp: unknown time zone "asia/tokyo": no zone file for Asia/Tokyo` {
		t.Errorf("Covers with an unloadable zone: err = %v", err)
	}
}

// TestZoneNamesLoad pins that every listed name loads, and that the list is
// the Go toolchain's own (minus Factory): a toolchain whose database adds or
// drops a name fails here until zones.go follows.
func TestZoneNamesLoad(t *testing.T) {
	listed := map[string]bool{}
	for _, n := range strings.Fields(tzNames) {
		listed[n] = true
		if _, err := time.LoadLocation(n); err != nil {
			t.Errorf("listed zone %q does not load: %v", n, err)
		}
	}
	if len(listed) != len(tzCanonical) {
		t.Errorf("%d names, %d case-folded: two names differ only in case", len(listed), len(tzCanonical))
	}
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go toolchain: %v", err)
	}
	zr, err := zip.OpenReader(filepath.Join(strings.TrimSpace(string(out)), "lib", "time", "zoneinfo.zip"))
	if err != nil {
		t.Skipf("no zoneinfo.zip: %v", err)
	}
	defer zr.Close()
	bundled := map[string]bool{}
	for _, f := range zr.File {
		if f.Name != "Factory" {
			bundled[f.Name] = true
		}
	}
	for n := range bundled {
		if !listed[n] {
			t.Errorf("the toolchain's database names %q; zones.go does not", n)
		}
	}
	for n := range listed {
		if !bundled[n] {
			t.Errorf("zones.go names %q; the toolchain's database does not", n)
		}
	}
}
