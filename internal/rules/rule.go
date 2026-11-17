// Package rules defines the generic rule catalogue. Every rule type checks
// a roster independently; the validator is the single source of truth and
// the solver (later) re-validates its output through the same code.
package rules

import (
	"fmt"
	"sort"

	"github.com/staal0/fairshift/internal/domain"
)

// Severity is how a broken rule is treated.
type Severity string

const (
	Hard Severity = "hard" // must hold: a violation is an error
	Soft Severity = "soft" // should hold: a violation is a warning
	Info Severity = "info" // only reported
)

// Level is the outcome level of a single finding.
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelError
)

func (l Level) String() string {
	return [...]string{"info", "warning", "error"}[l]
}

// Violation is one concrete finding.
type Violation struct {
	Rule    string
	Level   Level
	Message string // English text
	Format  string // printf format, the translation key
	Args    []any
	Note    string // extra explanation, e.g. a deviation's reason
	Person  string // optional
	Team    string // optional
	Day     *domain.DayRef
	// Deviation is set when the finding is a documented, accepted deviation.
	Deviation string
}

// Context gives rules access to the setup and roster.
type Context struct {
	Org    *domain.Org
	Roster *domain.Roster
}

// Rule is one configured rule instance.
type Rule interface {
	ID() string
	Type() string
	Label() string
	Severity() Severity
	Check(ctx *Context) []Violation
}

// Base carries the common fields every rule has.
type Base struct {
	RuleID    string
	RuleType  string
	RuleLabel string
	Sev       Severity
}

func (b Base) ID() string         { return b.RuleID }
func (b Base) Type() string       { return b.RuleType }
func (b Base) Label() string      { return b.RuleLabel }
func (b Base) Severity() Severity { return b.Sev }

// level maps the rule severity to a finding level.
func (b Base) level() Level {
	switch b.Sev {
	case Soft:
		return LevelWarning
	case Info:
		return LevelInfo
	default:
		return LevelError
	}
}

// v builds a violation at the rule's own level.
func (b Base) v(format string, args ...any) Violation {
	return Violation{Rule: b.RuleID, Level: b.level(), Message: fmt.Sprintf(format, args...), Format: format, Args: args}
}

func (v Violation) at(d domain.DayRef) Violation { v.Day = &d; return v }
func (v Violation) who(p string) Violation       { v.Person = p; return v }
func (v Violation) team(t string) Violation      { v.Team = t; return v }
func (v Violation) lvl(l Level) Violation        { v.Level = l; return v }

// Decoder decodes a rule's params into a typed struct (YAML, JSON …).
type Decoder func(v any) error

// Factory builds a rule from its base fields and params.
type Factory func(base Base, decode Decoder, org *domain.Org) (Rule, error)

var registry = map[string]Factory{}

// Register adds a rule type to the catalogue.
func Register(typ string, f Factory) {
	if _, dup := registry[typ]; dup {
		panic("rules: duplicate type " + typ)
	}
	registry[typ] = f
}

// New builds a rule of the given type.
func New(base Base, decode Decoder, org *domain.Org) (Rule, error) {
	f, ok := registry[base.RuleType]
	if !ok {
		return nil, fmt.Errorf("rule %q: unknown type %q (known: %v)", base.RuleID, base.RuleType, Types())
	}
	if base.Sev == "" {
		base.Sev = Hard
	}
	switch base.Sev {
	case Hard, Soft, Info:
	default:
		return nil, fmt.Errorf("rule %q: invalid severity %q", base.RuleID, base.Sev)
	}
	r, err := f(base, decode, org)
	if err != nil {
		return nil, fmt.Errorf("rule %q (%s): %w", base.RuleID, base.RuleType, err)
	}
	return r, nil
}

// Types lists registered rule types.
func Types() []string {
	var out []string
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
