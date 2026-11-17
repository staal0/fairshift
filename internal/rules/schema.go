package rules

// Field describes one rule parameter for the rule editor. Key is a dotted
// path into params ("metric.start_before").
type Field struct {
	Key  string
	Kind string
}

// Field kinds understood by the editor.
const (
	FText          = "text"
	FInt           = "int"
	FFloat         = "float"
	FBool          = "bool"
	FClock         = "clock"          // "HH:MM"
	FClockOrAnchor = "clock_anchor"   // "HH:MM", "opening" or "closing"
	FDuration      = "duration"       // "H:MM"
	FWindow        = "window"         // "HH:MM-HH:MM"
	FWindowOpening = "window_opening" // window or "opening"
	FClocks        = "clocks"         // comma separated clocks
	FWeekdays      = "weekdays"
	FWeeks         = "weeks"
	FTeams         = "teams"
	FPeople        = "people"
	FDuty          = "activity:duty"
	FTask          = "activity:task"
	FEvent         = "activity:event"
	FAnyActivity   = "activity:any"
	FDutySchedule  = "duty_schedule"
	FTeamCover     = "team_cover"
	FTeamOverride  = "team_override"
)

// Select returns a select field kind with options.
func Select(opts ...string) string {
	k := "select:"
	for i, o := range opts {
		if i > 0 {
			k += "|"
		}
		k += o
	}
	return k
}

var scopeFields = []Field{{"scope.teams", FTeams}, {"scope.people", FPeople}, {"scope.exclude_fixed", FBool}}
var dayFields = []Field{{"days", FWeekdays}, {"weeks", FWeeks}}

func join(parts ...[]Field) []Field {
	var out []Field
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Schemas lists the editable fields per rule type, in display order.
var Schemas = map[string][]Field{
	"coverage": join([]Field{{"window", FWindowOpening}, {"min", FInt}, {"max", FInt}}, scopeFields, dayFields),
	"duty": {{"duty", FDuty}, {"shape.start", FClockOrAnchor}, {"shape.end", FClockOrAnchor},
		{"shape.length", FDuration}, {"schedule", FDutySchedule}, {"levers", FWeekdays}},
	"shift_window": join([]Field{{"earliest_start", FClock}, {"latest_start", FClock}, {"earliest_end", FClock},
		{"latest_end", FClock}, {"allowed_starts", FClocks}, {"min_length", FDuration}, {"max_length", FDuration}},
		scopeFields, dayFields),
	"time_slot_count": join([]Field{{"edge", Select("start", "end")}, {"at", FClock},
		{"op", Select("exactly", "at_least", "at_most")}, {"n", FInt}}, scopeFields, dayFields),
	"fair_distribution": join([]Field{{"name", FText}, {"metric.start_before", FClock}, {"metric.start_after", FClock},
		{"metric.end_before", FClock}, {"metric.end_after", FClock}, {"metric.code", FAnyActivity},
		{"metric.days", FWeekdays}, {"period", Select("week", "cycle")}, {"per_person.min", FInt},
		{"per_person.max", FInt}, {"max_person_spread", FInt}, {"max_team_spread", FFloat}}, scopeFields),
	"in_shift_task": join([]Field{{"task", FTask}, {"window", FWindow}, {"per_week", FInt},
		{"distinct_days_within_team", FBool}, {"others_in_team_present", FBool}, {"team_overrides", FTeamOverride}},
		scopeFields),
	"team_event": {{"event", FEvent}, {"window", FWindow}, {"teams", FTeams}, {"per_week", FInt},
		{"exempt_fixed", FBool}, {"cover", FTeamCover}},
	"hours_target": join([]Field{{"period", Select("week", "cycle")}, {"tolerance", FDuration}, {"include_fixed", FBool}}, scopeFields),
	"pinned":       nil,
}
