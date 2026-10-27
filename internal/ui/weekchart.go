package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

// Chart geometry is in SVG user units. The template sizes the drawing to the
// container width, so twelve weeks stay inside a 390px phone without a sideways scroll.
const (
	weekChartWidth    = 360.0
	weekChartHeight   = 176.0
	weekPlotLeft      = 34.0
	weekPlotRight     = 8.0
	weekPlotTop       = 16.0
	weekPlotBottom    = 38.0
	weekTotalBarWidth = 10.0
	weekNightBarWidth = 6.0
	weekBarGap        = 2.0
)

type weekChartDrawing struct {
	ViewBox string
	Axis    weekChartLine
	CapLine weekChartLine
	Labels  []weekChartText
	Columns []weekChartColumn
}

type weekChartLine struct {
	X1, Y1, X2, Y2 string
}

type weekChartText struct {
	X, Y, DY, Text, Class, Anchor string
}

type weekChartColumn struct {
	Class string
	Title string
	Day   weekChartText
	Month weekChartText
	Bars  []weekChartBar
	Flag  string
}

type weekChartBar struct {
	Class      string
	X, Y, W, H string
}

func drawWeekChart(chart model.WeekCreditChart) weekChartDrawing {
	scale := float64(model.WeeklyHourCap)
	for _, week := range chart.Weeks {
		if week.LoggedTotal > scale {
			scale = week.LoggedTotal
		}
	}
	scale = math.Ceil(scale - 1e-9)
	baseline := weekChartHeight - weekPlotBottom
	plotHeight := baseline - weekPlotTop
	plotWidth := weekChartWidth - weekPlotLeft - weekPlotRight
	slot := plotWidth / float64(len(chart.Weeks))
	groupWidth := weekTotalBarWidth + weekBarGap + weekNightBarWidth
	capY := baseline - float64(model.WeeklyHourCap)/scale*plotHeight

	drawing := weekChartDrawing{
		ViewBox: fmt.Sprintf("0 0 %g %g", weekChartWidth, weekChartHeight),
		Axis:    weekChartLine{f1(weekPlotLeft), f1(baseline), f1(weekChartWidth - weekPlotRight), f1(baseline)},
		CapLine: weekChartLine{f1(weekPlotLeft), f1(capY), f1(weekChartWidth - weekPlotRight), f1(capY)},
		Labels: []weekChartText{
			axisLabel(baseline, "0", false),
			axisLabel(capY, strconv.Itoa(model.WeeklyHourCap), true),
		},
	}
	if scale >= float64(model.WeeklyHourCap)+2 {
		drawing.Labels = append(drawing.Labels, axisLabel(weekPlotTop, strconv.Itoa(int(scale)), false))
	}

	for i, week := range chart.Weeks {
		origin := weekPlotLeft + slot*float64(i) + (slot-groupWidth)/2
		center := origin + groupWidth/2
		column := weekChartColumn{
			Class: "week-column",
			Title: weekBarTitle(week),
			Day: weekChartText{
				X: f1(center), Y: f1(baseline + 14), Text: strconv.Itoa(week.Starts.Day()),
				Class: "week-label", Anchor: "middle",
			},
		}
		if week.Capped {
			column.Class = "week-column week-column--capped"
			column.Day.Class = "week-label week-label--capped"
			column.Flag = flagPoints(center, barTop(week.LoggedTotal, scale, baseline, plotHeight))
		}
		if i == 0 || week.Starts.Month() != chart.Weeks[i-1].Starts.Month() {
			column.Month = weekChartText{
				X: f1(center), Y: f1(baseline + 28), Text: week.Starts.Format("Jan"),
				Class: "week-month", Anchor: "middle",
			}
		}
		column.Bars = weekBars(origin, week, scale, baseline, plotHeight)
		drawing.Columns = append(drawing.Columns, column)
	}
	return drawing
}

func weekBars(origin float64, week model.WeekCreditBar, scale, baseline, plotHeight float64) []weekChartBar {
	totalX := origin
	nightX := origin + weekTotalBarWidth + weekBarGap
	specs := []struct {
		class string
		hours float64
		x, w  float64
	}{
		{"week-bar week-bar--logged", week.LoggedTotal, totalX, weekTotalBarWidth},
		{"week-bar week-bar--credited", week.CreditedTotal, totalX, weekTotalBarWidth},
		{"week-bar week-bar--night-logged", week.LoggedNight, nightX, weekNightBarWidth},
		{"week-bar week-bar--night", week.CreditedNight, nightX, weekNightBarWidth},
	}
	bars := make([]weekChartBar, 0, len(specs))
	for _, spec := range specs {
		if spec.hours <= 0 || scale <= 0 {
			continue
		}
		height := spec.hours / scale * plotHeight
		bars = append(bars, weekChartBar{
			Class: spec.class,
			X:     f1(spec.x),
			Y:     f1(baseline - height),
			W:     f1(spec.w),
			H:     f1(height),
		})
	}
	return bars
}

