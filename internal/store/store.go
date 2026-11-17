// Package store persists the organisation and its plans in MySQL.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/staal0/fairshift/internal/config"
	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/validate"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ db *sql.DB }

func Open(dsn string) (*Store, error) {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("mysql", dsn+sep+"parseTime=true&multiStatements=true")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Migrate applies pending migrations in file name order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, e.Name()).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, _ := migrations.ReadFile("migrations/" + e.Name())
		if _, err := s.db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?)`, e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// ---- settings ----

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k, v FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetSettings(ctx context.Context, kv map[string]string) error {
	for k, v := range kv {
		if _, err := s.db.ExecContext(ctx, `REPLACE INTO settings (k, v) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	return nil
}

// ---- teams, people, activities ----

func (s *Store) SaveTeam(ctx context.Context, t domain.Team, sortKey int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO teams (id, name, color, sort) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name = VALUES(name), color = VALUES(color), sort = VALUES(sort)`, t.ID, t.Name, t.Color, sortKey)
	return err
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM teams WHERE id = ?`, id)
	return err
}

func (s *Store) SavePerson(ctx context.Context, p domain.Person, sortKey int) error {
	fixed := map[string]string{}
	for d, w := range p.Fixed {
		fixed[d.Key()] = w.String()
	}
	fj, _ := json.Marshal(fixed)
	_, err := s.db.ExecContext(ctx, `INSERT INTO people (id, name, short_name, team_id, weekly_minutes, tags, fixed_json, sort)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE name = VALUES(name), short_name = VALUES(short_name), team_id = VALUES(team_id),
		weekly_minutes = VALUES(weekly_minutes), tags = VALUES(tags), fixed_json = VALUES(fixed_json), sort = VALUES(sort)`,
		p.ID, p.Name, p.Short, p.Team, int(p.WeeklyMinutes), strings.Join(p.Tags, ","), string(fj), sortKey)
	return err
}

func (s *Store) DeletePerson(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plan_cells WHERE person_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM people WHERE id = ?`, id)
	return err
}

func (s *Store) SaveActivity(ctx context.Context, a domain.Activity, sortKey int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO activities (id, kind, code, label, sort) VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE kind = VALUES(kind), code = VALUES(code), label = VALUES(label), sort = VALUES(sort)`,
		a.ID, string(a.Kind), a.Code, a.Label, sortKey)
	return err
}

func (s *Store) DeleteActivity(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM activities WHERE id = ?`, id)
	return err
}

// ---- rules and deviations ----

// RuleRow is a stored rule.
type RuleRow struct {
	ID       int64
	Type     string
	Label    string
	Severity string
	Enabled  bool
	Params   map[string]any
}

func (r RuleRow) Key() string { return strconv.FormatInt(r.ID, 10) }

func (r RuleRow) Def() config.RuleDef {
	return config.RuleDef{ID: r.Key(), Type: r.Type, Label: r.Label, Severity: r.Severity, Params: r.Params}
}

func (s *Store) Rules(ctx context.Context) ([]RuleRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, type, label, severity, enabled, params_json FROM rules ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleRow
	for rows.Next() {
		var r RuleRow
		var pj string
		if err := rows.Scan(&r.ID, &r.Type, &r.Label, &r.Severity, &r.Enabled, &pj); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pj), &r.Params); err != nil {
			return nil, fmt.Errorf("rule %d: %w", r.ID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SaveRule inserts (ID 0) or updates a rule and returns its ID.
func (s *Store) SaveRule(ctx context.Context, r RuleRow) (int64, error) {
	if r.Params == nil {
		r.Params = map[string]any{}
	}
	pj, err := json.Marshal(r.Params)
	if err != nil {
		return 0, err
	}
	if r.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO rules (type, label, severity, enabled, params_json, sort)
			SELECT ?, ?, ?, ?, ?, COALESCE(MAX(sort), 0) + 1 FROM rules`, r.Type, r.Label, r.Severity, r.Enabled, string(pj))
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err = s.db.ExecContext(ctx, `UPDATE rules SET type = ?, label = ?, severity = ?, enabled = ?, params_json = ? WHERE id = ?`,
		r.Type, r.Label, r.Severity, r.Enabled, string(pj), r.ID)
	return r.ID, err
}

func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	return err
}

