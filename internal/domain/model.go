package domain

import (
	"fmt"
	"strings"
)

// Weekday is 0=Monday … 6=Sunday.
type Weekday int

var weekdayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
var weekdayNames = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// AllWeekdays is Monday to Sunday.
var AllWeekdays = []Weekday{0, 1, 2, 3, 4, 5, 6}

// ParseWeekday accepts "mon", "Monday", "MON" …
func ParseWeekday(s string) (Weekday, error) {
	k := strings.ToLower(strings.TrimSpace(s))
	if len(k) >= 3 {
		k = k[:3]
	}
	for i, w := range weekdayKeys {
		if w == k {
			return Weekday(i), nil
		}
	}
	return 0, fmt.Errorf("unknown weekday %q", s)
}

func (d Weekday) Key() string    { return weekdayKeys[d] }
func (d Weekday) String() string { return weekdayNames[d] }
func (d Weekday) Short() string  { return weekdayNames[d][:3] }

// Org is the organisation setup: everything except the roster itself.
type Org struct {
	Name       string
	Lang       string   // UI language: "en" or "da"
	TeamLabel  string   // optional display word for "team", e.g. "Stue"
	Step       Duration // time grid granularity
	Days       []Weekday
	CycleWeeks int
	Opening    map[Weekday]Window
	Teams      []*Team
	People     []*Person
	Activities []*Activity

	teamByID   map[string]*Team
	personByID map[string]*Person
	actByID    map[string]*Activity
	actByCode  map[string]*Activity
}

type Team struct {
	ID    string
	Name  string
	Color string
}

type Person struct {
	ID            string
	Name          string
	Short         string
	Team          string
	Tags          []string
	WeeklyMinutes Duration
	// Fixed shifts are frozen input, the same in every cycle week.
	Fixed map[Weekday]Window
}

func (p *Person) IsFixed() bool { return len(p.Fixed) > 0 }

func (p *Person) HasTag(t string) bool {
	for _, x := range p.Tags {
		if x == t {
			return true
		}
	}
	return false
}

type ActivityKind string

const (
	KindDuty  ActivityKind = "duty"  // held by one person, e.g. opener
	KindTask  ActivityKind = "task"  // inside a shift, e.g. office hour
	KindEvent ActivityKind = "event" // a team together, e.g. meeting
)

// Activity is a duty, task or event. ID is stored; Code and Label are shown.
type Activity struct {
	ID    string
	Code  string
	Label string
	Kind  ActivityKind
}

// Index builds lookup tables. Call after construction.
func (o *Org) Index() error {
	o.teamByID = map[string]*Team{}
	o.personByID = map[string]*Person{}
	o.actByID = map[string]*Activity{}
	o.actByCode = map[string]*Activity{}
	for _, t := range o.Teams {
		if _, dup := o.teamByID[t.ID]; dup {
			return fmt.Errorf("duplicate team id %q", t.ID)
		}
		o.teamByID[t.ID] = t
	}
	for _, p := range o.People {
		if _, dup := o.personByID[p.ID]; dup {
			return fmt.Errorf("duplicate person id %q", p.ID)
		}
		if _, ok := o.teamByID[p.Team]; !ok {
			return fmt.Errorf("person %q: unknown team %q", p.ID, p.Team)
		}
		o.personByID[p.ID] = p
	}
	for _, a := range o.Activities {
		if _, dup := o.actByID[a.ID]; dup {
			return fmt.Errorf("duplicate activity id %q", a.ID)
		}
		if _, dup := o.actByCode[a.Code]; dup && a.Code != "" {
			return fmt.Errorf("duplicate activity code %q", a.Code)
		}
		o.actByID[a.ID] = a
		if a.Code != "" {
			o.actByCode[a.Code] = a
		}
	}
	if o.CycleWeeks < 1 {
		o.CycleWeeks = 1
	}
	if o.Step <= 0 {
		o.Step = 5
	}
	return nil
}

func (o *Org) Team(id string) *Team         { return o.teamByID[id] }
func (o *Org) Person(id string) *Person     { return o.personByID[id] }
func (o *Org) Activity(id string) *Activity { return o.actByID[id] }

