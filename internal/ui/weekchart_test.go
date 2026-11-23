package ui

import (
	"reflect"
	"testing"
	"time"

	"github.com/drywaters/permitpal/internal/model"
)

func monday(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

func uncappedWeek(starts time.Time) model.WeekCreditBar {
	return model.WeekCreditBar{Starts: starts, LoggedTotal: 5, CreditedTotal: 5, LoggedNight: 2, CreditedNight: 2}
}

func cappedWeek(starts time.Time) model.WeekCreditBar {
	return model.WeekCreditBar{Starts: starts, LoggedTotal: 14, CreditedTotal: 10, LoggedNight: 4, CreditedNight: 3, Capped: true}
}

func TestWeekBarsHeightsAndBaseline(t *testing.T) {
	const baseline, plotHeight = 138.0, 122.0
	tests := []struct {
		name  string
		week  model.WeekCreditBar
		scale float64
		want  []weekChartBar
	}{
		{
			name:  "uncapped week stacks logged and credited at the same height",
			week:  uncappedWeek(monday(time.August, 3)),
			scale: 10,
			want: []weekChartBar{
				{Class: "week-bar week-bar--logged", X: "0.0", Y: "77.0", W: "10.0", H: "61.0"},
				{Class: "week-bar week-bar--credited", X: "0.0", Y: "77.0", W: "10.0", H: "61.0"},
				{Class: "week-bar week-bar--night-logged", X: "12.0", Y: "113.6", W: "6.0", H: "24.4"},
				{Class: "week-bar week-bar--night", X: "12.0", Y: "113.6", W: "6.0", H: "24.4"},
			},
		},
		{
			name:  "capped week draws logged above credited on a taller scale",
			week:  cappedWeek(monday(time.August, 3)),
			scale: 14,
			want: []weekChartBar{
				{Class: "week-bar week-bar--logged", X: "0.0", Y: "16.0", W: "10.0", H: "122.0"},
				{Class: "week-bar week-bar--credited", X: "0.0", Y: "50.9", W: "10.0", H: "87.1"},
				{Class: "week-bar week-bar--night-logged", X: "12.0", Y: "103.1", W: "6.0", H: "34.9"},
				{Class: "week-bar week-bar--night", X: "12.0", Y: "111.9", W: "6.0", H: "26.1"},
			},
		},
		{
			name:  "zero week draws no bars",
			week:  model.WeekCreditBar{Starts: monday(time.August, 3)},
			scale: 10,
			want:  []weekChartBar{},
		},
		{
			name:  "night-only week draws only the narrow bars",
			week:  model.WeekCreditBar{Starts: monday(time.August, 3), LoggedNight: 3, CreditedNight: 3},
			scale: 10,
			want: []weekChartBar{
				{Class: "week-bar week-bar--night-logged", X: "12.0", Y: "101.4", W: "6.0", H: "36.6"},
				{Class: "week-bar week-bar--night", X: "12.0", Y: "101.4", W: "6.0", H: "36.6"},
			},
		},
		{
			name:  "logged but not credited week keeps the pale bar only",
			week:  model.WeekCreditBar{Starts: monday(time.August, 3), LoggedTotal: 2.5},
			scale: 10,
			want: []weekChartBar{
				{Class: "week-bar week-bar--logged", X: "0.0", Y: "107.5", W: "10.0", H: "30.5"},
			},
		},
		{
			name:  "non-positive scale draws nothing",
			week:  uncappedWeek(monday(time.August, 3)),
			scale: 0,
			want:  []weekChartBar{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weekBars(0, tt.week, tt.scale, baseline, plotHeight)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("weekBars() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBarTop(t *testing.T) {
	const baseline, plotHeight = 138.0, 122.0
	tests := []struct {
		name         string
		hours, scale float64
		want         float64
	}{
		{"zero hours sit on the baseline", 0, 10, baseline},
		{"negative hours sit on the baseline", -1, 10, baseline},
		{"zero scale sits on the baseline", 5, 0, baseline},
		{"half scale is half the plot height", 5, 10, 77},
		{"full scale reaches the plot top", 10, 10, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := barTop(tt.hours, tt.scale, baseline, plotHeight); got != tt.want {
				t.Fatalf("barTop(%g, %g) = %g, want %g", tt.hours, tt.scale, got, tt.want)
			}
		})
	}
}

func TestFlagPoints(t *testing.T) {
	tests := []struct {
		name        string
		center, top float64
		want        string
	}{
		{"sits eight units above the bar", 50, 40, "50.0,32.0 46.0,38.0 54.0,38.0"},
		{"clamps to the top of the viewBox", 50, 5, "50.0,2.0 46.0,8.0 54.0,8.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flagPoints(tt.center, tt.top); got != tt.want {
				t.Fatalf("flagPoints() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDrawWeekChartScaleAndCapLine(t *testing.T) {
	zeroLabel := weekChartText{X: "30.0", Y: "138.0", DY: "-3", Text: "0", Class: "week-axis-label", Anchor: "end"}
	tests := []struct {
		name       string
		weeks      []model.WeekCreditBar
		wantCapY   string
		wantLabels []weekChartText
	}{
		{
			name:     "uncapped weeks keep the cap at the top of the plot",
			weeks:    []model.WeekCreditBar{uncappedWeek(monday(time.August, 3))},
			wantCapY: "16.0",
			wantLabels: []weekChartText{
				zeroLabel,
				{X: "30.0", Y: "16.0", DY: "9", Text: "10", Class: "week-axis-label week-axis-label--cap", Anchor: "end"},
			},
		},
		{
			name:     "a week at exactly the cap does not grow the scale",
			weeks:    []model.WeekCreditBar{{Starts: monday(time.August, 3), LoggedTotal: 10, CreditedTotal: 10}},
			wantCapY: "16.0",
			wantLabels: []weekChartText{
				zeroLabel,
				{X: "30.0", Y: "16.0", DY: "9", Text: "10", Class: "week-axis-label week-axis-label--cap", Anchor: "end"},
			},
		},
		{
			name:     "a one-hour overage grows the scale but adds no top label",
			weeks:    []model.WeekCreditBar{{Starts: monday(time.August, 3), LoggedTotal: 11, CreditedTotal: 10, Capped: true}},
			wantCapY: "27.1",
			wantLabels: []weekChartText{
				zeroLabel,
				{X: "30.0", Y: "27.1", DY: "9", Text: "10", Class: "week-axis-label week-axis-label--cap", Anchor: "end"},
			},
		},
		{
			name:     "a capped week lowers the cap line and labels the new maximum",
			weeks:    []model.WeekCreditBar{cappedWeek(monday(time.August, 3))},
			wantCapY: "50.9",
			wantLabels: []weekChartText{
				zeroLabel,
				{X: "30.0", Y: "50.9", DY: "-3", Text: "10", Class: "week-axis-label week-axis-label--cap", Anchor: "end"},
				{X: "30.0", Y: "16.0", DY: "9", Text: "14", Class: "week-axis-label", Anchor: "end"},
			},
		},
		{
			name:     "fractional overage rounds the scale up to the next hour",
			weeks:    []model.WeekCreditBar{{Starts: monday(time.August, 3), LoggedTotal: 13.2, CreditedTotal: 10, Capped: true}},
			wantCapY: "50.9",
			wantLabels: []weekChartText{
				zeroLabel,
				{X: "30.0", Y: "50.9", DY: "-3", Text: "10", Class: "week-axis-label week-axis-label--cap", Anchor: "end"},
				{X: "30.0", Y: "16.0", DY: "9", Text: "14", Class: "week-axis-label", Anchor: "end"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := drawWeekChart(model.WeekCreditChart{Weeks: tt.weeks})
			if got.ViewBox != "0 0 360 176" {
				t.Errorf("ViewBox = %q, want %q", got.ViewBox, "0 0 360 176")
			}
			if want := (weekChartLine{X1: "34.0", Y1: "138.0", X2: "352.0", Y2: "138.0"}); got.Axis != want {
				t.Errorf("Axis = %+v, want %+v", got.Axis, want)
			}
			if want := (weekChartLine{X1: "34.0", Y1: tt.wantCapY, X2: "352.0", Y2: tt.wantCapY}); got.CapLine != want {
				t.Errorf("CapLine = %+v, want %+v", got.CapLine, want)
			}
			if !reflect.DeepEqual(got.Labels, tt.wantLabels) {
				t.Errorf("Labels = %+v, want %+v", got.Labels, tt.wantLabels)
			}
		})
	}
}

func TestDrawWeekChartColumns(t *testing.T) {
	chart := model.WeekCreditChart{Weeks: []model.WeekCreditBar{
		uncappedWeek(monday(time.July, 27)),
		cappedWeek(monday(time.August, 3)),
		{Starts: monday(time.August, 10)},
		{Starts: monday(time.August, 17), LoggedNight: 3, CreditedNight: 3},
	}}
	got := drawWeekChart(chart)
	if len(got.Columns) != 4 {
		t.Fatalf("got %d columns, want 4", len(got.Columns))
	}

	tests := []struct {
		name       string
		column     weekChartColumn
		wantClass  string
		wantDay    string
		wantMonth  string
		wantFlag   bool
		wantBarLen int
	}{
		{"first uncapped week labels its month", got.Columns[0], "week-column", "27", "Jul", false, 4},
		{"capped week is flagged and starts a new month", got.Columns[1], "week-column week-column--capped", "3", "Aug", true, 4},
		{"zero week has no bars, flag or month label", got.Columns[2], "week-column", "10", "", false, 0},
		{"night-only week draws two bars", got.Columns[3], "week-column", "17", "", false, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.column
			if c.Class != tt.wantClass {
				t.Errorf("Class = %q, want %q", c.Class, tt.wantClass)
			}
			if c.Day.Text != tt.wantDay || c.Day.Y != "152.0" || c.Day.Anchor != "middle" {
				t.Errorf("Day = %+v, want text %q at y 152.0", c.Day, tt.wantDay)
			}
			wantDayClass := "week-label"
			if tt.wantFlag {
				wantDayClass = "week-label week-label--capped"
			}
			if c.Day.Class != wantDayClass {
				t.Errorf("Day.Class = %q, want %q", c.Day.Class, wantDayClass)
			}
			if c.Month.Text != tt.wantMonth {
				t.Errorf("Month.Text = %q, want %q", c.Month.Text, tt.wantMonth)
			}
			if tt.wantMonth != "" && (c.Month.Y != "166.0" || c.Month.X != c.Day.X) {
				t.Errorf("Month = %+v, want y 166.0 and x %s", c.Month, c.Day.X)
			}
			if (c.Flag != "") != tt.wantFlag {
				t.Errorf("Flag = %q, want present=%v", c.Flag, tt.wantFlag)
			}
			if len(c.Bars) != tt.wantBarLen {
				t.Errorf("got %d bars, want %d", len(c.Bars), tt.wantBarLen)
			}
		})
	}
}

func TestDrawWeekChartCapFlagAndBarPlacement(t *testing.T) {
	// One capped week: the slot is 318 wide, so the group origin is 184 and the center 193.
	week := cappedWeek(monday(time.August, 3))
	got := drawWeekChart(model.WeekCreditChart{Weeks: []model.WeekCreditBar{week}})
	if len(got.Columns) != 1 {
		t.Fatalf("got %d columns, want 1", len(got.Columns))
	}
	column := got.Columns[0]
	if len(column.Bars) != 4 {
		t.Fatalf("got %d bars, want 4", len(column.Bars))
	}
	// The logged bar reaches the plot top (16), so the flag sits 8 units above it.
	if want := "193.0,8.0 189.0,14.0 197.0,14.0"; column.Flag != want {
		t.Errorf("Flag = %q, want %q", column.Flag, want)
	}
	if column.Day.X != "193.0" {
		t.Errorf("Day.X = %q, want 193.0", column.Day.X)
	}
	if got, want := column.Bars[0].X, "184.0"; got != want {
		t.Errorf("total bar X = %q, want %q", got, want)
	}
	if got, want := column.Bars[2].X, "196.0"; got != want {
		t.Errorf("night bar X = %q, want %q", got, want)
	}
	if column.Title != weekBarTitle(week) {
		t.Errorf("Title = %q, want the week title", column.Title)
	}
}

func TestDrawWeekChartEmptyWeeks(t *testing.T) {
	tests := []struct {
		name  string
		chart model.WeekCreditChart
	}{
		{"no weeks", model.WeekCreditChart{}},
		{"weeks with no hours", model.WeekCreditChart{Weeks: []model.WeekCreditBar{{Starts: monday(time.August, 3)}, {Starts: monday(time.August, 10)}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := drawWeekChart(tt.chart)
			if len(got.Columns) != len(tt.chart.Weeks) {
				t.Fatalf("got %d columns, want %d", len(got.Columns), len(tt.chart.Weeks))
			}
			for i, column := range got.Columns {
				if len(column.Bars) != 0 || column.Flag != "" {
					t.Errorf("column %d has bars %v or flag %q, want none", i, column.Bars, column.Flag)
				}
			}
			if want := (weekChartLine{X1: "34.0", Y1: "16.0", X2: "352.0", Y2: "16.0"}); got.CapLine != want {
				t.Errorf("CapLine = %+v, want %+v", got.CapLine, want)
			}
			if len(got.Labels) != 2 {
				t.Errorf("got %d axis labels, want 2 (0 and the cap)", len(got.Labels))
			}
		})
	}
}

func TestWeekBarTitle(t *testing.T) {
	tests := []struct {
		name string
		week model.WeekCreditBar
		want string
	}{
		{
			"uncapped",
			uncappedWeek(monday(time.August, 3)),
			"Week of Aug 3, 2026: logged 5.0 total and 2.0 night, credited 5.0 total and 2.0 night",
		},
		{
			"capped names the hours not credited",
			cappedWeek(monday(time.August, 3)),
			"Week of Aug 3, 2026: logged 14.0 total and 4.0 night, credited 10.0 total and 3.0 night, capped, 4.0 not credited",
		},
		{
			"zero",
			model.WeekCreditBar{Starts: monday(time.August, 3)},
			"Week of Aug 3, 2026: logged 0.0 total and 0.0 night, credited 0.0 total and 0.0 night",
		},
		{
			"night-only",
			model.WeekCreditBar{Starts: monday(time.August, 3), LoggedNight: 3, CreditedNight: 3},
			"Week of Aug 3, 2026: logged 0.0 total and 3.0 night, credited 0.0 total and 3.0 night",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekBarTitle(tt.week); got != tt.want {
				t.Fatalf("weekBarTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeekChartSummary(t *testing.T) {
	tests := []struct {
		name  string
		chart model.WeekCreditChart
		want  string
	}{
		{
			name: "empty weeks",
			chart: model.WeekCreditChart{
				Weeks: []model.WeekCreditBar{{Starts: monday(time.August, 3)}, {Starts: monday(time.August, 10)}},
			},
			want: "These 2 weeks credit 0.0 total and 0.0 night, the same weekly credited total shown above.",
		},
		{
			name: "uncapped window matches the totals",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{uncappedWeek(monday(time.August, 3))},
				WindowTotal: 5, WindowNight: 2, TotalHours: 5, NightHours: 2,
			},
			want: "These 1 weeks credit 5.0 total and 2.0 night, the same weekly credited total shown above.",
		},
		{
			name: "capped weeks list the lost hours",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{cappedWeek(monday(time.August, 3)), cappedWeek(monday(time.August, 10))},
				WindowTotal: 20, WindowNight: 6, TotalHours: 20, NightHours: 6,
			},
			want: "These 2 weeks credit 20.0 total and 6.0 night, the same weekly credited total shown above." +
				" Capped: week of Aug 3, 2026 (lost 4.0); week of Aug 10, 2026 (lost 4.0).",
		},
		{
			name: "night-only window",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{{Starts: monday(time.August, 3), LoggedNight: 3, CreditedNight: 3}},
				WindowNight: 3, NightHours: 3,
			},
			want: "These 1 weeks credit 0.0 total and 3.0 night, the same weekly credited total shown above.",
		},
		{
			name: "hours outside the window are itemized",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{uncappedWeek(monday(time.August, 3))},
				WindowTotal: 5, WindowNight: 2,
				EarlierTotal: 4, EarlierNight: 1,
				LaterTotal: 1.5, LaterNight: 0.5,
				UndatedTotal: 3, UndatedNight: 1,
				TotalHours: 13.5, NightHours: 4.5,
			},
			want: "These 1 weeks credit 5.0 total and 2.0 night." +
				" Earlier entry weeks credit 4.0 total and 1.0 night." +
				" Later entry weeks credit 1.5 total and 0.5 night." +
				" Undated opening hours add 3.0 total and 1.0 night." +
				" Together this is 13.5 total and 4.5 night, the same weekly credited total shown above.",
		},
		{
			name: "night hours alone outside the window still count as outside",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{uncappedWeek(monday(time.August, 3))},
				WindowTotal: 5, WindowNight: 2,
				EarlierNight: 1,
				TotalHours:   5, NightHours: 3,
			},
			want: "These 1 weeks credit 5.0 total and 2.0 night." +
				" Earlier entry weeks credit 0.0 total and 1.0 night." +
				" Together this is 5.0 total and 3.0 night, the same weekly credited total shown above.",
		},
		{
			name: "window that differs from the totals is explained",
			chart: model.WeekCreditChart{
				Weeks:       []model.WeekCreditBar{uncappedWeek(monday(time.August, 3))},
				WindowTotal: 5, WindowNight: 2, TotalHours: 6, NightHours: 2,
			},
			want: "These 1 weeks credit 5.0 total and 2.0 night." +
				" Together this is 6.0 total and 2.0 night, the same weekly credited total shown above.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekChartSummary(tt.chart); got != tt.want {
				t.Fatalf("weekChartSummary() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestWeekChartTitle(t *testing.T) {
	tests := []struct {
		name  string
		weeks []model.WeekCreditBar
		want  string
	}{
		{"no weeks", nil, "Logged and credited hours for the last 0 entry weeks. No week hit the cap."},
		{"uncapped", []model.WeekCreditBar{uncappedWeek(monday(time.August, 3)), {Starts: monday(time.August, 10)}}, "Logged and credited hours for the last 2 entry weeks. No week hit the cap."},
		{"one capped", []model.WeekCreditBar{uncappedWeek(monday(time.August, 3)), cappedWeek(monday(time.August, 10))}, "Logged and credited hours for the last 2 entry weeks. 1 weeks hit the 10-hour cap."},
		{"two capped", []model.WeekCreditBar{cappedWeek(monday(time.August, 3)), cappedWeek(monday(time.August, 10))}, "Logged and credited hours for the last 2 entry weeks. 2 weeks hit the 10-hour cap."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weekChartTitle(model.WeekCreditChart{Weeks: tt.weeks}); got != tt.want {
				t.Fatalf("weekChartTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeekChartNoteAndLabels(t *testing.T) {
	chart := model.WeekCreditChart{Weeks: []model.WeekCreditBar{{}, {}, {}}}
	want := "Wide bars are total hours and narrow bars are night hours, for the last 3 Monday–Sunday entry weeks. Filled color is credited. Pale color above it was logged and did not count. A red mark is a week over the 10-hour cap."
	if got := weekChartNote(chart); got != want {
		t.Errorf("weekChartNote() = %q, want %q", got, want)
	}
	if got := cappedLabel(true); got != "Yes" {
		t.Errorf("cappedLabel(true) = %q, want Yes", got)
	}
	if got := cappedLabel(false); got != "No" {
		t.Errorf("cappedLabel(false) = %q, want No", got)
	}
	starts := monday(time.August, 3)
	if got := weekStart(model.WeekCreditBar{Starts: starts}); got == nil || !got.Equal(starts) {
		t.Errorf("weekStart() = %v, want %v", got, starts)
	}
}
