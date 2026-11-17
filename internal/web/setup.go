package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/i18n"
)

// ---- settings ----

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	set, _ := s.st.Settings(r.Context())
	worksOn := map[domain.Weekday]bool{}
	for _, d := range setup.Org.Days {
		worksOn[d] = true
	}
	type dayRow struct {
		Day     domain.Weekday
		Works   bool
		Opening string
	}
	var days []dayRow
	for _, d := range domain.AllWeekdays {
		days = append(days, dayRow{d, worksOn[d], set["opening_"+d.Key()]})
	}
	s.render(w, r, "settings", "settings", "Settings", setup.Org, map[string]any{
		"Days": days, "Languages": i18n.Languages, "Steps": []int{5, 10, 15, 30},
		"Cycles": []int{1, 2, 3, 4}, "Step": int(setup.Org.Step), "Cycle": setup.Org.CycleWeeks,
	})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	lang := r.FormValue("lang")
	if lang != "da" {
		lang = "en"
	}
	set := map[string]string{
		"name":       strings.TrimSpace(r.FormValue("name")),
		"lang":       lang,
		"team_label": strings.TrimSpace(r.FormValue("team_label")),
	}
	if n, err := strconv.Atoi(r.FormValue("cycle_weeks")); err == nil && n >= 1 && n <= 4 {
		set["cycle_weeks"] = strconv.Itoa(n)
	}
	if n, err := strconv.Atoi(r.FormValue("time_step")); err == nil && n > 0 && n <= 60 {
		set["time_step"] = strconv.Itoa(n)
	}
	var days []string
	var bad []string
	for _, d := range domain.AllWeekdays {
		if r.FormValue("day_"+d.Key()) != "" {
			days = append(days, d.Key())
		}
		v := strings.TrimSpace(r.FormValue("opening_" + d.Key()))
		if v != "" {
			win, err := domain.ParseWindow(v)
			if err != nil {
				bad = append(bad, i18n.Day(lang, d)+": "+err.Error())
				continue
			}
			v = win.String()
		}
		set["opening_"+d.Key()] = v
	}
	set["days"] = strings.Join(days, ",")
	if err := s.st.SetSettings(r.Context(), set); err != nil {
		fail(w, err)
		return
	}
	if len(bad) > 0 {
		back(w, r, "/settings", "err", strings.Join(bad, "; "))
		return
	}
	back(w, r, "/settings", "msg", i18n.T(lang, "Saved"))
}

// ---- teams ----

func (s *Server) teams(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	count := map[string]int{}
	for _, p := range setup.Org.People {
		count[p.Team]++
	}
	s.render(w, r, "teams", "teams", "Teams", setup.Org, map[string]any{"Count": count})
}

var palette = []string{"#7C3AED", "#DC2626", "#DB2777", "#0EA5A4", "#2563EB", "#EA580C", "#16A34A", "#CA8A04"}

func (s *Server) saveTeam(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	o := setup.Org
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		back(w, r, "/teams", "err", i18n.T(o.Lang, "Name is required"))
		return
	}
	id := r.FormValue("id")
	sortKey, _ := strconv.Atoi(r.FormValue("sort"))
	if id == "" {
		id = uniqueID(name, func(x string) bool { return o.Team(x) != nil })
		sortKey = len(o.Teams) + 1
	}
	color := r.FormValue("color")
	if color == "" {
		color = palette[len(o.Teams)%len(palette)]
	}
	if err := s.st.SaveTeam(r.Context(), domain.Team{ID: id, Name: name, Color: color}, sortKey); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/teams", "msg", i18n.T(o.Lang, "Saved"))
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if len(setup.Org.Members(r.PathValue("id"))) > 0 {
		back(w, r, "/teams", "err", i18n.T(setup.Org.Lang, "Move or delete the people in this team first"))
		return
	}
	if err := s.st.DeleteTeam(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/teams", "msg", i18n.T(setup.Org.Lang, "Deleted"))
}

// ---- staff ----

func (s *Server) staff(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, r, "staff", "staff", "Staff", setup.Org, nil)
}

