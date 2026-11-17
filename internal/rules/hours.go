package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

// hours_target: planned hours match the contract per week (or averaged over
// the cycle), within a tolerance.
// People with fixed shifts are reported as info (their hours are input,
// not a decision), unless include_fixed is true.
type hoursTarget struct {
	Base
	P struct {
		Scope        Scope         `yaml:"scope"`
		Period       string        `yaml:"period"` // week | cycle
		Tolerance    DurationParam `yaml:"tolerance"`
		IncludeFixed bool          `yaml:"include_fixed"`
	}
}

func init() {
	Register("hours_target", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &hoursTarget{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		if r.P.Period == "" {
			r.P.Period = "week"
		}
		if r.P.Period != "week" && r.P.Period != "cycle" {
			return nil, fmt.Errorf("period must be week or cycle")
		}
		return r, r.P.Scope.validate(o)
	})
}

// Planned returns planned minutes per person per cycle week.
func Planned(o *domain.Org, ro *domain.Roster, person string) []domain.Duration {
	out := make([]domain.Duration, o.CycleWeeks)
	for _, d := range o.AllDays() {
		if s, ok := ro.Get(person, d); ok {
			out[d.Week] += s.Len()
		}
	}
	return out
}

func (r *hoursTarget) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	tol := r.P.Tolerance.Duration
	for _, p := range r.P.Scope.Select(o) {
		if p.WeeklyMinutes == 0 {
			continue
		}
		planned := Planned(o, ctx.Roster, p.ID)
		check := func(week int, got, want domain.Duration) {
			diff := got - want
			if diff >= -tol && diff <= tol {
				return
			}
			var v Violation
			if week == 0 {
				v = r.v("%s: %s planned in the cycle, contract %s (%+d min)", name(o, p.ID), got, want, int(diff))
			} else {
				v = r.v("%s: %s planned in week %d, contract %s (%+d min)", name(o, p.ID), got, week, want, int(diff))
			}
			v = v.who(p.ID)
			if p.IsFixed() && !r.P.IncludeFixed {
				v = v.lvl(LevelInfo)
				v.Note = "fixed shifts, accepted as input"
			}
			out = append(out, v)
		}
		if r.P.Period == "cycle" {
			var sum domain.Duration
			for _, x := range planned {
				sum += x
			}
			check(0, sum, p.WeeklyMinutes*domain.Duration(o.CycleWeeks))
			continue
		}
		for w, got := range planned {
			check(w+1, got, p.WeeklyMinutes)
		}
	}
	return out
}

// pinned: people with fixed shifts work exactly those shifts.
type pinned struct{ Base }

func init() {
	Register("pinned", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		var empty struct{}
		return &pinned{b}, dec(&empty)
	})
}

func (r *pinned) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	for _, p := range o.People {
		if !p.IsFixed() {
			continue
		}
		for _, d := range o.AllDays() {
			want, fixed := p.Fixed[d.Day]
			s, works := ctx.Roster.Get(p.ID, d)
			switch {
			case fixed && !works:
				out = append(out, r.v("%s is fixed to %s but is off", name(o, p.ID), want).at(d).who(p.ID))
			case !fixed && works:
				out = append(out, r.v("%s has no fixed shift this day but works %s", name(o, p.ID), s.Window).at(d).who(p.ID))
			case fixed && s.Window != want:
				out = append(out, r.v("%s is fixed to %s but works %s", name(o, p.ID), want, s.Window).at(d).who(p.ID))
			}
		}
	}
	return out
}
