package rules

import (
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

// Scope selects people. Empty fields mean "everyone".
type Scope struct {
	Teams        []string `yaml:"teams"`
	People       []string `yaml:"people"`
	Tags         []string `yaml:"tags"`
	Exclude      []string `yaml:"exclude"`       // person IDs
	ExcludeFixed bool     `yaml:"exclude_fixed"` // skip people with frozen shifts
}

func (s Scope) validate(o *domain.Org) error {
	for _, t := range s.Teams {
		if o.Team(t) == nil {
			return fmt.Errorf("scope: unknown team %q", t)
		}
	}
	for _, list := range [][]string{s.People, s.Exclude} {
		for _, p := range list {
			if o.Person(p) == nil {
				return fmt.Errorf("scope: unknown person %q", p)
			}
		}
	}
	return nil
}

func (s Scope) Matches(p *domain.Person) bool {
	if s.ExcludeFixed && p.IsFixed() {
		return false
	}
	if contains(s.Exclude, p.ID) {
		return false
	}
	if len(s.Teams) > 0 && !contains(s.Teams, p.Team) {
		return false
	}
	if len(s.People) > 0 && !contains(s.People, p.ID) {
		return false
	}
	for _, t := range s.Tags {
		if !p.HasTag(t) {
			return false
		}
	}
	return true
}

func (s Scope) Select(o *domain.Org) []*domain.Person {
	var out []*domain.Person
	for _, p := range o.People {
		if s.Matches(p) {
			out = append(out, p)
		}
	}
	return out
}

// DaySel selects days of the cycle. Empty fields mean "all".
type DaySel struct {
	Days  []string `yaml:"days"`  // weekday keys: mon, tue …
	Weeks []int    `yaml:"weeks"` // 1-based cycle weeks
	days  map[domain.Weekday]bool
}

func (d *DaySel) compile() error {
	d.days = map[domain.Weekday]bool{}
	for _, s := range d.Days {
		w, err := domain.ParseWeekday(s)
		if err != nil {
			return err
		}
		d.days[w] = true
	}
	return nil
}

func (d DaySel) Matches(r domain.DayRef) bool {
	if len(d.days) > 0 && !d.days[r.Day] {
		return false
	}
	if len(d.Weeks) > 0 {
		ok := false
		for _, w := range d.Weeks {
			if w-1 == r.Week {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (d DaySel) Select(o *domain.Org) []domain.DayRef {
	var out []domain.DayRef
	for _, r := range o.AllDays() {
		if d.Matches(r) {
			out = append(out, r)
		}
	}
	return out
}

// Clock is an "HH:MM" time in params.
type Clock struct {
	domain.Minute
	Set bool
}

func (c *Clock) UnmarshalText(b []byte) error {
	m, err := domain.ParseClock(string(b))
	if err != nil {
		return err
	}
	c.Minute, c.Set = m, true
	return nil
}

// Span is an "HH:MM-HH:MM" window in params.
type Span struct {
	domain.Window
	Set bool
}

func (s *Span) UnmarshalText(b []byte) error {
	w, err := domain.ParseWindow(string(b))
	if err != nil {
		return err
	}
	s.Window, s.Set = w, true
	return nil
}

// DurationParam is an "H:MM" duration in params.
type DurationParam struct {
	domain.Duration
	Set bool
}

func (d *DurationParam) UnmarshalText(b []byte) error {
	v, err := domain.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration, d.Set = v, true
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// headcountGaps returns the sub-intervals of w where fewer than min (or more
// than max, if max > 0) of the given people are working.
func headcountGaps(r *domain.Roster, people []*domain.Person, d domain.DayRef, w domain.Window, min, max int) (under, over []domain.Window) {
	// Collect breakpoints.
	pts := map[domain.Minute]bool{w.Start: true, w.End: true}
	var shifts []domain.Window
	for _, p := range people {
		if s, ok := r.Get(p.ID, d); ok {
			shifts = append(shifts, s.Window)
			if w.Contains(s.Start) {
				pts[s.Start] = true
			}
			if w.Contains(s.End) {
				pts[s.End] = true
			}
		}
	}
	var sorted []domain.Minute
	for t := range pts {
		sorted = append(sorted, t)
	}
	sortMinutes(sorted)
	add := func(list []domain.Window, seg domain.Window) []domain.Window {
		if n := len(list); n > 0 && list[n-1].End == seg.Start {
			list[n-1].End = seg.End
			return list
		}
		return append(list, seg)
	}
	for i := 0; i+1 < len(sorted); i++ {
		seg := domain.Window{Start: sorted[i], End: sorted[i+1]}
		n := 0
		for _, s := range shifts {
			if s.Covers(seg) {
				n++
			}
		}
		if n < min {
			under = add(under, seg)
		}
		if max > 0 && n > max {
			over = add(over, seg)
		}
	}
	return under, over
}

func sortMinutes(a []domain.Minute) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func joinWindows(ws []domain.Window) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += ", "
		}
		out += w.String()
	}
	return out
}

func name(o *domain.Org, id string) string {
	if p := o.Person(id); p != nil {
		if p.Short != "" {
			return p.Short
		}
		return p.Name
	}
	return id
}
