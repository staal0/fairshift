package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

// in_shift_task: a task that happens inside a shift (office hour, admin
// time). The shift must cover the task window – the task is not the start
// of the shift.
type inShiftTask struct {
	Base
	P struct {
		Task                   string `yaml:"task"`
		Window                 Span   `yaml:"window"`
		Scope                  Scope  `yaml:"scope"`
		PerWeek                *int   `yaml:"per_week"`
		DistinctDaysWithinTeam bool   `yaml:"distinct_days_within_team"`
		OthersInTeamPresent    bool   `yaml:"others_in_team_present"`
		TeamOverrides          map[string]struct {
			Window    Span   `yaml:"window"`
			Deviation string `yaml:"deviation"`
		} `yaml:"team_overrides"`
	}
	code  string
	label string
}

func init() {
	Register("in_shift_task", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &inShiftTask{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		a := o.Activity(r.P.Task)
		if a == nil || a.Kind != domain.KindTask {
			return nil, fmt.Errorf("unknown task %q", r.P.Task)
		}
		r.code, r.label = a.ID, a.Label
		if !r.P.Window.Set {
			return nil, fmt.Errorf("missing window")
		}
		for t := range r.P.TeamOverrides {
			if o.Team(t) == nil {
				return nil, fmt.Errorf("team_overrides: unknown team %q", t)
			}
		}
		return r, r.P.Scope.validate(o)
	})
}

func (r *inShiftTask) windowFor(team string) domain.Window {
	if ov, ok := r.P.TeamOverrides[team]; ok && ov.Window.Set {
		return ov.Window.Window
	}
	return r.P.Window.Window
}

func (r *inShiftTask) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org

	// Overrides that deviate from the base rule are reported as warnings so
	// they stay visible on every plan.
	for _, t := range o.Teams {
		if ov, ok := r.P.TeamOverrides[t.ID]; ok && ov.Deviation != "" {
			v := r.v("%s: %s runs %s instead of %s", t.Name, r.label, r.windowFor(t.ID), r.P.Window.Window).lvl(LevelWarning).team(t.ID)
			v.Deviation, v.Note = "override", ov.Deviation
			out = append(out, v)
		}
	}

	people := r.P.Scope.Select(o)
	for w := 0; w < o.CycleWeeks; w++ {
		usedDay := map[string]map[domain.Weekday]string{} // team -> day -> person
		for _, p := range people {
			win := r.windowFor(p.Team)
			count := 0
			for _, day := range o.Days {
				d := domain.DayRef{Week: w, Day: day}
				s, ok := ctx.Roster.Get(p.ID, d)
				if !ok || !s.Has(r.code) {
					continue
				}
				count++
				if !s.Covers(win) {
					out = append(out, r.v("%s has %s %s but works %s", name(o, p.ID), r.label, win, s.Window).at(d).who(p.ID))
				}
				if r.P.DistinctDaysWithinTeam {
					if usedDay[p.Team] == nil {
						usedDay[p.Team] = map[domain.Weekday]string{}
					}
					if other, dup := usedDay[p.Team][day]; dup {
						out = append(out, r.v("%s and %s both have %s on the same day", name(o, other), name(o, p.ID), r.label).at(d).who(p.ID).team(p.Team))
					}
					usedDay[p.Team][day] = p.ID
				}
				if r.P.OthersInTeamPresent {
					for _, m := range o.Members(p.Team) {
						if m.ID == p.ID {
							continue
						}
						ms, works := ctx.Roster.Get(m.ID, d)
						if works && !ms.Covers(win) {
							out = append(out, r.v("%s is not on the team %s during %s's %s (works %s)",
								name(o, m.ID), win, name(o, p.ID), r.label, ms.Window).at(d).who(m.ID).team(p.Team))
						}
					}
				}
			}
			if r.P.PerWeek != nil && count != *r.P.PerWeek {
				out = append(out, r.v("%s has %d × %s in week %d, want %d", name(o, p.ID), count, r.label, w+1, *r.P.PerWeek).who(p.ID))
			}
		}
	}
	return out
}
