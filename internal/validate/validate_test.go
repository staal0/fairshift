package validate_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/staal0/fairshift/internal/config"
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/excel"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/validate"
	"github.com/xuri/excelize/v2"
)

const dir = "../../testdata/example-daycare/"

func load(t *testing.T) (*config.Setup, *domain.Roster) {
	t.Helper()
	s, err := config.LoadOrg(dir + "org.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := config.LoadRoster(dir+"known-good.yaml", s.Org)
	if err != nil {
		t.Fatal(err)
	}
	return s, r
}

func TestKnownGoodHasOnlyDocumentedDeviations(t *testing.T) {
	s, r := load(t)
	rep := validate.Run(s.Org, s.Rules, r, s.Deviations)
	if n := rep.Count(rules.LevelError); n != 0 {
		for _, v := range rep.All() {
			t.Log(v.Rule, v.Message)
		}
		t.Fatalf("want 0 errors, got %d", n)
	}
	var warns []rules.Violation
	for _, v := range rep.All() {
		if v.Level == rules.LevelWarning {
			warns = append(warns, v)
		}
	}
	if len(warns) != 2 {
		t.Fatalf("want 2 warnings, got %d: %+v", len(warns), warns)
	}
	for _, w := range warns {
		if w.Rule != "office-hour" || w.Deviation == "" {
			t.Errorf("unexpected warning: %+v", w)
		}
	}
	if len(rep.Unused) != 0 {
		t.Errorf("unused deviations: %+v", rep.Unused)
	}
}

// Each mutation breaks one rule of the reference case.
func TestMutationsAreCaught(t *testing.T) {
	cases := []struct {
		name, person string
		week         int
		day          domain.Weekday
		cell         string
		rule         string
	}{
		{"opener ends late", "ida", 0, 0, "06:00-13:30 (opener)", "opener"},
		{"wrong friday opener", "camilla", 0, 4, "06:00-13:00 (opener)", "opener"},
		{"start after 09:00", "dorte", 0, 3, "09:15-16:30", "start-times"},
		{"second 06:45 starter", "dorte", 0, 3, "06:45-14:00", "ladder-0645"},
		{"nobody ends 15:45", "frida", 0, 0, "08:00-15:40 (meeting)", "ladder-1545"},
		{"too few early starts", "birthe", 0, 0, "08:00-15:15", "early-fair"},
		{"office hour with late colleague", "gitte", 0, 0, "08:30-15:30 (meeting)", "office-hour"},
		{"misses meeting", "dorte", 0, 0, "08:00-13:30 (office,meeting)", "room-meeting"},
		{"hours off", "anders", 0, 2, "08:00-15:00", "weekly-hours"},
		{"fixed shift moved", "jonna", 0, 0, "08:00-15:30", "fixed-shifts"},
		{"room c gap", "hanne", 1, 3, "", "room-c-cover"},
		{"unknown code", "anders", 0, 4, "08:00-15:00 (nope)", "structure"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, r := load(t)
			cell := domain.Cell{Person: c.person, Week: c.week, Day: c.day}
			sh, works, err := domain.ParseCell(c.cell)
			if err != nil {
				t.Fatal(err)
			}
			if works {
				r.Shifts[cell] = sh
			} else {
				delete(r.Shifts, cell)
			}
			rep := validate.Run(s.Org, s.Rules, r, s.Deviations)
			for _, v := range rep.All() {
				if v.Rule == c.rule && v.Level == rules.LevelError {
					return
				}
			}
			t.Errorf("rule %s did not report an error", c.rule)
		})
	}
}

func TestExcelExport(t *testing.T) {
	s, r := load(t)
	var buf bytes.Buffer
	msg := func(v rules.Violation) string { return v.Message }
	if err := excel.Export(&buf, s.Org, r, validate.Run(s.Org, s.Rules, r, s.Deviations), msg); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.GetCellValue("Week 1", "B5"); got != "09:00-16:45 (C)" {
		t.Errorf("B5 = %q, want closer shift with display code", got)
	}
	if fm, _ := f.GetCellFormula("Week 1", "G5"); !strings.Contains(fm, "TIMEVALUE") {
		t.Errorf("hours cell is not a live formula: %q", fm)
	}
}
