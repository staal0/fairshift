package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
	"gopkg.in/yaml.v3"
)

// duty: a named duty (opener, closer, on-call …) is held by exactly one
// person on each scheduled day, optionally by a fixed person or a rotation
// over the cycle weeks, with an implied shift shape.
type duty struct {
	Base
	P struct {
		Duty  string `yaml:"duty"`
		Shape struct {
			Start  string        `yaml:"start"`
			End    string        `yaml:"end"`
			Length DurationParam `yaml:"length"`
		} `yaml:"shape"`
		Schedule map[string]yaml.Node `yaml:"schedule"`
		Levers   []string             `yaml:"levers"` // days whose assignee is a planning lever
	}
	code  string
	label string
	sched map[domain.Weekday]assignee
}

type assignee struct {
	rotation []string // len 1 = fixed person
	pool     []string // nil + rotation nil = anyone
}

func init() {
	Register("duty", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &duty{Base: b, sched: map[domain.Weekday]assignee{}}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		a := o.Activity(r.P.Duty)
		if a == nil || a.Kind != domain.KindDuty {
			return nil, fmt.Errorf("unknown duty %q", r.P.Duty)
		}
		r.code, r.label = a.ID, a.Label
		for k, node := range r.P.Schedule {
			wd, err := domain.ParseWeekday(k)
			if err != nil {
				return nil, err
			}
			var as assignee
			switch node.Kind {
			case yaml.ScalarNode:
				if node.Value != "any" {
					as.rotation = []string{node.Value}
				}
			case yaml.SequenceNode:
				if err := node.Decode(&as.rotation); err != nil {
					return nil, err
				}
			case yaml.MappingNode:
				var m struct {
					Pool []string `yaml:"pool"`
				}
				if err := node.Decode(&m); err != nil {
					return nil, err
				}
				as.pool = m.Pool
			}
			for _, p := range append(append([]string{}, as.rotation...), as.pool...) {
				if o.Person(p) == nil {
					return nil, fmt.Errorf("schedule %s: unknown person %q", k, p)
				}
			}
			r.sched[wd] = as
		}
		for _, s := range []string{r.P.Shape.Start, r.P.Shape.End} {
			if s != "" && s != "opening" && s != "closing" {
				if _, err := domain.ParseClock(s); err != nil {
					return nil, err
				}
			}
		}
		return r, nil
	})
}

// expected returns the person who must hold the duty, or "" if open.
func (r *duty) expected(d domain.DayRef) (string, []string) {
	as := r.sched[d.Day]
	if len(as.rotation) > 0 {
		return as.rotation[d.Week%len(as.rotation)], nil
	}
	return "", as.pool
}

func (r *duty) resolve(spec string, d domain.DayRef, o *domain.Org, isEnd bool) (domain.Minute, bool) {
	switch spec {
	case "":
		return 0, false
	case "opening", "closing":
		w, ok := o.Opening[d.Day]
		if !ok {
			return 0, false
		}
		if isEnd {
			if spec == "closing" {
				return w.End, true
			}
			return w.Start, true
		}
		if spec == "opening" {
			return w.Start, true
		}
		return w.End, true
	}
	m, _ := domain.ParseClock(spec)
	return m, true
}

func (r *duty) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	for _, d := range o.AllDays() {
		if _, scheduled := r.sched[d.Day]; !scheduled {
			// Unscheduled day: nobody may carry the code.
			for _, p := range o.People {
				if s, ok := ctx.Roster.Get(p.ID, d); ok && s.Has(r.code) {
					out = append(out, r.v("%s holds %s on a day without that duty", name(o, p.ID), r.label).at(d).who(p.ID))
				}
			}
			continue
		}
		var holders []string
		for _, p := range o.People {
			if s, ok := ctx.Roster.Get(p.ID, d); ok && s.Has(r.code) {
				holders = append(holders, p.ID)
			}
		}
		if len(holders) != 1 {
			out = append(out, r.v("%d people hold %s, want exactly 1", len(holders), r.label).at(d))
			continue
		}
		h := holders[0]
		want, pool := r.expected(d)
		if want != "" && h != want {
			out = append(out, r.v("%s held by %s, scheduled for %s", r.label, name(o, h), name(o, want)).at(d).who(h))
		}
		if len(pool) > 0 && !contains(pool, h) {
			out = append(out, r.v("%s held by %s, who is not in the pool", r.label, name(o, h)).at(d).who(h))
		}
		s, _ := ctx.Roster.Get(h, d)
		if t, ok := r.resolve(r.P.Shape.Start, d, o, false); ok && s.Start != t {
			out = append(out, r.v("%s must start at %s, starts %s", r.label, t, s.Start).at(d).who(h))
		}
		if t, ok := r.resolve(r.P.Shape.End, d, o, true); ok && s.End != t {
			out = append(out, r.v("%s must end at %s, ends %s", r.label, t, s.End).at(d).who(h))
		}
		if r.P.Shape.Length.Set && s.Len() != r.P.Shape.Length.Duration {
			out = append(out, r.v("%s must last %s, lasts %s", r.label, r.P.Shape.Length.Duration, s.Len()).at(d).who(h))
		}
	}
	return out
}
