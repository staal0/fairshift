package web

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/staal0/fairshift/internal/config"
	"github.com/staal0/fairshift/internal/i18n"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/validate"
)

// formFor simulates a browser submitting the editor form as rendered.
func formFor(fvs []FieldView) url.Values {
	f := url.Values{}
	for _, fv := range fvs {
		switch fv.Kind {
		case rules.FBool:
			if fv.Value != "" {
				f.Set(fv.Name, "on")
			}
		case rules.FWeekdays, rules.FWeeks, rules.FTeams, rules.FPeople:
			for _, o := range fv.Options {
				if fv.Values[o.Value] {
					f.Add(fv.Name, o.Value)
				}
			}
		case rules.FDutySchedule:
			for _, row := range fv.Rows {
				for _, c := range row.Cells {
					f.Set(c.Name, c.Selected)
				}
			}
		case rules.FTeamCover:
			for _, row := range fv.Rows {
				for _, o := range row.Cells[0].Options {
					if row.Cells[0].Values[o.Value] {
						f.Add(row.Cells[0].Name, o.Value)
					}
				}
				f.Set(row.Cells[1].Name, row.Cells[1].Value)
			}
		case rules.FTeamOverride:
			for _, row := range fv.Rows {
				for _, c := range row.Cells {
					f.Set(c.Name, c.Value)
				}
			}
		default:
			f.Set(fv.Name, fv.Value)
		}
	}
	return f
}

func TestRuleEditorRoundTrip(t *testing.T) {
	s, err := config.LoadOrg("../../testdata/example-daycare/org.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ro, _, err := config.LoadRoster("../../testdata/example-daycare/known-good.yaml", s.Org)
	if err != nil {
		t.Fatal(err)
	}
	var rebuilt []rules.Rule
	for _, d := range s.RuleDefs {
		raw, _ := json.Marshal(d.Params)
		var params map[string]any
		_ = json.Unmarshal(raw, &params)
		got, err := parseParams(s.Org, d.Type, formFor(fieldViews(s.Org, d.Type, params, "en")))
		if err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		d.Params = got
		r, err := config.BuildRule(d, s.Org)
		if err != nil {
			t.Fatalf("%s: %v (params %v)", d.ID, err, got)
		}
		rebuilt = append(rebuilt, r)
	}
	rep := validate.Run(s.Org, rebuilt, ro, s.Deviations)
	if rep.Count(rules.LevelError) != 0 || rep.Count(rules.LevelWarning) != 2 {
		for _, v := range rep.All() {
			t.Log(v.Rule, v.Level, v.Message)
		}
		t.Fatalf("after editor round trip: %d errors, %d warnings", rep.Count(rules.LevelError), rep.Count(rules.LevelWarning))
	}
}

var keyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\{\{-?\s*t "([^"]+)"`),
	regexp.MustCompile(`\(t "([^"]+)"\)`),
	regexp.MustCompile(`\btf "([^"]+)"`),
	regexp.MustCompile(`i18n\.Tf?\([^,]+, "([^"]+)"`),
	regexp.MustCompile(`\bt\("([^"]+)"\)`),
	regexp.MustCompile(`r\.(?:v|inPeriod)\((?:per, )?"([^"]+)"`),
	regexp.MustCompile(`\bbad\("([^"]+)"`),
	regexp.MustCompile(`: +"(%d people [^"]+)"`),
	regexp.MustCompile(`plural [^"]*"([^"]+)" "([^"]+)"`),
}

// uiKeys collects the English texts used as translation keys.
func uiKeys(t *testing.T) []string {
	keys := map[string]bool{}
	files, _ := filepath.Glob("templates/*.html")
	more, _ := filepath.Glob("*.go")
	files = append(files, more...)
	for _, dir := range []string{"../rules/*.go", "../validate/*.go", "../excel/*.go"} {
		m, _ := filepath.Glob(dir)
		files = append(files, m...)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range keyPatterns {
			for _, m := range re.FindAllStringSubmatch(string(b), -1) {
				for _, k := range m[1:] {
					keys[k] = true
				}
			}
		}
	}
	// rules.go in the shape files prefixes formats with "%s: ".
	for k := range keys {
		if strings.HasPrefix(k, "starts ") || strings.HasPrefix(k, "ends ") || strings.HasPrefix(k, "start ") || strings.HasPrefix(k, "shift ") {
			delete(keys, k)
			keys["%s: "+k] = true
		}
	}
	delete(keys, "%s: ")
	for k := range keys {
		if strings.Contains(k, "in week %d") && strings.Contains(k, " × ") || strings.Contains(k, "between") {
			keys[strings.Replace(k, "week %d", "the cycle", 1)] = true
		}
	}
	for _, rt := range ruleTypes {
		keys[rt.Name], keys[rt.Desc] = true, true
	}
	for _, l := range fieldLabels {
		keys[l] = true
	}
	for _, f := range rules.Schemas {
		for _, fl := range f {
			if strings.HasPrefix(fl.Kind, "select:") {
				for _, o := range strings.Split(strings.TrimPrefix(fl.Kind, "select:"), "|") {
					keys[strings.ReplaceAll(o, "_", " ")] = true
				}
			}
		}
	}
	for _, k := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday",
		"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun", "pass", "info", "warning", "error", "hard", "soft",
		"duty", "task", "event", "week %d", "fixed shifts, accepted as input", "the cycle"} {
		keys[k] = true
	}
	var out []string
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestDanishIsComplete(t *testing.T) {
	if missing := i18n.Missing("da", uiKeys(t)); len(missing) > 0 {
		t.Errorf("da.json is missing %d keys:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}
