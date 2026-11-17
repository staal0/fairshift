package rules

import "github.com/staal0/fairshift/internal/domain"

// Limit says at most Max of Cells may carry activity Act. With Days set,
// it counts distinct days instead of cells. Cells are "person|day" keys.
type Limit struct {
	Act   string   `json:"act"`
	Cells []string `json:"cells"`
	Max   int      `json:"max"`
	Days  bool     `json:"days,omitempty"`
}

// Allowed says that on Day only People may carry Act.
type Allowed struct {
	Act    string   `json:"act"`
	Day    string   `json:"day"`
	People []string `json:"people"`
}

// Fit says the shift in Cells must match these times to carry Act, or with
// Apply, that choosing Act sets the shift to them. Times are minutes; -1 = any.
type Fit struct {
	Act    string   `json:"act"`
	Cells  []string `json:"cells"`
	Start  int      `json:"start"`
	End    int      `json:"end"`
	Length int      `json:"length"`
	Covers []int    `json:"covers,omitempty"` // [start, end] the shift must cover
	Apply  bool     `json:"apply,omitempty"`  // choosing Act sets the shift to these times
}

// Limits is everything the editor needs to offer only possible options.
type Limits struct {
	Limits  []Limit           `json:"limits"`
	Allowed []Allowed         `json:"allowed"`
	Fits    []Fit             `json:"fits"`
	Open    map[string][2]int `json:"open"` // opening hours per weekday key
}

// Limiter is implemented by rules that restrict where activities can go.
type Limiter interface {
	AddLimits(o *domain.Org, week int, l *Limits)
}

func cellKey(person string, d domain.Weekday) string { return person + "|" + d.Key() }

func (r *duty) AddLimits(o *domain.Org, week int, l *Limits) {
	for _, d := range o.Days {
		if _, scheduled := r.sched[d]; !scheduled {
			l.Allowed = append(l.Allowed, Allowed{Act: r.code, Day: d.Key(), People: []string{}})
			continue
		}
		var cells []string
		for _, p := range o.People {
			cells = append(cells, cellKey(p.ID, d))
		}
		l.Limits = append(l.Limits, Limit{Act: r.code, Cells: cells, Max: 1})
		ref := domain.DayRef{Week: week, Day: d}
		// Who holds the duty is checked by the validator, not limited here,
		// so a duty can be moved. Choosing it applies its shift shape.
		fit := Fit{Act: r.code, Cells: cells, Start: -1, End: -1, Length: -1, Apply: true}
		if t, ok := r.resolve(r.P.Shape.Start, ref, o, false); ok {
			fit.Start = int(t)
		}
		if t, ok := r.resolve(r.P.Shape.End, ref, o, true); ok {
			fit.End = int(t)
		}
		if r.P.Shape.Length.Set {
			fit.Length = int(r.P.Shape.Length.Duration)
		}
		l.Fits = append(l.Fits, fit)
	}
}

func (r *inShiftTask) AddLimits(o *domain.Org, week int, l *Limits) {
	var limits []Limit
	people := r.P.Scope.Select(o)
	for _, p := range people {
		win := r.windowFor(p.Team)
		var cells []string
		for _, d := range o.Days {
			cells = append(cells, cellKey(p.ID, d))
		}
		l.Fits = append(l.Fits, Fit{Act: r.code, Cells: cells, Start: -1, End: -1, Length: -1, Covers: []int{int(win.Start), int(win.End)}})
	}
	if r.P.PerWeek != nil {
		for _, p := range people {
			var cells []string
			for _, d := range o.Days {
				cells = append(cells, cellKey(p.ID, d))
			}
			limits = append(limits, Limit{Act: r.code, Cells: cells, Max: *r.P.PerWeek})
		}
	}
	if r.P.DistinctDaysWithinTeam {
		for _, t := range o.Teams {
			for _, d := range o.Days {
				var cells []string
				for _, p := range o.Members(t.ID) {
					if r.P.Scope.Matches(p) {
						cells = append(cells, cellKey(p.ID, d))
					}
				}
				if len(cells) > 1 {
					limits = append(limits, Limit{Act: r.code, Cells: cells, Max: 1})
				}
			}
		}
	}
	l.Limits = append(l.Limits, limits...)
}

func (r *teamEvent) AddLimits(o *domain.Org, week int, l *Limits) {
	win := r.P.Window.Window
	for _, tid := range r.P.Teams {
		var cells, mustCover []string
		for _, p := range o.Members(tid) {
			for _, d := range o.Days {
				cells = append(cells, cellKey(p.ID, d))
				if !(r.P.ExemptFixed && p.IsFixed()) {
					mustCover = append(mustCover, cellKey(p.ID, d))
				}
			}
		}
		l.Limits = append(l.Limits, Limit{Act: r.code, Cells: cells, Max: r.P.PerWeek, Days: true})
		l.Fits = append(l.Fits, Fit{Act: r.code, Cells: mustCover, Start: -1, End: -1, Length: -1, Covers: []int{int(win.Start), int(win.End)}})
	}
}