func barTop(hours, scale, baseline, plotHeight float64) float64 {
	if hours <= 0 || scale <= 0 {
		return baseline
	}
	return baseline - hours/scale*plotHeight
}

func flagPoints(center, top float64) string {
	y := top - 8
	if y < 2 {
		y = 2
	}
	return fmt.Sprintf("%s,%s %s,%s %s,%s", f1(center), f1(y), f1(center-4), f1(y+6), f1(center+4), f1(y+6))
}

func axisLabel(y float64, text string, cap bool) weekChartText {
	class := "week-axis-label"
	dy := "-3"
	if cap {
		class = "week-axis-label week-axis-label--cap"
	}
	if y < weekPlotTop+12 {
		dy = "9"
	}
	return weekChartText{X: f1(weekPlotLeft - 4), Y: f1(y), DY: dy, Text: text, Class: class, Anchor: "end"}
}

func weekBarTitle(week model.WeekCreditBar) string {
	starts := week.Starts
	title := fmt.Sprintf("Week of %s: logged %s total and %s night, credited %s total and %s night",
		model.DisplayDate(&starts),
		model.FormatHours(week.LoggedTotal), model.FormatHours(week.LoggedNight),
		model.FormatHours(week.CreditedTotal), model.FormatHours(week.CreditedNight))
	if week.Capped {
		title += fmt.Sprintf(", capped, %s not credited", model.FormatHours(week.LoggedTotal-week.CreditedTotal))
	}
	return title
}

func weekChartSummary(chart model.WeekCreditChart) string {
	var text strings.Builder
	fmt.Fprintf(&text, "These %d weeks credit %s total and %s night", len(chart.Weeks), model.FormatHours(chart.WindowTotal), model.FormatHours(chart.WindowNight))
	outside := chart.EarlierTotal != 0 || chart.EarlierNight != 0 || chart.LaterTotal != 0 || chart.LaterNight != 0 || chart.UndatedTotal != 0 || chart.UndatedNight != 0
	windowMatches := model.FormatHours(chart.WindowTotal) == model.FormatHours(chart.TotalHours) && model.FormatHours(chart.WindowNight) == model.FormatHours(chart.NightHours)
	if !outside && windowMatches {
		text.WriteString(", the same weekly credited total shown above.")
	} else {
		text.WriteString(".")
		if chart.EarlierTotal != 0 || chart.EarlierNight != 0 {
			fmt.Fprintf(&text, " Earlier entry weeks credit %s total and %s night.", model.FormatHours(chart.EarlierTotal), model.FormatHours(chart.EarlierNight))
		}
		if chart.LaterTotal != 0 || chart.LaterNight != 0 {
			fmt.Fprintf(&text, " Later entry weeks credit %s total and %s night.", model.FormatHours(chart.LaterTotal), model.FormatHours(chart.LaterNight))
		}
		if chart.UndatedTotal != 0 || chart.UndatedNight != 0 {
			fmt.Fprintf(&text, " Undated opening hours add %s total and %s night.", model.FormatHours(chart.UndatedTotal), model.FormatHours(chart.UndatedNight))
		}
		fmt.Fprintf(&text, " Together this is %s total and %s night, the same weekly credited total shown above.", model.FormatHours(chart.TotalHours), model.FormatHours(chart.NightHours))
	}
	var capped []string
	for _, week := range chart.Weeks {
		if !week.Capped {
			continue
		}
		starts := week.Starts
		capped = append(capped, fmt.Sprintf("week of %s (lost %s)", model.DisplayDate(&starts), model.FormatHours(week.LoggedTotal-week.CreditedTotal)))
	}
	if len(capped) > 0 {
		fmt.Fprintf(&text, " Capped: %s.", strings.Join(capped, "; "))
	}
	return text.String()
}

func weekChartNote(chart model.WeekCreditChart) string {
	return fmt.Sprintf("Wide bars are total hours and narrow bars are night hours, for the last %d Monday–Sunday entry weeks. Filled color is credited. Pale color above it was logged and did not count. A red mark is a week over the %d-hour cap.", len(chart.Weeks), model.WeeklyHourCap)
}

func weekStart(week model.WeekCreditBar) *time.Time {
	starts := week.Starts
	return &starts
}

func cappedLabel(capped bool) string {
	if capped {
		return "Yes"
	}
	return "No"
}

func weekChartTitle(chart model.WeekCreditChart) string {
	capped := 0
	for _, week := range chart.Weeks {
		if week.Capped {
			capped++
		}
	}
	if capped == 0 {
		return fmt.Sprintf("Logged and credited hours for the last %d entry weeks. No week hit the cap.", len(chart.Weeks))
	}
	return fmt.Sprintf("Logged and credited hours for the last %d entry weeks. %d weeks hit the %d-hour cap.", len(chart.Weeks), capped, model.WeeklyHourCap)
}

func f1(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}