// ActivitiesOf returns activities of one kind.
func (o *Org) ActivitiesOf(k ActivityKind) []*Activity {
	var out []*Activity
	for _, a := range o.Activities {
		if a.Kind == k {
			out = append(out, a)
		}
	}
	return out
}

// Members returns the people whose team is teamID, in order.
func (o *Org) Members(teamID string) []*Person {
	var out []*Person
	for _, p := range o.People {
		if p.Team == teamID {
			out = append(out, p)
		}
	}
	return out
}

// Cell addresses one person on one day of the cycle.
type Cell struct {
	Person string
	Week   int // 0-based week in cycle
	Day    Weekday
}

// DayRef addresses one day of the cycle.
type DayRef struct {
	Week int
	Day  Weekday
}

func (d DayRef) String() string { return fmt.Sprintf("week %d %s", d.Week+1, d.Day.Short()) }

// Shift is one person's working time on one day plus activity IDs.
type Shift struct {
	Window
	Acts []string
}

func (s Shift) Has(act string) bool {
	for _, c := range s.Acts {
		if c == act {
			return true
		}
	}
	return false
}

// Roster is a complete plan for one cycle. A missing cell means day off.
type Roster struct {
	Shifts map[Cell]Shift
}

func NewRoster() *Roster { return &Roster{Shifts: map[Cell]Shift{}} }

func (r *Roster) Get(person string, d DayRef) (Shift, bool) {
	s, ok := r.Shifts[Cell{person, d.Week, d.Day}]
	return s, ok
}

// AllDays lists every working day of the cycle in order.
func (o *Org) AllDays() []DayRef {
	var out []DayRef
	for w := 0; w < o.CycleWeeks; w++ {
		for _, d := range o.Days {
			out = append(out, DayRef{w, d})
		}
	}
	return out
}

// ParseCell parses a cell: a time window optionally followed by activity
// codes, either in parentheses "08:00-15:30 (a,b)" or as words "0800 1530 a b".
// Empty, "-" or "off" means off.
func ParseCell(s string) (Shift, bool, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\u2013", "-"))
	if s == "" || s == "-" || strings.EqualFold(s, "off") {
		return Shift{}, false, nil
	}
	var acts, timeParts []string
	if i := strings.Index(s, "("); i >= 0 {
		j := strings.Index(s, ")")
		if j < i {
			return Shift{}, false, fmt.Errorf("invalid cell %q: unbalanced parentheses", s)
		}
		acts = strings.Split(s[i+1:j], ",")
		timeParts = []string{s[:i]}
	} else {
		for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
			if strings.Trim(tok, "0123456789:.-") == "" {
				timeParts = append(timeParts, tok)
			} else {
				acts = append(acts, tok)
			}
		}
	}
	w, err := ParseWindow(strings.Join(timeParts, " "))
	if err != nil {
		return Shift{}, false, err
	}
	var clean []string
	for _, c := range acts {
		if c = strings.TrimSpace(c); c != "" {
			clean = append(clean, c)
		}
	}
	return Shift{Window: w, Acts: clean}, true, nil
}

// FormatCell is the inverse of ParseCell (activity IDs as stored).
func FormatCell(s Shift) string {
	out := s.String()
	if len(s.Acts) > 0 {
		out += " (" + strings.Join(s.Acts, ",") + ")"
	}
	return out
}

// DisplayCell formats a shift with display codes.
func (o *Org) DisplayCell(s Shift) string {
	out := s.String()
	if len(s.Acts) > 0 {
		var codes []string
		for _, id := range s.Acts {
			if a := o.actByID[id]; a != nil && a.Code != "" {
				codes = append(codes, a.Code)
			} else {
				codes = append(codes, id)
			}
		}
		out += " (" + strings.Join(codes, ",") + ")"
	}
	return out
}

// Slug makes a lower-case ASCII identifier from a name, transliterating
// common Nordic and German letters.
func Slug(s string) string {
	r := strings.NewReplacer("\u00e6", "ae", "\u00f8", "oe", "\u00e5", "aa", "\u00c6", "ae", "\u00d8", "oe", "\u00c5", "aa",
		"\u00e4", "a", "\u00f6", "o", "\u00fc", "u", "\u00e9", "e", "\u00e8", "e", "\u00df", "ss")
	s = strings.ToLower(r.Replace(s))
	var b strings.Builder
	dash := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
