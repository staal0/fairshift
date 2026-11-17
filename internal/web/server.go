// Package web serves the planner UI.
package web

import (
	"bytes"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/i18n"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

type Server struct {
	st    *store.Store
	pages map[string]*template.Template
}

var pageNames = []string{"plans", "plan", "settings", "teams", "staff", "person", "activities", "rules", "rule"}

func New(st *store.Store) (*Server, error) {
	s := &Server{st: st, pages: map[string]*template.Template{}}
	for _, p := range pageNames {
		t, err := template.New("layout.html").Funcs(funcs("en", nil)).
			ParseFS(templateFS, "templates/layout.html", "templates/"+p+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", p, err)
		}
		s.pages[p] = t
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("GET /{$}", s.home)

	mux.HandleFunc("GET /plans", s.plans)
	mux.HandleFunc("POST /plans", s.newPlan)
	mux.HandleFunc("GET /plans/{id}", s.plan)
	mux.HandleFunc("POST /plans/{id}/cells", s.saveCells)
	mux.HandleFunc("POST /plans/{id}/rename", s.renamePlan)
	mux.HandleFunc("POST /plans/{id}/copy", s.copyPlan)
	mux.HandleFunc("POST /plans/{id}/delete", s.deletePlan)
	mux.HandleFunc("GET /plans/{id}/export.xlsx", s.exportXLSX)

	mux.HandleFunc("GET /settings", s.settings)
	mux.HandleFunc("POST /settings", s.saveSettings)
	mux.HandleFunc("GET /teams", s.teams)
	mux.HandleFunc("POST /teams", s.saveTeam)
	mux.HandleFunc("POST /teams/{id}/delete", s.deleteTeam)
	mux.HandleFunc("GET /staff", s.staff)
	mux.HandleFunc("GET /staff/{id}", s.person)
	mux.HandleFunc("POST /staff/{id}", s.savePerson)
	mux.HandleFunc("POST /staff/{id}/delete", s.deletePerson)
	mux.HandleFunc("GET /activities", s.activities)
	mux.HandleFunc("POST /activities", s.saveActivity)
	mux.HandleFunc("POST /activities/{id}/delete", s.deleteActivity)
	mux.HandleFunc("GET /rules", s.rules)
	mux.HandleFunc("GET /rules/{id}", s.rule)
	mux.HandleFunc("POST /rules/{id}", s.saveRule)
	mux.HandleFunc("POST /rules/{id}/delete", s.deleteRule)
	mux.HandleFunc("POST /deviations", s.addDeviation)
	mux.HandleFunc("POST /deviations/{id}/delete", s.deleteDeviation)
	return logRequests(mux)
}

func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}

// funcs returns template functions bound to a language.
func funcs(lang string, org *domain.Org) template.FuncMap {
	return template.FuncMap{
		"t":      func(s string) string { return i18n.T(lang, s) },
		"tf":     func(f string, a ...any) string { return i18n.Tf(lang, f, a...) },
		"day":    func(d domain.Weekday) string { return i18n.Day(lang, d) },
		"dshort": func(d domain.Weekday) string { return i18n.DayShort(lang, d) },
		"dayref": func(d *domain.DayRef) string {
			if d == nil {
				return ""
			}
			return i18n.DayRef(lang, *d)
		},
		"msg": func(v rules.Violation) string { return message(lang, v) },
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return i18n.T(lang, one)
			}
			return i18n.T(lang, many)
		},
		"teamword": func() string {
			if org != nil && org.TeamLabel != "" {
				return org.TeamLabel
			}
			return i18n.T(lang, "Team")
		},
		"add":   func(a, b int) int { return a + b },
		"mul":   func(a, b int) int { return a * b },
		"clock": func(m domain.Minute) string { return m.String() },
		"dur":   func(d domain.Duration) string { return d.String() },
		"signed": func(d domain.Duration) string {
			if d > 0 {
				return "+" + d.String()
			}
			return d.String()
		},
		"heat": func(n, max int) string {
			if max == 0 {
				return "0%"
			}
			return fmt.Sprintf("%.0f%%", 100*(0.15+0.85*float64(n)/float64(max)))
		},
		"f2":     func(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) },
		"isHour": func(m domain.Minute) bool { return m%60 == 0 },
		"hour":   func(m domain.Minute) int { return int(m) / 60 },
	}
}

// message renders a finding in lang.
func message(lang string, v rules.Violation) string {
	out := v.Message
	if v.Format != "" {
		out = i18n.Tf(lang, v.Format, v.Args...)
	}
	if v.Note != "" {
		prefix := ""
		if v.Deviation != "" && v.Deviation != "override" {
			prefix = i18n.T(lang, "accepted deviation") + ": "
		}
		out += " – " + prefix + i18n.T(lang, v.Note)
	}
	return out
}

// view is the common page data.
type view struct {
	L     string
	Title string
	Nav   string
	Org   *domain.Org
	Msg   string
	Err   string
	D     map[string]any
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page, nav, title string, org *domain.Org, d map[string]any) {
	lang := "en"
	if org != nil && org.Lang != "" {
		lang = org.Lang
	}
	t, err := s.pages[page].Clone()
	if err == nil {
		t = t.Funcs(funcs(lang, org))
	}
	v := view{L: lang, Title: i18n.T(lang, title), Nav: nav, Org: org, Msg: r.URL.Query().Get("msg"), Err: r.URL.Query().Get("err"), D: d}
	if e, ok := d["Err"].(string); ok && e != "" {
		v.Err = e
	}
	var buf bytes.Buffer
	if err == nil {
		err = t.Execute(&buf, v)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func fail(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	log.Print(err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// back redirects to path with a message (key "msg") or error (key "err").
func back(w http.ResponseWriter, r *http.Request, path, key, text string) {
	if text != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + key + "=" + url.QueryEscape(text)
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	empty, err := s.st.Empty(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if empty {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}

// uniqueID returns an ASCII id from name that is not taken.
func uniqueID(name string, taken func(string) bool) string {
	base := domain.Slug(name)
	if base == "" {
		base = "item"
	}
	id := base
	for i := 2; taken(id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// parseHours accepts "36:15", "36.25" or "36,25".
func parseHours(s string) (domain.Duration, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, nil
	}
	if strings.Contains(s, ":") {
		return domain.ParseDuration(s)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("invalid hours %q", s)
	}
	return domain.Duration(f*60 + 0.5), nil
}
