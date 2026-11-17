package domain

import "testing"

func TestParseCell(t *testing.T) {
	cases := map[string]string{
		"06:45-14:00 (K,M)": "06:45-14:00 (K,M)",
		"08:00–15:15":       "08:00-15:15",
		" 09:00-16:20 (L) ": "09:00-16:20 (L)",
		"22:00-06:00":       "22:00-06:00",
		"08001530":          "08:00-15:30",
		"8001530":           "08:00-15:30",
		"0800 1530":         "08:00-15:30",
		"800-1530 K M":      "08:00-15:30 (K,M)",
		"8-15":              "08:00-15:00",
		"800930":            "08:00-09:30",
		"0600 1300 opener":  "06:00-13:00 (opener)",
		"7.30-14.45 (a)":    "07:30-14:45 (a)",
	}
	for in, want := range cases {
		s, ok, err := ParseCell(in)
		if err != nil || !ok {
			t.Fatalf("%q: %v", in, err)
		}
		if got := FormatCell(s); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	if s, _, _ := ParseCell("22:00-06:00"); s.Len() != 8*60 {
		t.Errorf("night shift length %v", s.Len())
	}
	for _, off := range []string{"", "-", "off"} {
		if _, ok, err := ParseCell(off); ok || err != nil {
			t.Errorf("%q should be off", off)
		}
	}
	for _, bad := range []string{"08:00", "25:00-26:00x", "123456789", "abc"} {
		if _, _, err := ParseCell(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("S\u00f8ren \u00c6r\u00f8 Jensen"); got != "soeren-aeroe-jensen" {
		t.Errorf("got %q", got)
	}
}
