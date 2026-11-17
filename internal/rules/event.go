package rules

import (
	"fmt"
	"sort"

	"github.com/staal0/fairshift/internal/domain"
)

// team_event: a team meets (or trains) together; everyone attends, and
// another team covers with a minimum headcount.
type teamEvent struct {
	Base
	P struct {
		Event       string   `yaml:"event"`
		Window      Span     `yaml:"window"`
		Teams       []string `yaml:"teams"`
		PerWeek     int      `yaml:"per_week"`
		ExemptFixed bool     `yaml:"exempt_fixed"`
		Cover       map[string]struct {
			By  []string `yaml:"by"`
			Min int      `yaml:"min"`
		} `yaml:"cover"`
	}
	code  string
	label string
}

func init() {
	Register("team_event", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &teamEvent{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		a := o.Activity(r.P.Event)
		if a == nil || a.Kind != domain.KindEvent {
			return nil, fmt.Errorf("unknown event %q", r.P.Event)
		}
		r.code, r.label = a.ID, a.Label
		if !r.P.Window.Set {
			return nil, fmt.Errorf("missing window")
		}
		if r.P.PerWeek == 0 {
			r.P.PerWeek = 1
		}
		if len(r.P.Teams) == 0 {
			for _, t := range o.Teams {
				r.P.Teams = append(r.P.Teams, t.ID)
			}
		}
		for _, t := range r.P.Teams {
			if o.Team(t) == nil {
				return nil, fmt.Errorf("unknown team %q", t)
			}
		}
		for t, c := range r.P.Cover {
			if o.Team(t) == nil {
				return nil, fmt.Errorf("cover: unknown team %q", t)
			}
			for _, by := range c.By {
				if o.Team(by) == nil {
					return nil, fmt.Errorf("cover: unknown team %q", by)
				}
			}
		}
		return r, nil
	})
}

func (r *teamEvent) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	win := r.P.Window.Window
	for w := 0; w < o.CycleWeeks; w++ {
		meetDays := map[string][]domain.Weekday{}
		for _, tid := range r.P.Teams {
			team := o.Team(tid)
			members := o.Members(tid)
			// Days where anyone on the team carries the event code.
			daySet := map[domain.Weekday]bool{}
			for _, p := range members {
				for _, day := range o.Days {
					if s, ok := ctx.Roster.Get(p.ID, domain.DayRef{Week: w, Day: day}); ok && s.Has(r.code) {
						daySet[day] = true
					}
				}
			}
			var days []domain.Weekday
			for d := range daySet {
				days = append(days, d)
			}
			sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
			meetDays[tid] = days
			if len(days) != r.P.PerWeek {
				out = append(out, r.v("%s has %s on %d day(s) in week %d, want %d", team.Name, r.label, len(days), w+1, r.P.PerWeek).team(tid))
			}
			for _, day := range days {
				d := domain.DayRef{Week: w, Day: day}
				for _, p := range members {
					s, works := ctx.Roster.Get(p.ID, d)
					if !works {
						out = append(out, r.v("%s is off during %s's %s", name(o, p.ID), team.Name, r.label).at(d).who(p.ID).team(tid))
						continue
					}
					if !s.Has(r.code) {
						out = append(out, r.v("%s is not marked for %s's %s", name(o, p.ID), team.Name, r.label).at(d).who(p.ID).team(tid))
					}
					if !s.Covers(win) && !(r.P.ExemptFixed && p.IsFixed()) {
						out = append(out, r.v("%s works %s and misses part of %s's %s %s", name(o, p.ID), s.Window, team.Name, r.label, win).at(d).who(p.ID).team(tid))
					}
				}
			}
		}
		// Cover requirements.
		for tid, c := range r.P.Cover {
			for _, day := range meetDays[tid] {
				d := domain.DayRef{Week: w, Day: day}
				n := 0
				for _, by := range c.By {
					for _, p := range o.Members(by) {
						s, ok := ctx.Roster.Get(p.ID, d)
						// A coverer in their own team's event at the same time doesn't count.
						if ok && s.Covers(win) && !(s.Has(r.code) && contains(meetDaysTeams(meetDays, day), by)) {
							n++
						}
					}
				}
				if n < c.Min {
					out = append(out, r.v("only %d covering %s's %s %s, want %d", n, o.Team(tid).Name, r.label, win, c.Min).at(d).team(tid))
				}
			}
		}
	}
	return out
}

func meetDaysTeams(m map[string][]domain.Weekday, day domain.Weekday) []string {
	var out []string
	for t, days := range m {
		for _, d := range days {
			if d == day {
				out = append(out, t)
			}
		}
	}
	return out
}
