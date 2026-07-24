package render

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	// The daylight-saving test names real IANA zones, so the test binary
	// carries the database rather than depending on the host having one.
	_ "time/tzdata"

	"github.com/muesli/termenv"

	"github.com/philipf/suntimes/internal/sun"
)

// at builds a Moment for a clock time on a date, in a zone — the fixture a
// hand-written expected row is easiest to read against.
func at(t *testing.T, date sun.Date, clock string, zone *time.Location) sun.Moment {
	t.Helper()

	parsed, err := time.Parse("15:04:05", clock)
	if err != nil {
		t.Fatalf("unparsable fixture time %q: %v", clock, err)
	}
	return sun.At(time.Date(date.Year, date.Month, date.Day,
		parsed.Hour(), parsed.Minute(), parsed.Second(), 0, zone))
}

// escapePattern matches an ANSI SGR sequence — the colour and attribute codes
// SUN-26 forbids in redirected output. Tests match on it directly rather than
// through a stripping library, so the thing being asserted about is the bytes
// themselves.
var escapePattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// reverseVideo is the escape the theme highlights today's row with. A test that
// looked only for "some escape" could not tell a highlighted row from a
// coloured one.
const reverseVideo = "\x1b[7m"

// parsed is a rendered table taken apart again: its heading cells, its data
// cells, and the raw lines the data rows came from, which are what the styled
// tests need — the styling lives between the cells, not in them.
type parsed struct {
	headings []string
	rows     [][]string
	rawRows  []string
	lines    []string
}

// parseTable reads a rendered table back into cells. Escapes are stripped
// first, so the same parser reads a plain table and a themed one and the
// assertions about content do not have to care which they were given.
func parseTable(t *testing.T, rendered string) parsed {
	t.Helper()

	var out parsed
	out.lines = strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")

	for _, line := range out.lines {
		plain := escapePattern.ReplaceAllString(line, "")
		if !strings.HasPrefix(plain, "│") {
			continue // a border rule, not a row of values
		}

		fields := strings.Split(strings.Trim(plain, "│"), "│")
		cells := make([]string, 0, len(fields))
		for _, field := range fields {
			cells = append(cells, strings.TrimSpace(field))
		}

		if out.headings == nil {
			out.headings = cells
			continue
		}
		out.rows = append(out.rows, cells)
		out.rawRows = append(out.rawRows, line)
	}

	if out.headings == nil {
		t.Fatalf("rendered table has no rows at all:\n%s", rendered)
	}
	return out
}

// week builds n consecutive days of plausible times starting at date, for the
// tests and the benchmark that care about shape rather than values.
func week(t *testing.T, date sun.Date, n int, zone *time.Location) []sun.Day {
	t.Helper()

	days := make([]sun.Day, 0, n)
	for offset := range n {
		on := date.AddDays(offset)
		days = append(days, sun.Day{
			Date:    on,
			Dawn:    at(t, on, "06:03:00", zone),
			Sunrise: at(t, on, "07:33:00", zone),
			Sunset:  at(t, on, "17:20:00", zone),
			Dusk:    at(t, on, "17:50:00", zone),
		})
	}
	return days
}

// render is Table into a buffer, for the tests that only want the string.
func render(t *testing.T, days []sun.Day, zone *time.Location, opts Options) string {
	t.Helper()

	var out bytes.Buffer
	if err := Table(&out, days, zone, opts); err != nil {
		t.Fatalf("Table returned error %v, want nil", err)
	}
	return out.String()
}

// SUN-21: the table is bordered and carries a header row naming the six
// columns, in the order the PRD lists them.
func TestTableHasAHeaderRowAndColumnsInOrder(t *testing.T) {
	zone := time.UTC
	days := week(t, sun.Date{Year: 2026, Month: time.July, Day: 24}, 2, zone)

	out := render(t, days, zone, Options{})
	table := parseTable(t, out)

	want := []string{"Date", "Day", "Dawn", "Sunrise", "Sunset", "Dusk"}
	if strings.Join(table.headings, "|") != strings.Join(want, "|") {
		t.Errorf("headings are %v, want %v\n---\n%s", table.headings, want, out)
	}
	if len(table.rows) != len(days) {
		t.Errorf("table has %d data rows, want %d\n---\n%s", len(table.rows), len(days), out)
	}

	// A header that is not separated from the data is not a header. The four
	// rules are the top border, the header rule, and the bottom border — plus
	// the vertical bars every row already proved.
	for _, want := range []string{"┌", "├", "└", "│"} {
		if !strings.Contains(out, want) {
			t.Errorf("table has no %q, want a bordered grid\n---\n%s", want, out)
		}
	}
}