func (s *Store) Deviations(ctx context.Context) ([]validate.Deviation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, rule_id, person_id, team_id, week, day, reason FROM deviations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []validate.Deviation
	for rows.Next() {
		var d validate.Deviation
		var id, rule int64
		if err := rows.Scan(&id, &rule, &d.Person, &d.Team, &d.Week, &d.Day, &d.Reason); err != nil {
			return nil, err
		}
		d.ID, d.Rule = strconv.FormatInt(id, 10), strconv.FormatInt(rule, 10)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AddDeviation(ctx context.Context, d validate.Deviation) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO deviations (rule_id, person_id, team_id, week, day, reason) VALUES (?, ?, ?, ?, ?, ?)`,
		d.Rule, d.Person, d.Team, d.Week, d.Day, d.Reason)
	return err
}

func (s *Store) DeleteDeviation(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM deviations WHERE id = ?`, id)
	return err
}

// ---- the whole setup ----

// Setup is the organisation, its rules and deviations as loaded from the DB.
type Setup struct {
	Org        *domain.Org
	Rules      []rules.Rule
	RuleRows   []RuleRow
	RuleErrors map[int64]error // rules that no longer build (e.g. a deleted team)
	Deviations []validate.Deviation
}

func (s *Store) LoadSetup(ctx context.Context) (*Setup, error) {
	set, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	o := &domain.Org{Name: set["name"], Lang: set["lang"], TeamLabel: set["team_label"], Opening: map[domain.Weekday]domain.Window{}}
	if o.Lang == "" {
		o.Lang = "en"
	}
	o.CycleWeeks, _ = strconv.Atoi(set["cycle_weeks"])
	step, _ := strconv.Atoi(set["time_step"])
	o.Step = domain.Duration(step)
	days := set["days"]
	if days == "" {
		days = "mon,tue,wed,thu,fri"
	}
	for _, d := range strings.Split(days, ",") {
		if wd, err := domain.ParseWeekday(d); err == nil {
			o.Days = append(o.Days, wd)
		}
	}
	for _, d := range domain.AllWeekdays {
		if w, err := domain.ParseWindow(set["opening_"+d.Key()]); err == nil {
			o.Opening[d] = w
		}
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color FROM teams ORDER BY sort, name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		t := &domain.Team{}
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			rows.Close()
			return nil, err
		}
		o.Teams = append(o.Teams, t)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT p.id, p.name, p.short_name, p.team_id, p.weekly_minutes, p.tags, p.fixed_json
		FROM people p JOIN teams t ON t.id = p.team_id ORDER BY t.sort, t.name, p.sort, p.name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		p := &domain.Person{}
		var mins int
		var tags, fj string
		if err := rows.Scan(&p.ID, &p.Name, &p.Short, &p.Team, &mins, &tags, &fj); err != nil {
			rows.Close()
			return nil, err
		}
		p.WeeklyMinutes = domain.Duration(mins)
		if tags != "" {
			p.Tags = strings.Split(tags, ",")
		}
		var fixed map[string]string
		_ = json.Unmarshal([]byte(fj), &fixed)
		for k, v := range fixed {
			wd, err1 := domain.ParseWeekday(k)
			w, err2 := domain.ParseWindow(v)
			if err1 == nil && err2 == nil {
				if p.Fixed == nil {
					p.Fixed = map[domain.Weekday]domain.Window{}
				}
				p.Fixed[wd] = w
			}
		}
		if p.Short == "" {
			p.Short = strings.Fields(p.Name + " ?")[0]
		}
		o.People = append(o.People, p)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT id, kind, code, label FROM activities ORDER BY sort, label`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		a := &domain.Activity{}
		if err := rows.Scan(&a.ID, &a.Kind, &a.Code, &a.Label); err != nil {
			rows.Close()
			return nil, err
		}
		o.Activities = append(o.Activities, a)
	}
	rows.Close()
	if err := o.Index(); err != nil {
		return nil, err
	}

	setup := &Setup{Org: o, RuleErrors: map[int64]error{}}
	if setup.RuleRows, err = s.Rules(ctx); err != nil {
		return nil, err
	}
	for _, rr := range setup.RuleRows {
		if !rr.Enabled {
			continue
		}
		r, err := config.BuildRule(rr.Def(), o)
		if err != nil {
			setup.RuleErrors[rr.ID] = err
			continue
		}
		setup.Rules = append(setup.Rules, r)
	}
	setup.Deviations, err = s.Deviations(ctx)
	return setup, err
}

// Reset deletes all data (used by seeding).
func (s *Store) Reset(ctx context.Context) error {
	for _, t := range []string{"plan_cells", "plans", "deviations", "rules", "activities", "people", "teams", "settings"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	return nil
}

// Empty reports whether no organisation has been set up yet.
func (s *Store) Empty(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM teams) + (SELECT COUNT(*) FROM settings)`).Scan(&n)
	return n == 0, err
}

// ---- plans ----

type PlanRow struct {
	ID   int64
	Name string
}

