// Package validate checks a roster against a set of rules. It does not
// depend on the solver, so it can check hand-edited and imported plans.
package validate

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/rules"
)

// Deviation is a documented, accepted exception: violations it matches are
// reported as warnings instead of errors.
type Deviation struct {
	ID     string
	Rule   string
	Person string // optional filters
	Team   string
	Week   int // 1-based; 0 = any
	Day    string
	Reason string
}

func (d Deviation) matches(v rules.Violation) bool {
	if d.Rule != v.Rule {
		return false
	}
	if d.Person != "" && d.Person != v.Person {
		return false
	}
	if d.Team != "" && d.Team != v.Team {
		return false
	}
	if d.Week != 0 && (v.Day == nil || v.Day.Week != d.Week-1) {
		return false
	}
	if d.Day != "" {
		wd, err := domain.ParseWeekday(d.Day)
		if err != nil || v.Day == nil || v.Day.Day != wd {
			return false
		}
	}
	return true
}

// RuleResult is the outcome for one rule.
type RuleResult struct {
	Rule     rules.Rule
	Findings []rules.Violation
}

// Status is the worst level among findings: "pass", "info", "warning", "error".
func (r RuleResult) Status() string {
	worst := -1
	for _, f := range r.Findings {
		worst = max(worst, int(f.Level))
	}
	if worst < 0 {
		return "pass"
	}
	return rules.Level(worst).String()
}

// Report is the full validation outcome.
type Report struct {
	Results    []RuleResult
	Structural []rules.Violation // problems outside any rule (bad codes, grid)
	Unused     []Deviation       // accepted deviations that matched nothing
	Deviations map[string]Deviation
}

// Count returns the number of findings at a level.
func (r *Report) Count(l rules.Level) int {
	n := 0
	for _, f := range r.All() {
		if f.Level == l {
			n++
		}
	}
	return n
}

// All returns every finding, structural first.
func (r *Report) All() []rules.Violation {
	out := append([]rules.Violation(nil), r.Structural...)
	for _, res := range r.Results {
		out = append(out, res.Findings...)
	}
	return out
}

// OK is true when there are no errors.
func (r *Report) OK() bool { return r.Count(rules.LevelError) == 0 }

// Run validates a roster.
func Run(org *domain.Org, rs []rules.Rule, roster *domain.Roster, devs []Deviation) *Report {
	ctx := &rules.Context{Org: org, Roster: roster}
	rep := &Report{Deviations: map[string]Deviation{}}
	used := map[string]bool{}
	for _, d := range devs {
		rep.Deviations[d.ID] = d
	}
	rep.Structural = structural(org, roster)
	for _, r := range rs {
		res := RuleResult{Rule: r}
		for _, v := range r.Check(ctx) {
			if v.Level == rules.LevelError {
				for _, d := range devs {
					if d.matches(v) {
						v.Level = rules.LevelWarning
						v.Deviation = d.ID
						v.Note = d.Reason
						used[d.ID] = true
						break
					}
				}
			}
			res.Findings = append(res.Findings, v)
		}
		rep.Results = append(rep.Results, res)
	}
	for _, d := range devs {
		if !used[d.ID] {
			rep.Unused = append(rep.Unused, d)
		}
	}
	return rep
}

func structural(org *domain.Org, roster *domain.Roster) []rules.Violation {
	var out []rules.Violation
	cells := make([]domain.Cell, 0, len(roster.Shifts))
	for c := range roster.Shifts {
		cells = append(cells, c)
	}
	sort.Slice(cells, func(i, j int) bool {
		a, b := cells[i], cells[j]
		if a.Week != b.Week {
			return a.Week < b.Week
		}
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		return a.Person < b.Person
	})
	for _, c := range cells {
		s := roster.Shifts[c]
		d := domain.DayRef{Week: c.Week, Day: c.Day}
		bad := func(f string, a ...any) {
			out = append(out, rules.Violation{Rule: "structure", Level: rules.LevelError, Message: fmt.Sprintf(f, a...), Format: f, Args: a, Person: c.Person, Day: &d})
		}
		if org.Person(c.Person) == nil {
			bad("unknown person %q", c.Person)
			continue
		}
		for _, act := range s.Acts {
			if org.Activity(act) == nil {
				bad("%s: unknown activity %q", c.Person, act)
			}
		}
		if org.Step > 0 && (int(s.Start)%int(org.Step) != 0 || int(s.End)%int(org.Step) != 0) {
			bad("%s: %s is not on the %d-minute grid", c.Person, s.Window, org.Step)
		}
		if s.End <= s.Start {
			bad("%s: shift %s has no length", c.Person, s.Window)
		}
	}
	return out
}

// Print writes a human-readable report.
func (r *Report) Print(w io.Writer, org *domain.Org, verbose bool) {
	mark := map[string]string{"pass": "PASS", "info": "INFO", "warning": "WARN", "error": "FAIL"}
	if len(r.Structural) > 0 {
		fmt.Fprintf(w, "FAIL  structure (%d)\n", len(r.Structural))
		for _, v := range r.Structural {
			fmt.Fprintf(w, "        %s\n", line(v))
		}
	}
	for _, res := range r.Results {
		st := res.Status()
		fmt.Fprintf(w, "%-4s  %-28s %s\n", mark[st], res.Rule.ID(), res.Rule.Label())
		for _, f := range res.Findings {
			if f.Level == rules.LevelInfo && !verbose {
				continue
			}
			fmt.Fprintf(w, "        %-7s %s\n", f.Level, line(f))
		}
	}
	for _, d := range r.Unused {
		fmt.Fprintf(w, "INFO  accepted deviation %q matched nothing – it may be stale\n", d.ID)
	}
	fmt.Fprintf(w, "\n%d error(s), %d warning(s), %d info\n",
		r.Count(rules.LevelError), r.Count(rules.LevelWarning), r.Count(rules.LevelInfo))
}

func line(v rules.Violation) string {
	var b strings.Builder
	if v.Day != nil {
		b.WriteString(v.Day.String())
		b.WriteString(": ")
	}
	b.WriteString(v.Message)
	if v.Note != "" {
		b.WriteString(" – " + v.Note)
	}
	return b.String()
}
