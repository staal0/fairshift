package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/staal0/fairshift/internal/config"
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/i18n"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/store"
	"github.com/staal0/fairshift/internal/validate"
)

// Rule type names and descriptions (English; translated in templates).
var ruleTypes = []struct{ Type, Name, Desc string }{
	{"coverage", "Staffing level", "At least (or at most) a number of people at work in a time window."},
	{"duty", "Duty", "One person holds a duty each day, e.g. opening or closing: a fixed person, a rotation or anyone."},
	{"shift_window", "Shift times", "Limits on when shifts start and end, and how long they are."},
	{"time_slot_count", "Start or end time", "How many people start or end at an exact time, e.g. staggered mornings."},
	{"fair_distribution", "Fair distribution", "Share a burden, like early starts, evenly between people and teams."},
	{"in_shift_task", "Task inside shift", "A task such as an office hour, a number of times per week, inside the shift."},
	{"team_event", "Team meeting", "A team meets together; everyone present, another team covers."},
	{"hours_target", "Contract hours", "Planned hours match each person's contract."},
	{"pinned", "Fixed shifts", "People with fixed shifts work exactly those shifts."},
}

func ruleTypeName(t string) string {
	for _, rt := range ruleTypes {
		if rt.Type == t {
			return rt.Name
		}
	}
	return t
}

// Field labels (English; translated in templates).
var fieldLabels = map[string]string{
	"window": "Time window", "min": "At least (people)", "max": "At most (people)",
	"scope.teams": "Only these teams", "scope.people": "Only these people", "scope.exclude_fixed": "Skip people with fixed shifts",
	"days": "Only these weekdays", "weeks": "Only these cycle weeks",
	"duty": "Duty", "shape.start": "Must start at", "shape.end": "Must end at", "shape.length": "Must last",
	"schedule": "Who holds the duty", "levers": "Flexible days (planning levers)",
	"earliest_start": "Earliest start", "latest_start": "Latest start", "earliest_end": "Earliest end", "latest_end": "Latest end",
	"allowed_starts": "Allowed start times", "min_length": "Shortest shift", "max_length": "Longest shift",
	"edge": "Count", "at": "Time", "op": "Comparison", "n": "Number of people",
	"name": "Name of what is counted", "metric.start_before": "Count shifts starting before",
	"metric.start_after": "Count shifts starting after", "metric.end_before": "Count shifts ending before",
	"metric.end_after": "Count shifts ending after", "metric.code": "Count shifts with activity",
	"metric.days": "Count only on weekdays", "period": "Period", "per_person.min": "At least per person",
	"per_person.max": "At most per person", "max_person_spread": "Max difference between people",
	"max_team_spread": "Max difference between teams (per person)",
	"task":            "Task", "per_week": "Times per week", "distinct_days_within_team": "On different days within a team",
	"others_in_team_present": "Rest of the team present meanwhile", "team_overrides": "Different time for a team",
	"event": "Event", "teams": "Teams", "exempt_fixed": "People with fixed shifts may leave early",
	"cover": "Cover while a team meets", "tolerance": "Allowed difference", "include_fixed": "Also check people with fixed shifts",
}

// Placeholders shown in empty inputs.
var kindHint = map[string]string{
	rules.FClock: "08:00", rules.FClockOrAnchor: "06:00 / opening / closing", rules.FDuration: "7:00",
	rules.FWindow: "08:00-09:00", rules.FWindowOpening: "opening / 07:30-15:30", rules.FClocks: "06:00, 06:45, 07:00",
}

type Opt struct{ Value, Label string }

type GridCell struct {
	Name     string
	Value    string
	Values   map[string]bool
	Options  []Opt
	Selected string
}

type GridRow struct {
	Label string
	Cells []GridCell
}

type FieldView struct {
	Key, Kind, Name, Label, Hint string
	Value                        string
	Values                       map[string]bool
	Options                      []Opt
	Heads                        []string
	Rows                         []GridRow
}

func getPath(m map[string]any, key string) any {
	var cur any = m
	for _, part := range strings.Split(key, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[part]
	}
	return cur
}

func setPath(m map[string]any, key string, v any) {
	parts := strings.Split(key, ".")
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "on"
		}
		return ""
	case []any:
		var parts []string
		for _, e := range x {
			parts = append(parts, str(e))
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

func set(v any) map[string]bool {
	out := map[string]bool{}
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			out[str(e)] = true
		}
	case string:
		out[x] = true
	}
	return out
}