// SUN-21, SUN-22, SUN-23: the cells of a row are date, weekday, dawn, sunrise,
// sunset and dusk, with a YYYY-MM-DD date, a three-letter weekday and 24-hour
// HH:MM times.
//
// The four times are deliberately distinct and out of clock order for sunset
// and dusk, so a row that reordered or duplicated a column could not pass.
func TestCellsOrderColumnsAndFormatValues(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	zone := time.UTC

	day := sun.Day{
		Date:    date,
		Dawn:    at(t, date, "01:11:00", zone),
		Sunrise: at(t, date, "02:22:00", zone),
		Sunset:  at(t, date, "13:33:00", zone),
		Dusk:    at(t, date, "14:44:00", zone),
	}

	got := strings.Join(Cells(day, zone), "|")
	want := "2026-07-24|Fri|01:11|02:22|13:33|14:44"
	if got != want {
		t.Errorf("Cells() =\n%q\nwant\n%q", got, want)
	}
}

// SUN-22: cells are rounded to the nearest minute, the way published almanacs
// print them, rather than truncated — which would report every time as up to a
// minute early.
func TestCellsRoundToTheNearestMinute(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	zone := time.UTC

	day := sun.Day{
		Date:    date,
		Dawn:    at(t, date, "06:57:58", zone), // rounds up
		Sunrise: at(t, date, "07:26:29", zone), // rounds down
		Sunset:  at(t, date, "17:28:35", zone), // rounds up
		Dusk:    at(t, date, "17:56:00", zone), // exact
	}

	got := strings.Join(Cells(day, zone), "|")
	want := "2026-07-24|Fri|06:58|07:26|17:29|17:56"
	if got != want {
		t.Errorf("Cells() =\n%q\nwant\n%q", got, want)
	}
}

// SUN-7, SUN-8: the instants are absolute, so the zone the row is rendered in
// decides the clock times.
func TestCellsRenderInTheGivenZone(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	moment := at(t, date, "12:00:00", time.UTC)

	day := sun.Day{Date: date, Dawn: moment, Sunrise: moment, Sunset: moment, Dusk: moment}

	if got, want := strings.Join(Cells(day, time.UTC), "|"),
		"2026-07-24|Fri|12:00|12:00|12:00|12:00"; got != want {
		t.Errorf("Cells() in UTC = %q, want %q", got, want)
	}
	east := time.FixedZone("UTC+12", 12*60*60)
	if got, want := strings.Join(Cells(day, east), "|"),
		"2026-07-24|Fri|00:00|00:00|00:00|00:00"; got != want {
		t.Errorf("Cells() in UTC+12 = %q, want %q", got, want)
	}
}

// SUN-26: output that is not going to a terminal carries no ANSI escape codes
// at all, so it stays greppable and diffable. The assertion is on the bytes,
// not on how the table looks: an escape anywhere in the output fails it.
//
// Today is deliberately inside the range, so a renderer that highlighted
// regardless of style would be caught here rather than passing by accident.
func TestPlainTableContainsNoEscapeCodes(t *testing.T) {
	zone := time.UTC
	today := sun.Date{Year: 2026, Month: time.July, Day: 24}
	days := week(t, today, 7, zone)
	days[3].Dawn = sun.Never() // the placeholder path is styled too

	styles := map[string]Style{
		"zero value": {},
		"plain":      Plain(),
	}

	for name, style := range styles {
		t.Run(name, func(t *testing.T) {
			out := []byte(render(t, days, zone, Options{Today: today, Style: style}))

			if index := bytes.IndexByte(out, 0x1b); index >= 0 {
				t.Fatalf("escape byte at offset %d of plain output:\n%q", index, out)
			}
			if escapePattern.Match(out) {
				t.Fatalf("plain output matches %s:\n%q", escapePattern, out)
			}
		})
	}
}

