package rules

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/staal0/fairshift/internal/domain"
)

// Metric is a predicate on a shift that counts as one "unit" of some kind
// of burden (an early start, a close, a weekend day …).
type Metric struct {
	StartBefore Clock    `yaml:"start_before"`
	StartAfter  Clock    `yaml:"start_after"`
	EndAfter    Clock    `yaml:"end_after"`
	EndBefore   Clock    `yaml:"end_before"`
	Code        string   `yaml:"code"`
	Days        []string `yaml:"days"`
	days        map[domain.Weekday]bool
}

func (m *Metric) compile() error {
	m.days = map[domain.Weekday]bool{}
	for _, s := range m.Days {
		w, err := domain.ParseWeekday(s)
		if err != nil {
			return err
		}
		m.days[w] = true
	}
	if !m.StartBefore.Set && !m.StartAfter.Set && !m.EndAfter.Set && !m.EndBefore.Set && m.Code == "" && len(m.Days) == 0 {
		return fmt.Errorf("metric needs at least one condition")
	}
	return nil
}

// Counts reports whether a worked shift on day d counts for the metric.
func (m *Metric) Counts(s domain.Shift, d domain.DayRef) bool {
	if m.StartBefore.Set && s.Start >= m.StartBefore.Minute {
		return false
	}
	if m.StartAfter.Set && s.Start <= m.StartAfter.Minute {
		return false
	}
	if m.EndAfter.Set && s.End <= m.EndAfter.Minute {
		return false
	}
	if m.EndBefore.Set && s.End >= m.EndBefore.Minute {
		return false
	}
	if m.Code != "" && !s.Has(m.Code) {
		return false
	}
	if len(m.days) > 0 && !m.days[d.Day] {
		return false
	}
	return true
}

// fair_distribution: share a burden evenly. Per-person bounds per period,
// optional max spread between people, optional max spread between teams
// (team totals normalised by the number of people in scope).
type fairDistribution struct {
	Base
	P struct {
		Name      string `yaml:"name"`
		Metric    Metric `yaml:"metric"`
		Scope     Scope  `yaml:"scope"`
		Period    string `yaml:"period"` // week | cycle
		PerPerson struct {
			Min *int `yaml:"min"`
			Max *int `yaml:"max"`
		} `yaml:"per_person"`
		MaxPersonSpread *int     `yaml:"max_person_spread"`
		MaxTeamSpread   *float64 `yaml:"max_team_spread"`
	}
}

func init() {
	Register("fair_distribution", func(b Base, dec Decoder, o *domain.Org) (Rule, error) {
		r := &fairDistribution{Base: b}
		if err := dec(&r.P); err != nil {
			return nil, err
		}
		if r.P.Period == "" {
			r.P.Period = "week"
		}
		if r.P.Period != "week" && r.P.Period != "cycle" {
			return nil, fmt.Errorf("period must be week or cycle")
		}
		if r.P.Name == "" {
			r.P.Name = b.RuleLabel
		}
		if err := r.P.Scope.validate(o); err != nil {
			return nil, err
		}
		return r, r.P.Metric.compile()
	})
}

// FairnessTable is the per-period count per person, exported for analytics.
type FairnessTable struct {
	Metric  string
	Periods []int                // week numbers; 0 means the whole cycle
	People  []string             // person IDs in scope
	Counts  map[string][]int     // person -> count per period
	Teams   map[string][]float64 // team -> average per person per period
}

// Table computes the metric counts for a roster.
func (r *fairDistribution) Table(ctx *Context) FairnessTable {
	o := ctx.Org
	nPer := o.CycleWeeks
	var periods []int
	if r.P.Period == "cycle" {
		nPer = 1
		periods = []int{0}
	} else {
		for w := 1; w <= o.CycleWeeks; w++ {
			periods = append(periods, w)
		}
	}
	t := FairnessTable{Metric: r.P.Name, Periods: periods, Counts: map[string][]int{}, Teams: map[string][]float64{}}
	people := r.P.Scope.Select(o)
	teamN := map[string]int{}
	for _, p := range people {
		t.People = append(t.People, p.ID)
		c := make([]int, nPer)
		for _, d := range o.AllDays() {
			if s, ok := ctx.Roster.Get(p.ID, d); ok && r.P.Metric.Counts(s, d) {
				i := d.Week
				if nPer == 1 {
					i = 0
				}
				c[i]++
			}
		}
		t.Counts[p.ID] = c
		teamN[p.Team]++
		if t.Teams[p.Team] == nil {
			t.Teams[p.Team] = make([]float64, nPer)
		}
		for i := range c {
			t.Teams[p.Team][i] += float64(c[i])
		}
	}
	for team, v := range t.Teams {
		for i := range v {
			v[i] /= float64(teamN[team])
		}
	}
	return t
}

func (r *fairDistribution) Check(ctx *Context) []Violation {
	var out []Violation
	o := ctx.Org
	t := r.Table(ctx)
	for i, per := range t.Periods {
		lo, hi := math.MaxInt, math.MinInt
		for _, pid := range t.People {
			n := t.Counts[pid][i]
			lo, hi = min(lo, n), max(hi, n)
			if r.P.PerPerson.Min != nil && n < *r.P.PerPerson.Min {
				out = append(out, r.inPeriod(per, "%s: %d × %s in week %d, want at least %d", name(o, pid), n, r.P.Name, per, *r.P.PerPerson.Min).who(pid))
			}
			if r.P.PerPerson.Max != nil && n > *r.P.PerPerson.Max {
				out = append(out, r.inPeriod(per, "%s: %d × %s in week %d, want at most %d", name(o, pid), n, r.P.Name, per, *r.P.PerPerson.Max).who(pid))
			}
		}
		if r.P.MaxPersonSpread != nil && len(t.People) > 0 && hi-lo > *r.P.MaxPersonSpread {
			out = append(out, r.inPeriod(per, "%s spread between people is %d in week %d, want at most %d", r.P.Name, hi-lo, per, *r.P.MaxPersonSpread))
		}
		if r.P.MaxTeamSpread != nil && len(t.Teams) > 1 {
			var teams []string
			for k := range t.Teams {
				teams = append(teams, k)
			}
			sort.Strings(teams)
			flo, fhi := math.Inf(1), math.Inf(-1)
			for _, k := range teams {
				flo, fhi = math.Min(flo, t.Teams[k][i]), math.Max(fhi, t.Teams[k][i])
			}
			if fhi-flo > *r.P.MaxTeamSpread+1e-9 {
				out = append(out, r.inPeriod(per, "%s per person differs by %.2f between teams in week %d, want at most %.2f", r.P.Name, fhi-flo, per, *r.P.MaxTeamSpread))
			}
		}
	}
	return out
}

// inPeriod builds a violation; for period 0 "week %d" becomes "the cycle".
func (r *fairDistribution) inPeriod(per int, format string, args ...any) Violation {
	if per != 0 {
		return r.v(format, args...)
	}
	return r.v(strings.Replace(format, "week %d", "the cycle", 1), dropInt(args, per)...)
}

func dropInt(args []any, v int) []any {
	out := make([]any, 0, len(args))
	dropped := false
	for _, a := range args {
		if x, ok := a.(int); ok && x == v && !dropped {
			dropped = true
			continue
		}
		out = append(out, a)
	}
	return out
}

// Tabler is implemented by rules that can show a fairness table.
type Tabler interface {
	Table(ctx *Context) FairnessTable
}
