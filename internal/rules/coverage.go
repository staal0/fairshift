package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

// coverage: at least min (and at most max) people from scope are working
// during a time window. The window may be "opening" (the org calendar).
type coverage struct {
	Base
	P struct {
		Scope  Scope `yaml:"scope"`
		DaySel `yaml:",inline"`
		Window string `yaml:"window"` // "HH:MM-HH:MM" or "opening"
		Min    int    `yaml:"min"`
		Max    int    `yaml:"max"`
	}
	window  domain.Window
	opening bool
}

func init() {
	Register("coverage", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &coverage{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		if err := r.P.Scope.validate(o); err != nil {
			return nil, err
		}
		if err := r.P.DaySel.compile(); err != nil {
			return nil, err
		}
		if r.P.Window == "opening" || r.P.Window == "" {
			r.opening = true
		} else {
			w, err := domain.ParseWindow(r.P.Window)
			if err != nil {
				return nil, err
			}
			r.window = w
		}
		if r.P.Min <= 0 && r.P.Max <= 0 {
			return nil, fmt.Errorf("need min and/or max")
		}
		return r, nil
	})
}

func (r *coverage) Check(ctx *Context) []Violation {
	var out []Violation
	people := r.P.Scope.Select(ctx.Org)
	for _, d := range r.P.DaySel.Select(ctx.Org) {
		w := r.window
		if r.opening {
			ow, ok := ctx.Org.Opening[d.Day]
			if !ok {
				continue // closed that day
			}
			w = ow
		}
		under, over := headcountGaps(ctx.Roster, people, d, w, r.P.Min, r.P.Max)
		if len(under) > 0 {
			out = append(out, r.v("fewer than %d present during %s", r.P.Min, joinWindows(under)).at(d))
		}
		if len(over) > 0 {
			out = append(out, r.v("more than %d present during %s", r.P.Max, joinWindows(over)).at(d))
		}
	}
	return out
}