// SUN-25, SUN-27a: a themed table is styled, and today's row is styled
// differently from every other row.
//
// The rows are compared by the escapes around them rather than by "row 0 looks
// special", because which row is today is a property of the dates, not of the
// position.
func TestThemedTableStylesTheTableAndHighlightsToday(t *testing.T) {
	zone := time.UTC
	start := sun.Date{Year: 2026, Month: time.July, Day: 24}
	today := start.AddDays(2) // not the first row, and not the last
	days := week(t, start, 5, zone)

	out := render(t, days, zone, Options{Today: today, Style: Themed()})
	if !escapePattern.MatchString(out) {
		t.Fatalf("themed output carries no escape codes at all:\n%q", out)
	}

	table := parseTable(t, out)
	if len(table.rawRows) != len(days) {
		t.Fatalf("table has %d data rows, want %d\n---\n%s", len(table.rawRows), len(days), out)
	}

	for index, raw := range table.rawRows {
		highlighted := strings.Contains(raw, reverseVideo)
		isToday := days[index].Date == today

		if highlighted != isToday {
			t.Errorf("row %d (%s) highlighted = %t, want %t (today is %s)\n%q",
				index, days[index].Date, highlighted, isToday, today, raw)
		}
	}

	// The values themselves survive the styling — a highlight that ate the text
	// would satisfy every assertion above.
	if got, want := table.rows[2][0], today.String(); got != want {
		t.Errorf("highlighted row reads date %q, want %q", got, want)
	}
}

// SUN-25: today need not be in the range at all — `--date 2028-02-29` and a
// range that ended last week both leave it out. Nothing is highlighted then,
// and in particular the first row is not highlighted on the assumption that it
// must be today.
func TestThemedTableHighlightsNothingWhenTodayIsAbsent(t *testing.T) {
	zone := time.UTC
	start := sun.Date{Year: 2026, Month: time.July, Day: 24}
	days := week(t, start, 3, zone)

	todays := map[string]sun.Date{
		"today is after the range":  start.AddDays(30),
		"today is before the range": start.AddDays(-30),
		"today was never set":       {},
	}

	for name, today := range todays {
		t.Run(name, func(t *testing.T) {
			out := render(t, days, zone, Options{Today: today, Style: Themed()})

			table := parseTable(t, out)
			for index, raw := range table.rawRows {
				if strings.Contains(raw, reverseVideo) {
					t.Errorf("row %d is highlighted with today = %s:\n%q", index, today, raw)
				}
			}
			// The table is still themed; only the highlight is absent.
			if !escapePattern.MatchString(out) {
				t.Errorf("themed output carries no escape codes at all:\n%q", out)
			}
		})
	}
}

// SUN-27b: a terminal that reports fewer colours than the theme asks for gets
// codes it can actually show, not 256-colour ones it would print as garbage.
// The theme steps down; the table does not change shape.
//
// The 4-bit profile is built here rather than detected, because detection needs
// a terminal and the point of the test is the rendering, not the sniffing.
func TestThemedTableDegradesForALimitedColourTerminal(t *testing.T) {
	zone := time.UTC
	today := sun.Date{Year: 2026, Month: time.July, Day: 24}
	days := week(t, today, 3, zone)

	limited := Style{colour: true, profile: termenv.ANSI}
	out := render(t, days, zone, Options{Today: today, Style: limited})

	if !escapePattern.MatchString(out) {
		t.Fatalf("a 16-colour terminal was given no styling at all:\n%q", out)
	}
	// "38;5;" introduces a 256-colour foreground. A terminal that only knows 16
	// would show the digits.
	if strings.Contains(out, "38;5;") {
		t.Errorf("256-colour codes sent to a 16-colour terminal:\n%q", out)
	}
	if !strings.Contains(out, reverseVideo) {
		t.Errorf("today is not highlighted on a 16-colour terminal:\n%q", out)
	}

	// Styling is decoration: strip it and the table is the plain one, to the
	// character. This is what "the table remains legible" has to mean.
	plain := render(t, days, zone, Options{Today: today, Style: Plain()})
	if got := escapePattern.ReplaceAllString(out, ""); got != plain {
		t.Errorf("stripped of styling, the themed table reads\n%s\nwant\n%s", got, plain)
	}
}

