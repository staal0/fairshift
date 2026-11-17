// Package analytics computes the numbers and charts shown to planners.
package analytics

import (
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/rules"
)

// HoursRow is planned vs contract hours for one person.
type HoursRow struct {
	Person   *domain.Person
	Planned  []domain.Duration // per cycle week
	Contract domain.Duration
	Diff     []domain.Duration
	Total    domain.Duration // cycle total diff
}

func Hours(o *domain.Org, r *domain.Roster) []HoursRow {
	var out []HoursRow
	for _, p := range o.People {
		row := HoursRow{Person: p, Planned: rules.Planned(o, r, p.ID), Contract: p.WeeklyMinutes}
		for _, x := range row.Planned {
			d := x - p.WeeklyMinutes
			row.Diff = append(row.Diff, d)
			row.Total += d
		}
		out = append(out, row)
	}
	return out
}

// Fairness returns a table per fairness rule (deduplicated by metric name).
func Fairness(o *domain.Org, rs []rules.Rule, ro *domain.Roster) []rules.FairnessTable {
	ctx := &rules.Context{Org: o, Roster: ro}
	seen := map[string]bool{}
	var out []rules.FairnessTable
	for _, r := range rs {
		if t, ok := r.(rules.Tabler); ok {
			tab := t.Table(ctx)
			if !seen[tab.Metric] {
				seen[tab.Metric] = true
				out = append(out, tab)
			}
		}
	}
	return out
}

// Coverage is a headcount grid for one day: rows are teams (+ total),
// columns are time slots.
type Coverage struct {
	Day   domain.DayRef
	Slots []domain.Minute
	Rows  []CoverageRow
	Open  []bool // slot is inside opening hours
}

type CoverageRow struct {
	Label  string
	Color  string
	Counts []int
	Max    int
}

// CoverageGrid computes headcount per team per slot for every day of a week.
func CoverageGrid(o *domain.Org, ro *domain.Roster, week int, slot domain.Duration) []Coverage {
	lo, hi := domain.Minute(24*60), domain.Minute(0)
	for _, w := range o.Opening {
		lo, hi = min(lo, w.Start), max(hi, w.End)
	}
	for c, s := range ro.Shifts {
		if c.Week == week {
			lo, hi = min(lo, s.Start), max(hi, s.End)
		}
	}
	if hi <= lo {
		return nil
	}
	lo -= lo % domain.Minute(slot)
	var out []Coverage
	for _, day := range o.Days {
		d := domain.DayRef{Week: week, Day: day}
		cv := Coverage{Day: d}
		open, isOpen := o.Opening[day]
		for t := lo; t < hi; t += domain.Minute(slot) {
			cv.Slots = append(cv.Slots, t)
			cv.Open = append(cv.Open, isOpen && open.Contains(t))
		}
		total := CoverageRow{Label: "Total", Counts: make([]int, len(cv.Slots))}
		for _, t := range o.Teams {
			row := CoverageRow{Label: t.Name, Color: t.Color, Counts: make([]int, len(cv.Slots))}
			for _, p := range o.Members(t.ID) {
				s, ok := ro.Get(p.ID, d)
				if !ok {
					continue
				}
				for i, ts := range cv.Slots {
					if s.Covers(domain.Window{Start: ts, End: ts + domain.Minute(slot)}) {
						row.Counts[i]++
						total.Counts[i]++
					}
				}
			}
			for _, n := range row.Counts {
				row.Max = max(row.Max, n)
			}
			cv.Rows = append(cv.Rows, row)
		}
		for _, n := range total.Counts {
			total.Max = max(total.Max, n)
		}
		cv.Rows = append(cv.Rows, total)
		out = append(out, cv)
	}
	return out
}
