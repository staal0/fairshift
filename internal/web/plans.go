package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/staal0/fairshift/internal/analytics"
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/excel"
	"github.com/staal0/fairshift/internal/i18n"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/store"
	"github.com/staal0/fairshift/internal/validate"
)

func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	plans, err := s.st.Plans(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, r, "plans", "plans", "Plans", setup.Org, map[string]any{"Plans": plans})
}

// newPlan creates a plan, prefilled with everyone's fixed shifts.
func (s *Server) newPlan(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = i18n.T(setup.Org.Lang, "New plan")
	}
	ro := domain.NewRoster()
	for _, p := range setup.Org.People {
		for _, d := range setup.Org.AllDays() {
			if win, ok := p.Fixed[d.Day]; ok {
				ro.Shifts[domain.Cell{Person: p.ID, Week: d.Week, Day: d.Day}] = domain.Shift{Window: win}
			}
		}
	}
	id, err := s.st.CreatePlan(r.Context(), name, ro)
	if err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/plans/%d", id), http.StatusSeeOther)
}

func (s *Server) loadPlan(r *http.Request) (*store.Setup, *store.PlanRow, *domain.Roster, error) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		return nil, nil, nil, err
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := s.st.Plan(r.Context(), id)
	if err != nil {
		return nil, nil, nil, err
	}
	ro, err := s.st.Roster(r.Context(), id)
	return setup, p, ro, err
}

// Cell is one grid cell in the plan view.
type Cell struct {
	Person   string
	Day      domain.Weekday
	Time     string
	Acts     []ActToggle
	Level    string // "", "info", "warning", "error"
	Findings []rules.Violation
}

type ActToggle struct {
	*domain.Activity
	On bool
}

type PersonRow struct {
	Person *domain.Person
	Cells  []Cell
	Hours  domain.Duration
	Diff   domain.Duration
}

type TeamBlock struct {
	Team *domain.Team
	Rows []PersonRow
}

type DayHead struct {
	Day      domain.Weekday
	Level    string
	Findings []rules.Violation
}

func worst(vs []rules.Violation) string {
	w := -1
	for _, v := range vs {
		w = max(w, int(v.Level))
	}
	if w < 0 {
		return ""
	}
	return rules.Level(w).String()
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	setup, p, ro, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	o := setup.Org
	week, _ := strconv.Atoi(r.URL.Query().Get("week"))
	if week < 1 || week > o.CycleWeeks {
		week = 1
	}
	wk := week - 1
	rep := validate.Run(o, setup.Rules, ro, setup.Deviations)

	type key struct {
		p string
		d domain.Weekday
	}
	cellFind := map[key][]rules.Violation{}
	dayFind := map[domain.Weekday][]rules.Violation{}
	for _, v := range rep.All() {
		switch {
		case v.Day == nil || v.Day.Week != wk:
		case v.Person != "":
			cellFind[key{v.Person, v.Day.Day}] = append(cellFind[key{v.Person, v.Day.Day}], v)
		default:
			dayFind[v.Day.Day] = append(dayFind[v.Day.Day], v)
		}
	}

	var blocks []TeamBlock
	for _, t := range o.Teams {
		b := TeamBlock{Team: t}
		for _, pp := range o.Members(t.ID) {
			pr := PersonRow{Person: pp}
			for _, day := range o.Days {
				c := Cell{Person: pp.ID, Day: day}
				sh, works := ro.Get(pp.ID, domain.DayRef{Week: wk, Day: day})
				if works {
					c.Time = sh.String()
					pr.Hours += sh.Len()
				}
				for _, a := range o.Activities {
					c.Acts = append(c.Acts, ActToggle{Activity: a, On: works && sh.Has(a.ID)})
				}
				c.Findings = cellFind[key{pp.ID, day}]
				c.Level = worst(c.Findings)
				pr.Cells = append(pr.Cells, c)
			}
			pr.Diff = pr.Hours - pp.WeeklyMinutes
			b.Rows = append(b.Rows, pr)
		}
		blocks = append(blocks, b)
	}
	var heads []DayHead
	for _, d := range o.Days {
		heads = append(heads, DayHead{d, worst(dayFind[d]), dayFind[d]})
	}
	var weeks []int
	for i := 1; i <= o.CycleWeeks; i++ {
		weeks = append(weeks, i)
	}
	// Where activities may go, from the hard rules (used by the editor).
	ld := rules.Limits{Open: map[string][2]int{}}
	for d, win := range o.Opening {
		ld.Open[d.Key()] = [2]int{int(win.Start), int(win.End)}
	}
	for _, rl := range setup.Rules {
		if lim, ok := rl.(rules.Limiter); ok && rl.Severity() == rules.Hard {
			lim.AddLimits(o, wk, &ld)
		}
	}
	limitsJSON, _ := json.Marshal(ld)

	s.render(w, r, "plan", "plans", p.Name, o, map[string]any{
		"Limits": string(limitsJSON),
		"Plan":   p, "Week": week, "Weeks": weeks, "Heads": heads, "Blocks": blocks,
		"Report": rep, "Errors": rep.Count(rules.LevelError), "Warnings": rep.Count(rules.LevelWarning),
		"Infos": rep.Count(rules.LevelInfo), "RuleErrors": len(setup.RuleErrors),
		"Hours":    analytics.Hours(o, ro),
		"Fairness": analytics.Fairness(o, setup.Rules, ro),
		"Coverage": analytics.CoverageGrid(o, ro, wk, 15),
	})
}