func (s *Store) Plans(ctx context.Context) ([]PlanRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM plans ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanRow
	for rows.Next() {
		var p PlanRow
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Plan(ctx context.Context, id int64) (*PlanRow, error) {
	var p PlanRow
	if err := s.db.QueryRowContext(ctx, `SELECT id, name FROM plans WHERE id = ?`, id).Scan(&p.ID, &p.Name); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) RenamePlan(ctx context.Context, id int64, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE plans SET name = ? WHERE id = ?`, name, id)
	return err
}

// CreatePlan stores a new plan with all its cells.
func (s *Store) CreatePlan(ctx context.Context, name string, r *domain.Roster) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO plans (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	for c, sh := range r.Shifts {
		if err := putCell(ctx, tx, id, c, sh); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

func (s *Store) DeletePlan(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plans WHERE id = ?`, id)
	return err
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func putCell(ctx context.Context, db execer, plan int64, c domain.Cell, sh domain.Shift) error {
	_, err := db.ExecContext(ctx, `REPLACE INTO plan_cells (plan_id, person_id, week, weekday, start_min, end_min, acts)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, plan, c.Person, c.Week, int(c.Day), int(sh.Start), int(sh.End), strings.Join(sh.Acts, ","))
	return err
}

// SetCell updates one cell; works=false clears it (day off).
func (s *Store) SetCell(ctx context.Context, plan int64, c domain.Cell, sh domain.Shift, works bool) error {
	if !works {
		_, err := s.db.ExecContext(ctx, `DELETE FROM plan_cells WHERE plan_id = ? AND person_id = ? AND week = ? AND weekday = ?`,
			plan, c.Person, c.Week, int(c.Day))
		return err
	}
	return putCell(ctx, s.db, plan, c, sh)
}

func (s *Store) Roster(ctx context.Context, plan int64) (*domain.Roster, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT person_id, week, weekday, start_min, end_min, acts FROM plan_cells WHERE plan_id = ?`, plan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r := domain.NewRoster()
	for rows.Next() {
		var c domain.Cell
		var day, start, end int
		var acts string
		if err := rows.Scan(&c.Person, &c.Week, &day, &start, &end, &acts); err != nil {
			return nil, err
		}
		c.Day = domain.Weekday(day)
		sh := domain.Shift{Window: domain.Window{Start: domain.Minute(start), End: domain.Minute(end)}}
		if acts != "" {
			sh.Acts = strings.Split(acts, ",")
		}
		r.Shifts[c] = sh
	}
	return r, rows.Err()
}

// Seed replaces all data with a YAML setup and optional plan.
func (s *Store) Seed(ctx context.Context, cs *config.Setup, plan *domain.Roster, planName string) error {
	if err := s.Reset(ctx); err != nil {
		return err
	}
	o := cs.Org
	var days []string
	for _, d := range o.Days {
		days = append(days, d.Key())
	}
	set := map[string]string{"name": o.Name, "lang": o.Lang, "team_label": o.TeamLabel,
		"cycle_weeks": strconv.Itoa(o.CycleWeeks), "time_step": strconv.Itoa(int(o.Step)), "days": strings.Join(days, ",")}
	for d, w := range o.Opening {
		set["opening_"+d.Key()] = w.String()
	}
	if err := s.SetSettings(ctx, set); err != nil {
		return err
	}
	for i, t := range o.Teams {
		if err := s.SaveTeam(ctx, *t, i); err != nil {
			return err
		}
	}
	for i, p := range o.People {
		if err := s.SavePerson(ctx, *p, i); err != nil {
			return err
		}
	}
	for i, a := range o.Activities {
		if err := s.SaveActivity(ctx, *a, i); err != nil {
			return err
		}
	}
	ids := map[string]int64{}
	for _, d := range cs.RuleDefs {
		params := map[string]any{}
		for k, v := range d.Params {
			params[k] = v
		}
		// Round-trip through JSON so params are stored in their canonical form.
		raw, _ := json.Marshal(params)
		_ = json.Unmarshal(raw, &params)
		sev := d.Severity
		if sev == "" {
			sev = "hard"
		}
		id, err := s.SaveRule(ctx, RuleRow{Type: d.Type, Label: d.Label, Severity: sev, Enabled: d.Enabled == nil || *d.Enabled, Params: params})
		if err != nil {
			return err
		}
		ids[d.ID] = id
	}
	for _, d := range cs.Deviations {
		d.Rule = strconv.FormatInt(ids[d.Rule], 10)
		if err := s.AddDeviation(ctx, d); err != nil {
			return err
		}
	}
	if plan != nil {
		_, err := s.CreatePlan(ctx, planName, plan)
		return err
	}
	return nil
}
