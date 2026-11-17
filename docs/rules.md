# Rule reference

Rules are created in the app under **Rules**. This page lists the rule types and their parameters
as stored. The example setup in [testdata/example-daycare/org.yaml](../testdata/example-daycare/org.yaml)
uses all of them (that file is a test fixture and demo seed).

Every rule has a `type`, a `label`, a `severity` (`hard` default, `soft`, `info`) and `params`.

Common params: `scope: {teams, people, tags, exclude, exclude_fixed}` selects people; `days: [mon, …]` and `weeks: [1, 2]` select days.

| Type | What it checks | Example params |
|---|---|---|
| `coverage` | At least `min` / at most `max` people at work during a window (`opening` = opening hours) | `{scope: {teams: [c]}, window: "07:30-15:30", min: 1}` |
| `duty` | Exactly one holder per scheduled day; fixed person, rotation over cycle weeks, pool or `any`; shift shape | `{duty: opener, shape: {start: "06:00", end: "13:00"}, schedule: {mon: ida, fri: [emma, camilla], thu: any}}` (a list rotates over the cycle weeks) |
| `shift_window` | Start/end bounds, allowed start times, min/max length | `{latest_start: "09:00", allowed_starts: ["06:00", "08:00"], min_length: "6:00"}` |
| `time_slot_count` | Number of people starting/ending at an exact time (ladders) | `{edge: start, at: "06:45", op: exactly, n: 1}` |
| `fair_distribution` | Burden counts per person and balance between people/teams | `{metric: {start_before: "08:00"}, scope: {exclude_fixed: true}, per_person: {min: 2, max: 3}}` |
| `in_shift_task` | Task inside a shift, N per week, distinct days per team, rest of team present | `{task: office, window: "08:00-09:00", per_week: 1, distinct_days_within_team: true, others_in_team_present: true}` |
| `team_event` | Team meeting: everyone present, cover from other teams | `{event: meeting, window: "13:00-14:00", cover: {b: {by: [a], min: 3}}}` |
| `hours_target` | Planned hours vs contract per week or cycle, with tolerance | `{period: week, tolerance: "0:15"}` |
| `pinned` | People with `fixed` shifts work exactly those | – |

Metric conditions for `fair_distribution`: `start_before`, `start_after`, `end_before`, `end_after`, `code`, `days`.

## Accepted deviations

A known, accepted violation is added under **Rules → Accepted deviations** and becomes a warning.
In a fixture it looks like this:

```yaml
deviations:
  - id: room-b-week2-office
    rule: office-hour
    team: b        # optional filters: person, team, week, day
    week: 2
    day: wed
    reason: Only three usable office-hour days in week 2.
```