// SUN-27: an event that does not occur renders as the placeholder rather than a
// misleading time, and the row it is in stays the same width as every other —
// the placeholder is one character but three bytes, and a table that measured
// bytes would draw a ragged grid. Issue #7 adds the detection that produces
// these; this is what proves the layout is already ready for them.
func TestTableRendersAbsentEventsAsAlignedPlaceholders(t *testing.T) {
	zone := time.UTC
	date := sun.Date{Year: 2026, Month: time.June, Day: 21}
	moment := at(t, date, "05:00:00", zone)

	days := []sun.Day{{
		Date: date, Dawn: moment, Sunrise: moment, Sunset: moment, Dusk: moment,
	}, {
		Date:    date.AddDays(1),
		Dawn:    sun.Never(),
		Sunrise: at(t, date, "02:30:00", zone),
		Sunset:  at(t, date, "23:30:00", zone),
		Dusk:    sun.Never(),
	}, {
		Date:    date.AddDays(2),
		Dawn:    sun.Never(),
		Sunrise: sun.Never(),
		Sunset:  sun.Never(),
		Dusk:    sun.Never(),
	}}

	out := render(t, days, zone, Options{})
	table := parseTable(t, out)

	wants := [][]string{
		{"2026-06-21", "Sun", "05:00", "05:00", "05:00", "05:00"},
		{"2026-06-22", "Mon", Placeholder, "02:30", "23:30", Placeholder},
		{"2026-06-23", "Tue", Placeholder, Placeholder, Placeholder, Placeholder},
	}
	for index, want := range wants {
		if got := strings.Join(table.rows[index], "|"); got != strings.Join(want, "|") {
			t.Errorf("row %d = %q, want %q", index, got, strings.Join(want, "|"))
		}
	}
	if strings.Contains(out, "00:00") {
		t.Errorf("an absent event printed a time:\n%s", out)
	}
	assertRectangular(t, out)
}

// A table of one row and a table of many are both drawn correctly: nothing in
// the layout assumes the default week.
func TestTableRendersAnyNumberOfRows(t *testing.T) {
	zone := time.UTC
	start := sun.Date{Year: 2026, Month: time.July, Day: 24}

	for _, count := range []int{1, 7, 400} {
		days := week(t, start, count, zone)
		out := render(t, days, zone, Options{Today: start.AddDays(count / 2)})

		table := parseTable(t, out)
		if len(table.rows) != count {
			t.Errorf("a %d-day table has %d data rows\n---\n%s", count, len(table.rows), out)
			continue
		}
		// Top border, header, header rule, the rows, bottom border.
		if want := count + 4; len(table.lines) != want {
			t.Errorf("a %d-day table is %d lines, want %d", count, len(table.lines), want)
		}
		for index, row := range table.rows {
			if got, want := row[0], days[index].Date.String(); got != want {
				t.Errorf("row %d of the %d-day table is for %s, want %s", index, count, got, want)
			}
		}
		assertRectangular(t, out)
	}
}

// assertRectangular checks every line of a table is the same display width.
// Every character the table can contain — box-drawing rules, digits and the
// em-dash placeholder — is one cell wide, so counting runes is counting cells.
func assertRectangular(t *testing.T, rendered string) {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	width := utf8.RuneCountInString(lines[0])
	for index, line := range lines {
		if got := utf8.RuneCountInString(line); got != width {
			t.Errorf("line %d is %d columns wide, line 0 is %d:\n%s", index, got, width, rendered)
		}
	}
}

// dstDay is one day of a range that spans a daylight-saving transition: the
// calendar date the row is about, its four event instants written in UTC, and
// the cells those instants must render as in the zone under test.
//
// The instants are UTC because that is what the calculation produces — an
// absolute moment, with no opinion about anybody's clock. Writing the fixture
// in the display zone would assume the answer.
type dstDay struct {
	date                          string
	dawn, sunrise, sunset, dusk   string
	want                          string // the row's cells, joined with "|"
	sunriseShiftFromPreviousInMin int    // 0 on the first day of the range
}

