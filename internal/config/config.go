// Package config reads YAML fixtures: an organisation setup and a plan.
// Used by tests and to seed a demo database.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/validate"
	"gopkg.in/yaml.v3"
)

// Setup is an organisation with its rules.
type Setup struct {
	Org        *domain.Org
	Rules      []rules.Rule
	RuleDefs   []RuleDef
	Deviations []validate.Deviation
}

// RuleDef is a rule as stored: params are kept as JSON-compatible data.
type RuleDef struct {
	ID       string         `yaml:"id"`
	Type     string         `yaml:"type"`
	Label    string         `yaml:"label"`
	Severity string         `yaml:"severity"`
	Enabled  *bool          `yaml:"enabled"`
	Params   map[string]any `yaml:"params"`
}

type file struct {
	Organization struct {
		Name       string            `yaml:"name"`
		Lang       string            `yaml:"lang"`
		TeamLabel  string            `yaml:"team_label"`
		TimeStep   int               `yaml:"time_step"`
		Days       []string          `yaml:"days"`
		CycleWeeks int               `yaml:"cycle_weeks"`
		Opening    map[string]string `yaml:"opening"`
	} `yaml:"organization"`
	Teams []struct {
		ID    string `yaml:"id"`
		Name  string `yaml:"name"`
		Color string `yaml:"color"`
	} `yaml:"teams"`
	People []struct {
		ID          string            `yaml:"id"`
		Name        string            `yaml:"name"`
		Short       string            `yaml:"short"`
		Team        string            `yaml:"team"`
		Tags        []string          `yaml:"tags"`
		WeeklyHours string            `yaml:"weekly_hours"`
		Fixed       map[string]string `yaml:"fixed"`
	} `yaml:"people"`
	Activities []struct {
		ID    string `yaml:"id"`
		Kind  string `yaml:"kind"`
		Code  string `yaml:"code"`
		Label string `yaml:"label"`
	} `yaml:"activities"`
	Rules      []RuleDef `yaml:"rules"`
	Deviations []struct {
		ID     string `yaml:"id"`
		Rule   string `yaml:"rule"`
		Person string `yaml:"person"`
		Team   string `yaml:"team"`
		Week   int    `yaml:"week"`
		Day    string `yaml:"day"`
		Reason string `yaml:"reason"`
	} `yaml:"deviations"`
}

// StrictDecode decodes YAML (or JSON) and rejects unknown fields.
func StrictDecode(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// BuildRule turns a stored rule definition into a checkable rule.
func BuildRule(d RuleDef, o *domain.Org) (rules.Rule, error) {
	decode := func(v any) error {
		if len(d.Params) == 0 {
			return nil
		}
		raw, err := yaml.Marshal(d.Params)
		if err != nil {
			return err
		}
		return StrictDecode(raw, v)
	}
	label := d.Label
	if label == "" {
		label = d.ID
	}
	return rules.New(rules.Base{RuleID: d.ID, RuleType: d.Type, RuleLabel: label, Sev: rules.Severity(d.Severity)}, decode, o)
}

func LoadOrg(path string) (*Setup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := StrictDecode(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	fo := f.Organization
	o := &domain.Org{Name: fo.Name, Lang: fo.Lang, TeamLabel: fo.TeamLabel, Step: domain.Duration(fo.TimeStep),
		CycleWeeks: fo.CycleWeeks, Opening: map[domain.Weekday]domain.Window{}}
	for _, d := range fo.Days {
		wd, err := domain.ParseWeekday(d)
		if err != nil {
			return nil, err
		}
		o.Days = append(o.Days, wd)
	}
	for k, v := range fo.Opening {
		wd, err := domain.ParseWeekday(k)
		if err != nil {
			return nil, err
		}
		w, err := domain.ParseWindow(v)
		if err != nil {
			return nil, err
		}
		o.Opening[wd] = w
	}
	for _, t := range f.Teams {
		o.Teams = append(o.Teams, &domain.Team{ID: t.ID, Name: t.Name, Color: t.Color})
	}
	for _, p := range f.People {
		pp := &domain.Person{ID: p.ID, Name: p.Name, Short: p.Short, Team: p.Team, Tags: p.Tags}
		if p.WeeklyHours != "" {
			if pp.WeeklyMinutes, err = domain.ParseDuration(p.WeeklyHours); err != nil {
				return nil, fmt.Errorf("person %s: %w", p.ID, err)
			}
		}
		for k, v := range p.Fixed {
			wd, err := domain.ParseWeekday(k)
			if err != nil {
				return nil, err
			}
			w, err := domain.ParseWindow(v)
			if err != nil {
				return nil, err
			}
			if pp.Fixed == nil {
				pp.Fixed = map[domain.Weekday]domain.Window{}
			}
			pp.Fixed[wd] = w
		}
		o.People = append(o.People, pp)
	}
	for _, a := range f.Activities {
		o.Activities = append(o.Activities, &domain.Activity{ID: a.ID, Code: a.Code, Label: a.Label, Kind: domain.ActivityKind(a.Kind)})
	}
	if err := o.Index(); err != nil {
		return nil, err
	}
	s := &Setup{Org: o}
	for _, d := range f.Rules {
		s.RuleDefs = append(s.RuleDefs, d)
		if d.Enabled != nil && !*d.Enabled {
			continue
		}
		r, err := BuildRule(d, o)
		if err != nil {
			return nil, err
		}
		s.Rules = append(s.Rules, r)
	}
	for _, d := range f.Deviations {
		s.Deviations = append(s.Deviations, validate.Deviation{ID: d.ID, Rule: d.Rule, Person: d.Person,
			Team: d.Team, Week: d.Week, Day: d.Day, Reason: d.Reason})
	}
	return s, nil
}

// LoadRoster reads a plan: one map per cycle week from person ID to cells.
func LoadRoster(path string, o *domain.Org) (*domain.Roster, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var f struct {
		Plan struct {
			Name  string                `yaml:"name"`
			Weeks []map[string][]string `yaml:"weeks"`
		} `yaml:"plan"`
	}
	if err := StrictDecode(data, &f); err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	r := domain.NewRoster()
	for w, week := range f.Plan.Weeks {
		for pid, cells := range week {
			if o.Person(pid) == nil {
				return nil, "", fmt.Errorf("%s: unknown person %q", path, pid)
			}
			for i, c := range cells {
				if i >= len(o.Days) {
					break
				}
				s, works, err := domain.ParseCell(c)
				if err != nil {
					return nil, "", fmt.Errorf("%s: %s: %w", path, pid, err)
				}
				if works {
					r.Shifts[domain.Cell{Person: pid, Week: w, Day: o.Days[i]}] = s
				}
			}
		}
	}
	return r, f.Plan.Name, nil
}
