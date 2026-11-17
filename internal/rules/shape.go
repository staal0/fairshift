package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

// shift_window: bounds on start/end times and shift length.
type shiftWindow struct {
	Base
	P struct {
		Scope         Scope `yaml:"scope"`
		DaySel        `yaml:",inline"`
		EarliestStart Clock         `yaml:"earliest_start"`
		LatestStart   Clock         `yaml:"latest_start"`
		EarliestEnd   Clock         `yaml:"earliest_end"`
		LatestEnd     Clock         `yaml:"latest_end"`
		AllowedStarts []Clock       `yaml:"allowed_starts"`
		MinLength     DurationParam `yaml:"min_length"`
		MaxLength     DurationParam `yaml:"max_length"`
	}
}

func init() {
	Register("shift_window", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &shiftWindow{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		if err := r.P.Scope.validate(o); err != nil {
			return nil, err
		}
		return r, r.P.DaySel.compile()
	})
}

func (r *shiftWindow) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	for _, p := range r.P.Scope.Select(o) {
		for _, d := range r.P.DaySel.Select(o) {
			s, ok := ctx.Roster.Get(p.ID, d)
			if !ok {
				continue
			}
			bad := func(f string, a ...any) {
				out = append(out, r.v("%s: "+f, append([]any{name(o, p.ID)}, a...)...).at(d).who(p.ID))
			}
			if r.P.EarliestStart.Set && s.Start < r.P.EarliestStart.Minute {
				bad("starts %s, earliest allowed %s", s.Start, r.P.EarliestStart.Minute)
			}
			if r.P.LatestStart.Set && s.Start > r.P.LatestStart.Minute {
				bad("starts %s, latest allowed %s", s.Start, r.P.LatestStart.Minute)
			}
			if r.P.EarliestEnd.Set && s.End < r.P.EarliestEnd.Minute {
				bad("ends %s, earliest allowed %s", s.End, r.P.EarliestEnd.Minute)
			}
			if r.P.LatestEnd.Set && s.End > r.P.LatestEnd.Minute {
				bad("ends %s, latest allowed %s", s.End, r.P.LatestEnd.Minute)
			}
			if len(r.P.AllowedStarts) > 0 {
				ok := false
				for _, c := range r.P.AllowedStarts {
					if c.Minute == s.Start {
						ok = true
					}
				}
				if !ok {
					bad("start %s is not an allowed start time", s.Start)
				}
			}
			if r.P.MinLength.Set && s.Len() < r.P.MinLength.Duration {
				bad("shift %s is shorter than %s", s.Len(), r.P.MinLength.Duration)
			}
			if r.P.MaxLength.Set && s.Len() > r.P.MaxLength.Duration {
				bad("shift %s is longer than %s", s.Len(), r.P.MaxLength.Duration)
			}
		}
	}
	return out
}

// time_slot_count: how many people start (or end) at an exact time.
// This is the building block for staggered "ladders".
type timeSlotCount struct {
	Base
	P struct {
		Scope  Scope `yaml:"scope"`
		DaySel `yaml:",inline"`
		Edge   string `yaml:"edge"` // start | end
		At     Clock  `yaml:"at"`
		Op     string `yaml:"op"` // exactly | at_least | at_most
		N      int    `yaml:"n"`
	}
}

func init() {
	Register("time_slot_count", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &timeSlotCount{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		if r.P.Edge != "start" && r.P.Edge != "end" {
			return nil, fmt.Errorf("edge must be start or end")
		}
		if r.P.Op != "exactly" && r.P.Op != "at_least" && r.P.Op != "at_most" {
			return nil, fmt.Errorf("op must be exactly, at_least or at_most")
		}
		if !r.P.At.Set {
			return nil, fmt.Errorf("missing at")
		}
		if err := r.P.Scope.validate(o); err != nil {
			return nil, err
		}
		return r, r.P.DaySel.compile()
	})
}

func (r *timeSlotCount) Check(ctx *Context) []Violation {
	var out []Violation
	people := r.P.Scope.Select(ctx.Org)
	for _, d := range r.P.DaySel.Select(ctx.Org) {
		n := 0
		for _, p := range people {
			if s, ok := ctx.Roster.Get(p.ID, d); ok {
				t := s.Start
				if r.P.Edge == "end" {
					t = s.End
				}
				if t == r.P.At.Minute {
					n++
				}
			}
		}
		ok := (r.P.Op == "exactly" && n == r.P.N) ||
			(r.P.Op == "at_least" && n >= r.P.N) ||
			(r.P.Op == "at_most" && n <= r.P.N)
		if !ok {
			out = append(out, r.v(slotFormats[r.P.Edge+" "+r.P.Op], n, r.P.At.Minute, r.P.N).at(d))
		}

	}
	return out
}

var slotFormats = map[string]string{
	"start exactly":  "%d people start at %s, want exactly %d",
	"start at_least": "%d people start at %s, want at least %d",
	"start at_most":  "%d people start at %s, want at most %d",
	"end exactly":    "%d people end at %s, want exactly %d",
	"end at_least":   "%d people end at %s, want at least %d",
	"end at_most":    "%d people end at %s, want at most %d",
}