// SUN-10: a range spanning a daylight-saving transition shows the times a
// person in that zone would read off a clock. The renderer does no arithmetic
// of its own — the hour comes from the zone's rules applied to an absolute
// instant — so the printed clock jumps by an hour on the transition day even
// though consecutive sunrises are only a minute or two apart.
//
// Both directions are covered: London's clocks go back on 25 October 2026 (BST
// to GMT at 02:00 local, when 01:00 UTC comes round) and Auckland's go forward
// on 27 September 2026 (NZST to NZDT at 02:00 local, at 14:00 UTC the day
// before). A tool that stored a fixed offset instead of a zone would print the
// first day of each range correctly and every day after it an hour out.
func TestTableFollowsDaylightSavingTransition(t *testing.T) {
	tests := map[string]struct {
		zone string
		days []dstDay
	}{
		"clocks go back in London": {
			zone: "Europe/London",
			days: []dstDay{{
				date: "2026-10-24",
				dawn: "2026-10-24T06:05:00Z", sunrise: "2026-10-24T06:39:00Z",
				sunset: "2026-10-24T16:50:00Z", dusk: "2026-10-24T17:24:00Z",
				want: "2026-10-24|Sat|07:05|07:39|17:50|18:24", // BST, UTC+1
			}, {
				date: "2026-10-25",
				dawn: "2026-10-25T06:07:00Z", sunrise: "2026-10-25T06:41:00Z",
				sunset: "2026-10-25T16:48:00Z", dusk: "2026-10-25T17:22:00Z",
				want: "2026-10-25|Sun|06:07|06:41|16:48|17:22", // GMT, UTC+0
				// The sun rose two minutes later than the day before, but the
				// clock had gone back an hour, so the printed time is 58
				// minutes earlier.
				sunriseShiftFromPreviousInMin: -58,
			}, {
				date: "2026-10-26",
				dawn: "2026-10-26T06:08:00Z", sunrise: "2026-10-26T06:43:00Z",
				sunset: "2026-10-26T16:46:00Z", dusk: "2026-10-26T17:20:00Z",
				want:                          "2026-10-26|Mon|06:08|06:43|16:46|17:20", // GMT, and stays there
				sunriseShiftFromPreviousInMin: 2,
			}},
		},
		"clocks go forward in Auckland": {
			zone: "Pacific/Auckland",
			days: []dstDay{{
				date: "2026-09-26",
				dawn: "2026-09-25T17:39:00Z", sunrise: "2026-09-25T18:05:00Z",
				sunset: "2026-09-26T06:20:00Z", dusk: "2026-09-26T06:46:00Z",
				want: "2026-09-26|Sat|05:39|06:05|18:20|18:46", // NZST, UTC+12
			}, {
				date: "2026-09-27",
				dawn: "2026-09-26T17:38:00Z", sunrise: "2026-09-26T18:04:00Z",
				sunset: "2026-09-27T06:21:00Z", dusk: "2026-09-27T06:46:00Z",
				want: "2026-09-27|Sun|06:38|07:04|19:21|19:46", // NZDT, UTC+13
				// A minute earlier than the day before, an hour later on the
				// clock.
				sunriseShiftFromPreviousInMin: 59,
			}, {
				date: "2026-09-28",
				dawn: "2026-09-27T17:36:00Z", sunrise: "2026-09-27T18:02:00Z",
				sunset: "2026-09-28T06:21:00Z", dusk: "2026-09-28T06:47:00Z",
				want:                          "2026-09-28|Mon|06:36|07:02|19:21|19:47", // NZDT, and stays there
				sunriseShiftFromPreviousInMin: -2,
			}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			zone := mustLoadLocation(t, test.zone)

			days := make([]sun.Day, 0, len(test.days))
			for _, day := range test.days {
				days = append(days, sun.Day{
					Date:    mustParseDate(t, day.date),
					Dawn:    sun.At(mustParseInstant(t, day.dawn)),
					Sunrise: sun.At(mustParseInstant(t, day.sunrise)),
					Sunset:  sun.At(mustParseInstant(t, day.sunset)),
					Dusk:    sun.At(mustParseInstant(t, day.dusk)),
				})
			}

			out := render(t, days, zone, Options{})
			table := parseTable(t, out)
			if len(table.rows) != len(test.days) {
				t.Fatalf("table has %d data rows, want %d:\n%s", len(table.rows), len(test.days), out)
			}

			for index, day := range test.days {
				if got := strings.Join(table.rows[index], "|"); got != day.want {
					t.Errorf("row %d =\n%q\nwant\n%q", index, got, day.want)
				}
				if index == 0 {
					continue
				}
				// The rows above already pin the answer; this states the point
				// of the fixture, so a failure says "the clock did not change"
				// rather than only "row 1 differs".
				got := sunriseCell(t, table.rows[index]) - sunriseCell(t, table.rows[index-1])
				if got != day.sunriseShiftFromPreviousInMin {
					t.Errorf("printed sunrise moved %+d minutes from %s to %s, want %+d",
						got, test.days[index-1].date, day.date, day.sunriseShiftFromPreviousInMin)
				}
			}
		})
	}
}

