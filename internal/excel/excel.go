// Package excel exports rosters as .xlsx.
package excel

import (
	"fmt"
	"io"
	"strings"

	"github.com/staal0/fairshift/internal/domain"
	"github.com/staal0/fairshift/internal/i18n"
	"github.com/staal0/fairshift/internal/rules"
	"github.com/staal0/fairshift/internal/validate"
	"github.com/xuri/excelize/v2"
)

// Export writes one sheet per cycle week with a live hours formula per
// person, plus a sheet with the rule check. msg renders a finding.
func Export(w io.Writer, o *domain.Org, ro *domain.Roster, rep *validate.Report, msg func(rules.Violation) string) error {
	t := func(s string) string { return i18n.T(o.Lang, s) }
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	x := &writer{f: f}

	title, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	head, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4F46E5"}, Pattern: 1}})
	team, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"E0E7FF"}, Pattern: 1}})
	num, _ := f.NewStyle(&excelize.Style{NumFmt: 2})

	nd := len(o.Days)
	col := func(i int) string { c, _ := excelize.ColumnNumberToName(i); return c }
	for wk := 0; wk < o.CycleWeeks; wk++ {
		sheet := i18n.Tf(o.Lang, "Week %d", wk+1)
		if wk == 0 {
			x.SetSheetName("Sheet1", sheet)
		} else {
			x.NewSheet(sheet)
		}
		x.SetCellValue(sheet, "A1", o.Name+" – "+sheet)
		x.SetCellStyle(sheet, "A1", "A1", title)
		x.SetCellValue(sheet, "A3", t("Staff"))
		for i, d := range o.Days {
			x.SetCellValue(sheet, col(i+2)+"3", i18n.Day(o.Lang, d))
		}
		x.SetCellValue(sheet, col(nd+2)+"3", t("Hours"))
		x.SetCellValue(sheet, col(nd+3)+"3", t("Contract"))
		x.SetCellValue(sheet, col(nd+4)+"3", t("Diff"))
		x.SetCellStyle(sheet, "A3", col(nd+4)+"3", head)
		row := 4
		for _, tm := range o.Teams {
			x.SetCellValue(sheet, fmt.Sprintf("A%d", row), tm.Name)
			x.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("%s%d", col(nd+4), row), team)
			row++
			for _, p := range o.Members(tm.ID) {
				x.SetCellValue(sheet, fmt.Sprintf("A%d", row), p.Name)
				var terms []string
				for i, d := range o.Days {
					ref := fmt.Sprintf("%s%d", col(i+2), row)
					if s, ok := ro.Get(p.ID, domain.DayRef{Week: wk, Day: d}); ok {
						x.SetCellValue(sheet, ref, o.DisplayCell(s))
					}
					// Cells are fixed-width "HH:MM-HH:MM", so edits update the total.
					terms = append(terms, fmt.Sprintf(`IF(%s="",0,(TIMEVALUE(MID(%s,7,5))-TIMEVALUE(LEFT(%s,5)))*24)`, ref, ref, ref))
				}
				h := fmt.Sprintf("%s%d", col(nd+2), row)
				n := fmt.Sprintf("%s%d", col(nd+3), row)
				x.SetCellFormula(sheet, h, strings.Join(terms, "+"))
				x.SetCellValue(sheet, n, float64(p.WeeklyMinutes)/60)
				x.SetCellFormula(sheet, fmt.Sprintf("%s%d", col(nd+4), row), h+"-"+n)
				x.SetCellStyle(sheet, h, fmt.Sprintf("%s%d", col(nd+4), row), num)
				row++
			}
		}
		row++
		var legend []string
		for _, a := range o.Activities {
			legend = append(legend, a.Code+" = "+a.Label)
		}
		x.SetCellValue(sheet, fmt.Sprintf("A%d", row), strings.Join(legend, "   ·   "))
		x.SetColWidth(sheet, "A", "A", 28)
		x.SetColWidth(sheet, "B", col(nd+1), 18)
	}

	if rep != nil {
		sheet := t("Rules")
		x.NewSheet(sheet)
		x.SetCellValue(sheet, "A1", t("Status"))
		x.SetCellValue(sheet, "B1", t("Rule"))
		x.SetCellValue(sheet, "C1", t("Finding"))
		x.SetCellStyle(sheet, "A1", "C1", head)
		row := 2
		for _, res := range rep.Results {
			x.SetCellValue(sheet, fmt.Sprintf("A%d", row), t(res.Status()))
			x.SetCellValue(sheet, fmt.Sprintf("B%d", row), res.Rule.Label())
			row++
			for _, v := range res.Findings {
				text := msg(v)
				if v.Day != nil {
					text = i18n.DayRef(o.Lang, *v.Day) + ": " + text
				}
				x.SetCellValue(sheet, fmt.Sprintf("A%d", row), t(v.Level.String()))
				x.SetCellValue(sheet, fmt.Sprintf("C%d", row), text)
				row++
			}
		}
		x.SetColWidth(sheet, "A", "A", 10)
		x.SetColWidth(sheet, "B", "B", 40)
		x.SetColWidth(sheet, "C", "C", 120)
	}
	if x.err != nil {
		return x.err
	}
	return f.Write(w)
}

// writer wraps excelize calls and keeps the first error, so the export
// reads as a list of cell writes.
type writer struct {
	f   *excelize.File
	err error
}

func (x *writer) do(err error) {
	if x.err == nil {
		x.err = err
	}
}

func (x *writer) SetSheetName(a, b string)             { x.do(x.f.SetSheetName(a, b)) }
func (x *writer) NewSheet(name string)                 { _, err := x.f.NewSheet(name); x.do(err) }
func (x *writer) SetCellValue(sh, ref string, v any)   { x.do(x.f.SetCellValue(sh, ref, v)) }
func (x *writer) SetCellStyle(sh, a, b string, st int) { x.do(x.f.SetCellStyle(sh, a, b, st)) }
func (x *writer) SetCellFormula(sh, ref, fm string)    { x.do(x.f.SetCellFormula(sh, ref, fm)) }
func (x *writer) SetColWidth(sh, a, b string, w float64) {
	x.do(x.f.SetColWidth(sh, a, b, w))
}
