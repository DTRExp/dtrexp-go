package dtrexp

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

type vectorFile struct {
	Spec     float64 `json:"spec"`
	Coverage []struct {
		ID         string          `json:"id"`
		Expression string          `json:"expression"`
		TZ         string          `json:"tz"`
		Cases      json.RawMessage `json:"cases"`
	} `json:"coverage"`
	Invalid []struct {
		Expression string `json:"expression"`
		Reason     string `json:"reason"`
	} `json:"invalid"`
	Warnings []struct {
		Expression string `json:"expression"`
		Warning    string `json:"warning"`
	} `json:"warnings"`
	Quiet []struct {
		Expression string `json:"expression"`
		Note       string `json:"note"`
	} `json:"quiet"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	data, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vf vectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	return vf
}

func TestCoverage(t *testing.T) {
	vf := loadVectors(t)
	pass, fail := 0, 0
	for _, grp := range vf.Coverage {
		e, err := Parse(grp.Expression)
		if err != nil {
			t.Errorf("[%s] %q: unexpected parse error: %v", grp.ID, grp.Expression, err)
			fail++
			continue
		}
		var cases map[string]bool
		if err := json.Unmarshal(grp.Cases, &cases); err != nil {
			t.Fatalf("[%s] bad cases: %v", grp.ID, err)
		}
		for iso, want := range cases {
			instant, err := time.Parse(time.RFC3339Nano, iso)
			if err != nil {
				t.Fatalf("[%s] bad instant %q: %v", grp.ID, iso, err)
			}
			got, err := e.Covers(instant, grp.TZ)
			if err != nil {
				t.Fatalf("[%s] covers error: %v", grp.ID, err)
			}
			if got != want {
				t.Errorf("[%s] %q @ %s (%s): got %v want %v", grp.ID, grp.Expression, iso, grp.TZ, got, want)
				fail++
			} else {
				pass++
			}
		}
	}
	t.Logf("coverage instants: %d passed, %d failed", pass, fail)
}

func TestInvalid(t *testing.T) {
	vf := loadVectors(t)
	pass, fail := 0, 0
	for _, c := range vf.Invalid {
		_, err := Parse(c.Expression)
		if err == nil {
			t.Errorf("expected parse error for %q (%s), got none", c.Expression, c.Reason)
			fail++
			continue
		}
		var pe ParseError
		if !errors.As(err, &pe) {
			t.Errorf("parse error for %q is %T, want ParseError", c.Expression, err)
			fail++
			continue
		}
		if pe.Pos < 0 || pe.Pos > len(c.Expression) {
			t.Errorf("parse error for %q at %d, want a position within the source", c.Expression, pe.Pos)
			fail++
			continue
		}
		pass++
	}
	t.Logf("invalid: %d rejected, %d wrongly accepted", pass, fail)
}

func TestWarnings(t *testing.T) {
	vf := loadVectors(t)
	pass, fail := 0, 0
	for _, c := range vf.Warnings {
		e, err := Parse(c.Expression)
		if err != nil {
			t.Errorf("expected %q to parse (with warning), got error: %v", c.Expression, err)
			fail++
			continue
		}
		w := e.Warnings()
		if len(w) == 0 {
			t.Errorf("expected a warning for %q (%s), got none", c.Expression, c.Warning)
			fail++
			continue
		}
		ok := true
		for _, warn := range w {
			if warn.Pos < 0 || warn.Pos >= len(c.Expression) {
				t.Errorf("warning for %q at %d, want a position within the source", c.Expression, warn.Pos)
				ok = false
			}
		}
		if ok {
			pass++
		} else {
			fail++
		}
	}
	t.Logf("warnings: %d flagged, %d missed", pass, fail)
}

func TestQuiet(t *testing.T) {
	vf := loadVectors(t)
	pass, fail := 0, 0
	for _, c := range vf.Quiet {
		e, err := Parse(c.Expression)
		if err != nil {
			t.Errorf("expected %q to parse quietly (%s), got error: %v", c.Expression, c.Note, err)
			fail++
			continue
		}
		if w := e.Warnings(); len(w) > 0 {
			t.Errorf("expected NO warning for %q (%s), got %v", c.Expression, c.Note, w)
			fail++
		} else {
			pass++
		}
	}
	t.Logf("quiet: %d clean, %d false positives", pass, fail)
}