// sunriseCell reads the sunrise column out of a parsed row as minutes since
// midnight, so two rows can be compared as a person compares two clock times.
func sunriseCell(t *testing.T, row []string) int {
	t.Helper()

	const sunriseColumn = 3 // date, day, dawn, sunrise, …
	if len(row) <= sunriseColumn {
		t.Fatalf("row %v has %d columns, want at least %d", row, len(row), sunriseColumn+1)
	}

	hours, minutes, found := strings.Cut(row[sunriseColumn], ":")
	if !found {
		t.Fatalf("sunrise cell %q in row %v is not HH:MM", row[sunriseColumn], row)
	}
	hour, hourErr := strconv.Atoi(hours)
	minute, minuteErr := strconv.Atoi(minutes)
	if hourErr != nil || minuteErr != nil {
		t.Fatalf("unparsable sunrise cell %q in row %v", row[sunriseColumn], row)
	}
	return hour*60 + minute
}

// mustLoadLocation resolves an IANA name the test itself depends on.
func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	zone, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("loading timezone %q: %v", name, err)
	}
	return zone
}

// mustParseInstant reads an RFC 3339 fixture instant.
func mustParseInstant(t *testing.T, value string) time.Time {
	t.Helper()

	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("unparsable fixture instant %q: %v", value, err)
	}
	return instant
}

// mustParseDate reads a YYYY-MM-DD fixture date.
func mustParseDate(t *testing.T, value string) sun.Date {
	t.Helper()

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("unparsable fixture date %q: %v", value, err)
	}
	return sun.Date{Year: parsed.Year(), Month: parsed.Month(), Day: parsed.Day()}
}

// SUN-26: Detect answers "plain" for anything that is not a terminal, which is
// what a redirected run and every test hands it.
func TestDetectIsPlainForSomethingThatIsNotATerminal(t *testing.T) {
	if style := Detect(&bytes.Buffer{}); style.Styles() {
		t.Errorf("Detect(buffer) asks for styling, want plain")
	}
	if Plain().Styles() {
		t.Errorf("Plain() asks for styling")
	}
	if !Themed().Styles() {
		t.Errorf("Themed() does not ask for styling")
	}
}

// A write failure is reported rather than swallowed.
func TestTableReportsWriteErrors(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	day := sun.Day{Date: date}

	err := Table(failingWriter{}, []sun.Day{day}, time.UTC, Options{})
	if !errors.Is(err, errWriteFailed) {
		t.Errorf("Table returned error %v, want %v", err, errWriteFailed)
	}
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

// NFR-3: a seven-day table renders in well under 100 ms. The budget covers the
// whole run, so the layout has to be a rounding error inside it.
func BenchmarkSevenDayTable(b *testing.B) {
	zone := time.UTC
	today := sun.Date{Year: 2026, Month: time.July, Day: 24}

	days := make([]sun.Day, 0, 7)
	for offset := range 7 {
		on := today.AddDays(offset)
		days = append(days, sun.Day{
			Date:    on,
			Dawn:    sun.At(time.Date(on.Year, on.Month, on.Day, 6, 3, 0, 0, zone)),
			Sunrise: sun.At(time.Date(on.Year, on.Month, on.Day, 7, 33, 0, 0, zone)),
			Sunset:  sun.At(time.Date(on.Year, on.Month, on.Day, 17, 20, 0, 0, zone)),
			Dusk:    sun.At(time.Date(on.Year, on.Month, on.Day, 17, 50, 0, 0, zone)),
		})
	}

	styles := map[string]Style{"plain": Plain(), "themed": Themed()}
	for name, style := range styles {
		b.Run(name, func(b *testing.B) {
			opts := Options{Today: today, Style: style}
			b.ReportAllocs()
			for b.Loop() {
				if err := Table(io.Discard, days, zone, opts); err != nil {
					b.Fatalf("Table returned error %v, want nil", err)
				}
			}
		})
	}
}