// fieldViews builds the editor fields for a rule type from its params.
func fieldViews(o *domain.Org, typ string, params map[string]any, lang string) []FieldView {
	t := func(s string) string { return i18n.T(lang, s) }
	var out []FieldView
	for _, f := range rules.Schemas[typ] {
		fv := FieldView{Key: f.Key, Kind: f.Kind, Name: "p." + f.Key, Label: t(fieldLabels[f.Key]), Hint: kindHint[f.Kind]}
		v := getPath(params, f.Key)
		fv.Value, fv.Values = str(v), set(v)
		switch {
		case strings.HasPrefix(f.Kind, "select:"):
			fv.Kind = "select"
			fv.Options = append(fv.Options, Opt{"", "–"})
			for _, opt := range strings.Split(strings.TrimPrefix(f.Kind, "select:"), "|") {
				fv.Options = append(fv.Options, Opt{opt, t(strings.ReplaceAll(opt, "_", " "))})
			}
		case strings.HasPrefix(f.Kind, "activity:"):
			kind := strings.TrimPrefix(f.Kind, "activity:")
			fv.Kind = "select"
			fv.Options = append(fv.Options, Opt{"", "–"})
			for _, a := range o.Activities {
				if kind == "any" || string(a.Kind) == kind {
					fv.Options = append(fv.Options, Opt{a.ID, a.Code + " – " + a.Label})
				}
			}
		case f.Kind == rules.FWeekdays:
			for _, d := range o.Days {
				fv.Options = append(fv.Options, Opt{d.Key(), i18n.DayShort(lang, d)})
			}
		case f.Kind == rules.FWeeks:
			if o.CycleWeeks < 2 {
				continue
			}
			for w := 1; w <= o.CycleWeeks; w++ {
				fv.Options = append(fv.Options, Opt{strconv.Itoa(w), strconv.Itoa(w)})
			}
		case f.Kind == rules.FTeams:
			for _, tm := range o.Teams {
				fv.Options = append(fv.Options, Opt{tm.ID, tm.Name})
			}
		case f.Kind == rules.FPeople:
			for _, p := range o.People {
				fv.Options = append(fv.Options, Opt{p.ID, p.Name})
			}
		case f.Kind == rules.FDutySchedule:
			opts := []Opt{{"", "–"}, {"any", t("anyone")}}
			for _, p := range o.People {
				opts = append(opts, Opt{p.ID, p.Name})
			}
			for w := 1; w <= o.CycleWeeks; w++ {
				fv.Heads = append(fv.Heads, i18n.Tf(lang, "Week %d", w))
			}
			sched, _ := v.(map[string]any)
			for _, d := range o.Days {
				row := GridRow{Label: i18n.Day(lang, d)}
				var vals []string
				switch x := sched[d.Key()].(type) {
				case string:
					vals = []string{x}
				case []any:
					for _, e := range x {
						vals = append(vals, str(e))
					}
				}
				for w := 0; w < o.CycleWeeks; w++ {
					sel := ""
					if len(vals) > 0 {
						sel = vals[w%len(vals)]
					}
					row.Cells = append(row.Cells, GridCell{Name: fmt.Sprintf("p.schedule.%s.%d", d.Key(), w), Options: opts, Selected: sel})
				}
				fv.Rows = append(fv.Rows, row)
			}
		case f.Kind == rules.FTeamCover:
			fv.Heads = []string{t("Covered by"), t("At least")}
			cover, _ := v.(map[string]any)
			for _, tm := range o.Teams {
				c, _ := cover[tm.ID].(map[string]any)
				var opts []Opt
				for _, other := range o.Teams {
					if other.ID != tm.ID {
						opts = append(opts, Opt{other.ID, other.Name})
					}
				}
				fv.Rows = append(fv.Rows, GridRow{Label: tm.Name, Cells: []GridCell{
					{Name: "p.cover." + tm.ID + ".by", Options: opts, Values: set(c["by"])},
					{Name: "p.cover." + tm.ID + ".min", Value: str(c["min"])},
				}})
			}
		case f.Kind == rules.FTeamOverride:
			fv.Heads = []string{t("Time window"), t("Reason (shown as warning)")}
			ov, _ := v.(map[string]any)
			for _, tm := range o.Teams {
				c, _ := ov[tm.ID].(map[string]any)
				fv.Rows = append(fv.Rows, GridRow{Label: tm.Name, Cells: []GridCell{
					{Name: "p.team_overrides." + tm.ID + ".window", Value: str(c["window"])},
					{Name: "p.team_overrides." + tm.ID + ".deviation", Value: str(c["deviation"])},
				}})
			}
		}
		out = append(out, fv)
	}
	return out
}