func (s *Server) person(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	o := setup.Org
	p := o.Person(r.PathValue("id"))
	isNew := p == nil
	if isNew {
		if r.PathValue("id") != "new" {
			http.NotFound(w, r)
			return
		}
		p = &domain.Person{}
		if len(o.Teams) > 0 {
			p.Team = o.Teams[0].ID
		}
	}
	type fixedRow struct {
		Day   domain.Weekday
		Value string
	}
	var fixed []fixedRow
	for _, d := range o.Days {
		v := ""
		if win, ok := p.Fixed[d]; ok {
			v = win.String()
		}
		fixed = append(fixed, fixedRow{d, v})
	}
	hours := ""
	if p.WeeklyMinutes > 0 {
		hours = p.WeeklyMinutes.String()
	}
	title := p.Name
	if isNew {
		title = "New person"
	}
	s.render(w, r, "person", "staff", title, o, map[string]any{
		"P": p, "New": isNew, "Fixed": fixed, "Hours": hours, "Tags": strings.Join(p.Tags, ", "),
	})
}

func (s *Server) savePerson(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	o := setup.Org
	id := r.PathValue("id")
	self := "/staff/" + id
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		back(w, r, self, "err", i18n.T(o.Lang, "Name is required"))
		return
	}
	if o.Team(r.FormValue("team")) == nil {
		back(w, r, self, "err", i18n.T(o.Lang, "Choose a team"))
		return
	}
	mins, err := parseHours(r.FormValue("hours"))
	if err != nil {
		back(w, r, self, "err", err.Error())
		return
	}
	p := domain.Person{Name: name, Short: strings.TrimSpace(r.FormValue("short")), Team: r.FormValue("team"), WeeklyMinutes: mins}
	for _, t := range strings.Split(r.FormValue("tags"), ",") {
		if t = domain.Slug(t); t != "" {
			p.Tags = append(p.Tags, t)
		}
	}
	for _, d := range o.Days {
		v := strings.TrimSpace(r.FormValue("fixed_" + d.Key()))
		if v == "" {
			continue
		}
		win, err := domain.ParseWindow(v)
		if err != nil {
			back(w, r, self, "err", i18n.Day(o.Lang, d)+": "+err.Error())
			return
		}
		if p.Fixed == nil {
			p.Fixed = map[domain.Weekday]domain.Window{}
		}
		p.Fixed[d] = win
	}
	sortKey := 0
	if id == "new" {
		id = uniqueID(name, func(x string) bool { return o.Person(x) != nil })
		sortKey = len(o.People) + 1
	} else if o.Person(id) == nil {
		http.NotFound(w, r)
		return
	} else {
		for i, x := range o.People {
			if x.ID == id {
				sortKey = i
			}
		}
	}
	p.ID = id
	if err := s.st.SavePerson(r.Context(), p, sortKey); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/staff", "msg", i18n.T(o.Lang, "Saved"))
}

func (s *Server) deletePerson(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if r.FormValue("confirm") != "yes" {
		back(w, r, "/staff/"+r.PathValue("id"), "err", i18n.T(setup.Org.Lang, "Tick the box to confirm"))
		return
	}
	if err := s.st.DeletePerson(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/staff", "msg", i18n.T(setup.Org.Lang, "Deleted"))
}

// ---- activities ----

func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, r, "activities", "activities", "Activities", setup.Org, map[string]any{
		"Kinds": []domain.ActivityKind{domain.KindDuty, domain.KindTask, domain.KindEvent},
	})
}

func (s *Server) saveActivity(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	o := setup.Org
	label := strings.TrimSpace(r.FormValue("label"))
	code := strings.TrimSpace(r.FormValue("code"))
	kind := domain.ActivityKind(r.FormValue("kind"))
	if label == "" || code == "" {
		back(w, r, "/activities", "err", i18n.T(o.Lang, "Name and code are required"))
		return
	}
	if kind != domain.KindDuty && kind != domain.KindTask && kind != domain.KindEvent {
		kind = domain.KindDuty
	}
	id := r.FormValue("id")
	sortKey, _ := strconv.Atoi(r.FormValue("sort"))
	if id == "" {
		id = uniqueID(label, func(x string) bool { return o.Activity(x) != nil })
		sortKey = len(o.Activities) + 1
	}
	for _, a := range o.Activities {
		if a.Code == code && a.ID != id {
			back(w, r, "/activities", "err", i18n.T(o.Lang, "Code is already used"))
			return
		}
	}
	if err := s.st.SaveActivity(r.Context(), domain.Activity{ID: id, Code: code, Label: label, Kind: kind}, sortKey); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/activities", "msg", i18n.T(o.Lang, "Saved"))
}

func (s *Server) deleteActivity(w http.ResponseWriter, r *http.Request) {
	setup, err := s.st.LoadSetup(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.st.DeleteActivity(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, "/activities", "msg", i18n.T(setup.Org.Lang, "Deleted"))
}
