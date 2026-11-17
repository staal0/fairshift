// Package domain holds the core scheduling types. It has no I/O.
package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// Minute is a point in time measured in minutes since midnight of the
// shift's anchor day. Values >= 1440 mean the next day (night shifts).
type Minute int

// ParseClock parses a time of day: "08:00", "8:00", "8.00", "0800", "800" or "8".
func ParseClock(s string) (Minute, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ".", ":")
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		if s == "" || strings.Trim(s, "0123456789") != "" || len(s) > 4 {
			return 0, fmt.Errorf("invalid time %q: want HH:MM", s)
		}
		h, m = s, "00"
		if len(s) > 2 {
			h, m = s[:len(s)-2], s[len(s)-2:]
		}
	}
	hh, err := strconv.Atoi(h)
	if err != nil || hh < 0 || hh > 47 || len(h) > 2 {
		return 0, fmt.Errorf("invalid hour in %q", s)
	}
	mm, err := strconv.Atoi(m)
	if err != nil || mm < 0 || mm > 59 || len(m) != 2 {
		return 0, fmt.Errorf("invalid minute in %q", s)
	}
	return Minute(hh*60 + mm), nil
}

// MustClock is ParseClock for constants; it panics on bad input.
func MustClock(s string) Minute {
	m, err := ParseClock(s)
	if err != nil {
		panic(err)
	}
	return m
}

// String formats as HH:MM.
func (m Minute) String() string {
	return fmt.Sprintf("%02d:%02d", int(m)/60, int(m)%60)
}

// Duration is a length of time in minutes.
type Duration int

// ParseDuration parses "H:MM" (e.g. "36:15") or a plain number of minutes.
func ParseDuration(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if h, m, ok := strings.Cut(s, ":"); ok {
		hh, err1 := strconv.Atoi(h)
		mm, err2 := strconv.Atoi(m)
		if err1 != nil || err2 != nil || mm < 0 || mm > 59 || hh < 0 {
			return 0, fmt.Errorf("invalid duration %q: want H:MM", s)
		}
		return Duration(hh*60 + mm), nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid duration %q: want H:MM or minutes", s)
	}
	return Duration(n), nil
}

// String formats as H:MM.
func (d Duration) String() string {
	sign := ""
	if d < 0 {
		sign, d = "-", -d
	}
	return fmt.Sprintf("%s%d:%02d", sign, int(d)/60, int(d)%60)
}

// Window is a half-open time interval [Start, End).
type Window struct {
	Start, End Minute
}

// ParseWindow parses "HH:MM-HH:MM". Separators may be left out:
// "0800 1530", "800-1530", "08001530" and "8-15" all work.
func ParseWindow(s string) (Window, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\u2013", "-"))
	var a, b string
	if x, y, ok := strings.Cut(s, "-"); ok {
		a, b = x, y
	} else if f := strings.Fields(s); len(f) == 2 {
		a, b = f[0], f[1]
	} else if len(f) == 1 && strings.Trim(s, "0123456789") == "" {
		// Digits only: try the longest end time that gives a valid window.
		for n := 4; n >= 1; n-- {
			if len(s) <= n {
				continue
			}
			start, err1 := ParseClock(s[:len(s)-n])
			end, err2 := ParseClock(s[len(s)-n:])
			if err1 == nil && err2 == nil && end > start {
				return Window{start, end}, nil
			}
		}
		return Window{}, fmt.Errorf("invalid window %q: want HH:MM-HH:MM", s)
	} else {
		return Window{}, fmt.Errorf("invalid window %q: want HH:MM-HH:MM", s)
	}
	start, err := ParseClock(a)
	if err != nil {
		return Window{}, err
	}
	end, err := ParseClock(b)
	if err != nil {
		return Window{}, err
	}
	if end <= start {
		end += 1440 // crosses midnight
	}
	return Window{start, end}, nil
}

func (w Window) String() string         { return w.Start.String() + "-" + (w.End % 1440).String() }
func (w Window) Len() Duration          { return Duration(w.End - w.Start) }
func (w Window) Covers(o Window) bool   { return w.Start <= o.Start && w.End >= o.End }
func (w Window) Contains(t Minute) bool { return w.Start <= t && t < w.End }