// parseParams reads rule params from a submitted form.
func parseParams(o *domain.Org, typ string, form url.Values) (map[string]any, error) {
	params := map[string]any{}
	for _, f := range rules.Schemas[typ] {
		name := "p." + f.Key
		val := strings.TrimSpace(form.Get(name))
		switch {
		case f.Kind == rules.FBool:
			if val != "" {
				setPath(params, f.Key, true)
			}
		case f.Kind == rules.FInt:
			if val != "" {
				n, err := strconv.Atoi(val)
				if err != nil {
					return nil, fmt.Errorf("%s: %q is not a whole number", fieldLabels[f.Key], val)
				}
				setPath(params, f.Key, n)
			}
		case f.Kind == rules.FFloat:
			if val != "" {
				n, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64)
				if err != nil {
					return nil, fmt.Errorf("%s: %q is not a number", fieldLabels[f.Key], val)
				}
				setPath(params, f.Key, n)
			}
		case f.Kind == rules.FClocks:
			var list []any
			for _, part := range strings.Split(val, ",") {
				if part = strings.TrimSpace(part); part != "" {
					list = append(list, part)
				}
			}
			if len(list) > 0 {
				setPath(params, f.Key, list)
			}
		case f.Kind == rules.FWeekdays || f.Kind == rules.FTeams || f.Kind == rules.FPeople:
			if vs := form[name]; len(vs) > 0 {
				list := make([]any, len(vs))
				for i, v := range vs {
					list[i] = v
				}
				setPath(params, f.Key, list)
			}
		case f.Kind == rules.FWeeks:
			var list []any
			for _, v := range form[name] {
				if n, err := strconv.Atoi(v); err == nil {
					list = append(list, n)
				}
			}
			if len(list) > 0 {
				setPath(params, f.Key, list)
			}
		case f.Kind == rules.FDutySchedule:
			sched := map[string]any{}
			for _, d := range o.Days {
				var vals []string
				anySet := false
				for w := 0; w < o.CycleWeeks; w++ {
					v := form.Get(fmt.Sprintf("p.schedule.%s.%d", d.Key(), w))
					vals = append(vals, v)
					anySet = anySet || v != ""
				}
				if !anySet {
					continue
				}
				same := true
				for _, v := range vals {
					same = same && v == vals[0]
				}
				if same {
					sched[d.Key()] = vals[0]
					continue
				}
				list := make([]any, len(vals))
				for i, v := range vals {
					if v == "" || v == "any" {
						return nil, fmt.Errorf("%s: a rotation needs a person in every week", i18n.Day("en", d))
					}
					list[i] = v
				}
				sched[d.Key()] = list
			}
			if len(sched) > 0 {
				setPath(params, f.Key, sched)
			}
		case f.Kind == rules.FTeamCover:
			cover := map[string]any{}
			for _, tm := range o.Teams {
				by := form["p.cover."+tm.ID+".by"]
				minv := strings.TrimSpace(form.Get("p.cover." + tm.ID + ".min"))
				if len(by) == 0 && minv == "" {
					continue
				}
				c := map[string]any{}
				if len(by) > 0 {
					list := make([]any, len(by))
					for i, b := range by {
						list[i] = b
					}
					c["by"] = list
				}
				if minv != "" {
					n, err := strconv.Atoi(minv)
					if err != nil {
						return nil, fmt.Errorf("%s: %q is not a whole number", tm.Name, minv)
					}
					c["min"] = n
				}
				cover[tm.ID] = c
			}
			if len(cover) > 0 {
				setPath(params, f.Key, cover)
			}
		case f.Kind == rules.FTeamOverride:
			ov := map[string]any{}
			for _, tm := range o.Teams {
				win := strings.TrimSpace(form.Get("p.team_overrides." + tm.ID + ".window"))
				note := strings.TrimSpace(form.Get("p.team_overrides." + tm.ID + ".deviation"))
				if win == "" {
					continue
				}
				c := map[string]any{"window": win}
				if note != "" {
					c["deviation"] = note
				}
				ov[tm.ID] = c
			}
			if len(ov) > 0 {
				setPath(params, f.Key, ov)
			}
		default:
			if val != "" {
				setPath(params, f.Key, val)
			}
		}
	}
	return params, nil
}

