// Package i18n translates UI text. English text is the key; other
// languages are JSON files mapping English text to the translation.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/staal0/fairshift/internal/domain"
)

//go:embed *.json
var files embed.FS

// Languages lists supported languages (code -> native name key).
var Languages = []string{"en", "da"}

var catalogs = map[string]map[string]string{}

func init() {
	for _, lang := range Languages[1:] {
		raw, err := files.ReadFile(lang + ".json")
		if err != nil {
			panic(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			panic(fmt.Sprintf("i18n %s: %v", lang, err))
		}
		catalogs[lang] = m
	}
}

// T translates text into lang, falling back to the English text.
func T(lang, text string) string {
	if v, ok := catalogs[lang][text]; ok && v != "" {
		return v
	}
	return text
}

// Tf translates a printf format and applies the arguments.
func Tf(lang, format string, args ...any) string {
	return fmt.Sprintf(T(lang, format), args...)
}

func Day(lang string, d domain.Weekday) string      { return T(lang, d.String()) }
func DayShort(lang string, d domain.Weekday) string { return T(lang, d.Short()) }

// DayRef formats "week 1 Mon" in lang.
func DayRef(lang string, d domain.DayRef) string {
	return Tf(lang, "week %d", d.Week+1) + " " + DayShort(lang, d.Day)
}

// Missing returns English keys used in the UI that lack a translation.
func Missing(lang string, keys []string) []string {
	var out []string
	for _, k := range keys {
		if _, ok := catalogs[lang][k]; !ok {
			out = append(out, k)
		}
	}
	return out
}