func (s *Server) saveCells(w http.ResponseWriter, r *http.Request) {
	setup, p, ro, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	o := setup.Org
	week, _ := strconv.Atoi(r.FormValue("week"))
	if week < 1 || week > o.CycleWeeks {
		http.Error(w, "bad week", 400)
		return
	}
	type change struct {
		cell  domain.Cell
		shift domain.Shift
		works bool
	}
	var bad []string
	var changes []change
	for _, pp := range o.People {
		for _, day := range o.Days {
			field := "c_" + pp.ID + "_" + day.Key()
			if _, present := r.Form[field]; !present {
				continue
			}
			c := domain.Cell{Person: pp.ID, Week: week - 1, Day: day}
			var sh domain.Shift
			text := strings.TrimSpace(r.FormValue(field))
			works := text != ""
			if works {
				// ParseCell also accepts "08:00-15:00 (A,K)" from pages loaded before
				// codes moved to buttons; those codes count as chosen too.
				parsed, _, err := domain.ParseCell(text)
				codes, ok := activityIDs(o, parsed.Acts)
				if err != nil || !ok {
					bad = append(bad, fmt.Sprintf("%s %s: %q %s", pp.Short, i18n.DayShort(o.Lang, day), text, i18n.T(o.Lang, "is not a valid time")))
					continue
				}
				sh.Window = parsed.Window
				chosen := append(r.Form["a_"+pp.ID+"_"+day.Key()], codes...)
				for _, a := range o.Activities { // keep the configured order
					for _, id := range chosen {
						if id == a.ID {
							sh.Acts = append(sh.Acts, a.ID)
							break
						}
					}
				}
			}
			old, had := ro.Shifts[c]
			if had == works && (!works || domain.FormatCell(old) == domain.FormatCell(sh)) {
				continue
			}
			changes = append(changes, change{c, sh, works})
		}
	}
	path := fmt.Sprintf("/plans/%d?week=%d", p.ID, week)
	if len(bad) > 0 {
		// All or nothing: a partly saved plan is confusing.
		back(w, r, path, "err", i18n.T(o.Lang, "Not saved")+": "+strings.Join(bad, "; "))
		return
	}
	for _, ch := range changes {
		if err := s.st.SetCell(r.Context(), p.ID, ch.cell, ch.shift, ch.works); err != nil {
			fail(w, err)
			return
		}
	}
	changed := len(changes)
	back(w, r, path, "msg", i18n.Tf(o.Lang, "%d cell(s) saved", changed))
}

func (s *Server) renamePlan(w http.ResponseWriter, r *http.Request) {
	_, p, _, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	if name := strings.TrimSpace(r.FormValue("name")); name != "" {
		if err := s.st.RenamePlan(r.Context(), p.ID, name); err != nil {
			fail(w, err)
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/plans/%d", p.ID), http.StatusSeeOther)
}

func (s *Server) copyPlan(w http.ResponseWriter, r *http.Request) {
	setup, p, ro, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	id, err := s.st.CreatePlan(r.Context(), p.Name+" ("+i18n.T(setup.Org.Lang, "copy")+")", ro)
	if err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/plans/%d", id), http.StatusSeeOther)
}

func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request) {
	setup, p, _, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	if r.FormValue("confirm") != "yes" {
		back(w, r, fmt.Sprintf("/plans/%d", p.ID), "err", i18n.T(setup.Org.Lang, "Tick the box to confirm"))
		return
	}
	if err := s.st.DeletePlan(r.Context(), p.ID); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/plans", "msg", i18n.T(setup.Org.Lang, "Plan deleted"))
}

func (s *Server) exportXLSX(w http.ResponseWriter, r *http.Request) {
	setup, p, ro, err := s.loadPlan(r)
	if err != nil {
		fail(w, err)
		return
	}
	rep := validate.Run(setup.Org, setup.Rules, ro, setup.Deviations)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, strings.ReplaceAll(p.Name, `"`, "")))
	tr := func(v rules.Violation) string { return message(setup.Org.Lang, v) }
	if err := excel.Export(w, setup.Org, ro, rep, tr); err != nil {
		log.Print(err)
	}
}

// activityIDs maps display codes (or IDs) to activity IDs.
func activityIDs(o *domain.Org, tokens []string) ([]string, bool) {
	var out []string
	for _, tok := range tokens {
		found := false
		for _, a := range o.Activities {
			if tok == a.Code || tok == a.ID {
				out = append(out, a.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return out, true
}