// ---- handlers ----

type ruleListRow struct {
	Row      store.RuleRow
	TypeName string
	Err      string
}

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	var list []ruleListRow
	byKey := map[string]string{}
	for _, rr := range setup.RuleRows {
		row := ruleListRow{Row: rr, TypeName: ruleTypeName(rr.Type)}
		if e := setup.RuleErrors[rr.ID]; e != nil {
			row.Err = e.Error()
		}
		list = append(list, row)
		byKey[rr.Key()] = rr.Label
	}
	s.render(w, r, "rules", "rules", "Rules", setup.Org, map[string]any{
		"Rules": list, "Types": ruleTypes, "Deviations": setup.Deviations, "RuleLabel": byKey,
	})
}

func (s *Server) rule(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	row, ok := findRule(setup, r.PathValue("id"), r.URL.Query().Get("type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.renderRule(w, r, setup, row, "")
}

func findRule(setup *store.Setup, id, newType string) (store.RuleRow, bool) {
	if id == "new" {
		if _, ok := rules.Schemas[newType]; !ok {
			return store.RuleRow{}, false
		}
		return store.RuleRow{Type: newType, Label: ruleTypeName(newType), Severity: "hard", Enabled: true, Params: map[string]any{}}, true
	}
	for _, rr := range setup.RuleRows {
		if rr.Key() == id {
			return rr, true
		}
	}
	return store.RuleRow{}, false
}

func (s *Server) renderRule(w http.ResponseWriter, r *http.Request, setup *store.Setup, row store.RuleRow, errText string) {
	o := setup.Org
	if errText == "" {
		if e := setup.RuleErrors[row.ID]; e != nil {
			errText = e.Error()
		}
	}
	desc := ""
	for _, rt := range ruleTypes {
		if rt.Type == row.Type {
			desc = rt.Desc
		}
	}
	title := row.Label
	if row.ID == 0 {
		title = "New rule"
	}
	s.render(w, r, "rule", "rules", title, o, map[string]any{
		"R": row, "TypeName": ruleTypeName(row.Type), "Desc": desc, "Err": errText,
		"Fields": fieldViews(o, row.Type, row.Params, o.Lang), "Severities": []string{"hard", "soft", "info"},
	})
}

func (s *Server) saveRule(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	row, ok := findRule(setup, r.PathValue("id"), r.FormValue("type"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	row.Label = strings.TrimSpace(r.FormValue("label"))
	if row.Label == "" {
		row.Label = ruleTypeName(row.Type)
	}
	row.Severity = r.FormValue("severity")
	if row.Severity != "soft" && row.Severity != "info" {
		row.Severity = "hard"
	}
	row.Enabled = r.FormValue("enabled") != ""
	params, err := parseParams(setup.Org, row.Type, r.Form)
	if err == nil {
		row.Params = params
		_, err = config.BuildRule(row.Def(), setup.Org)
	}
	if err != nil {
		if params != nil {
			row.Params = params
		}
		msg := err.Error()
		if i := strings.Index(msg, "): "); i >= 0 && strings.HasPrefix(msg, "rule ") {
			msg = msg[i+3:]
		}
		s.renderRule(w, r, setup, row, msg)
		return
	}
	if _, err := s.st.SaveRule(r.Context(), row); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/rules", "msg", i18n.T(setup.Org.Lang, "Saved"))
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.st.DeleteRule(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/rules", "msg", i18n.T(setup.Org.Lang, "Deleted"))
}

func (s *Server) addDeviation(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	d := validate.Deviation{Rule: r.FormValue("rule"), Person: r.FormValue("person"), Team: r.FormValue("team"),
		Day: r.FormValue("day"), Reason: strings.TrimSpace(r.FormValue("reason"))}
	d.Week, _ = strconv.Atoi(r.FormValue("week"))
	if _, ok := findRule(setup, d.Rule, ""); !ok || d.Reason == "" {
		back(w, r, "/rules", "err", i18n.T(setup.Org.Lang, "Choose a rule and give a reason"))
		return
	}
	if err := s.st.AddDeviation(r.Context(), d); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/rules", "msg", i18n.T(setup.Org.Lang, "Saved"))
}

func (s *Server) deleteDeviation(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.st.DeleteDeviation(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/rules", "msg", i18n.T(setup.Org.Lang, "Deleted"))
}
