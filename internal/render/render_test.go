package render

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	// The daylight-saving test names real IANA zones, so the test binary
	// carries the database rather than depending on the host having one.
	_ "time/tzdata"

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

// SUN-21, SUN-22, SUN-23: one row, columns in the order Date, Day, Dawn,
// Sunrise, Sunset, Dusk, with a YYYY-MM-DD date, a three-letter weekday and
// 24-hour HH:MM times.
//
// The four times are deliberately distinct and out of clock order for sunset
// and dusk, so a row that reordered or duplicated a column could not pass.
func TestRowOrdersColumnsAndFormatsCells(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	zone := time.UTC

	day := sun.Day{
		Date:    date,
		Dawn:    at(t, date, "01:11:00", zone),
		Sunrise: at(t, date, "02:22:00", zone),
		Sunset:  at(t, date, "13:33:00", zone),
		Dusk:    at(t, date, "14:44:00", zone),
	}

	got := Row(day, zone)
	want := "2026-07-24  Fri  01:11  02:22  13:33  14:44"
	if got != want {
		t.Errorf("Row() =\n%q\nwant\n%q", got, want)
	}
}

// SUN-22: cells are rounded to the nearest minute, the way published almanacs
// print them, rather than truncated — which would report every time as up to a
// minute early.
func TestRowRoundsToTheNearestMinute(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	zone := time.UTC

	day := sun.Day{
		Date:    date,
		Dawn:    at(t, date, "06:57:58", zone), // rounds up
		Sunrise: at(t, date, "07:26:29", zone), // rounds down
		Sunset:  at(t, date, "17:28:35", zone), // rounds up
		Dusk:    at(t, date, "17:56:00", zone), // exact
	}

	got := Row(day, zone)
	want := "2026-07-24  Fri  06:58  07:26  17:29  17:56"
	if got != want {
		t.Errorf("Row() =\n%q\nwant\n%q", got, want)
	}
}

// SUN-7, SUN-8: the instants are absolute, so the zone the row is rendered in
// decides the clock times. This is the seam issue #4 turns on the configured
// timezone.
func TestRowRendersInTheGivenZone(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	moment := at(t, date, "12:00:00", time.UTC)

	day := sun.Day{Date: date, Dawn: moment, Sunrise: moment, Sunset: moment, Dusk: moment}

	if got, want := Row(day, time.UTC), "2026-07-24  Fri  12:00  12:00  12:00  12:00"; got != want {
		t.Errorf("Row() in UTC = %q, want %q", got, want)
	}
	east := time.FixedZone("UTC+12", 12*60*60)
	if got, want := Row(day, east), "2026-07-24  Fri  00:00  00:00  00:00  00:00"; got != want {
		t.Errorf("Row() in UTC+12 = %q, want %q", got, want)
	}
}

// dstDay is one day of a range that spans a daylight-saving transition: the
// calendar date the row is about, its four event instants written in UTC, and
// the row those instants must render as in the zone under test.
//
// The instants are UTC because that is what the calculation produces — an
// absolute moment, with no opinion about anybody's clock. Writing the fixture
// in the display zone would assume the answer.
type dstDay struct {
	date                          string
	dawn, sunrise, sunset, dusk   string
	want                          string
	sunriseShiftFromPreviousInMin int // 0 on the first day of the range
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
				want: "2026-10-24  Sat  07:05  07:39  17:50  18:24", // BST, UTC+1
			}, {
				date: "2026-10-25",
				dawn: "2026-10-25T06:07:00Z", sunrise: "2026-10-25T06:41:00Z",
				sunset: "2026-10-25T16:48:00Z", dusk: "2026-10-25T17:22:00Z",
				want: "2026-10-25  Sun  06:07  06:41  16:48  17:22", // GMT, UTC+0
				// The sun rose two minutes later than the day before, but the
				// clock had gone back an hour, so the printed time is 58
				// minutes earlier.
				sunriseShiftFromPreviousInMin: -58,
			}, {
				date: "2026-10-26",
				dawn: "2026-10-26T06:08:00Z", sunrise: "2026-10-26T06:43:00Z",
				sunset: "2026-10-26T16:46:00Z", dusk: "2026-10-26T17:20:00Z",
				want:                          "2026-10-26  Mon  06:08  06:43  16:46  17:20", // GMT, and stays there
				sunriseShiftFromPreviousInMin: 2,
			}},
		},
		"clocks go forward in Auckland": {
			zone: "Pacific/Auckland",
			days: []dstDay{{
				date: "2026-09-26",
				dawn: "2026-09-25T17:39:00Z", sunrise: "2026-09-25T18:05:00Z",
				sunset: "2026-09-26T06:20:00Z", dusk: "2026-09-26T06:46:00Z",
				want: "2026-09-26  Sat  05:39  06:05  18:20  18:46", // NZST, UTC+12
			}, {
				date: "2026-09-27",
				dawn: "2026-09-26T17:38:00Z", sunrise: "2026-09-26T18:04:00Z",
				sunset: "2026-09-27T06:21:00Z", dusk: "2026-09-27T06:46:00Z",
				want: "2026-09-27  Sun  06:38  07:04  19:21  19:46", // NZDT, UTC+13
				// A minute earlier than the day before, an hour later on the
				// clock.
				sunriseShiftFromPreviousInMin: 59,
			}, {
				date: "2026-09-28",
				dawn: "2026-09-27T17:36:00Z", sunrise: "2026-09-27T18:02:00Z",
				sunset: "2026-09-28T06:21:00Z", dusk: "2026-09-28T06:47:00Z",
				want:                          "2026-09-28  Mon  06:36  07:02  19:21  19:47", // NZDT, and stays there
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

			var out bytes.Buffer
			if err := Table(&out, days, zone); err != nil {
				t.Fatalf("Table returned error %v, want nil", err)
			}

			rows := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			if len(rows) != len(test.days) {
				t.Fatalf("Table wrote %d rows, want %d:\n%s", len(rows), len(test.days), out.String())
			}

			for index, day := range test.days {
				if rows[index] != day.want {
					t.Errorf("row %d =\n%q\nwant\n%q", index, rows[index], day.want)
				}
				if index == 0 {
					continue
				}
				// The rows above already pin the answer; this states the point
				// of the fixture, so a failure says "the clock did not change"
				// rather than only "row 1 differs".
				got := sunriseCell(t, rows[index]) - sunriseCell(t, rows[index-1])
				if got != day.sunriseShiftFromPreviousInMin {
					t.Errorf("printed sunrise moved %+d minutes from %s to %s, want %+d",
						got, test.days[index-1].date, day.date, day.sunriseShiftFromPreviousInMin)
				}
			}
		})
	}
}

// sunriseCell reads the sunrise column out of a rendered row as minutes since
// midnight, so two rows can be compared as a person compares two clock times.
func sunriseCell(t *testing.T, row string) int {
	t.Helper()

	const sunriseColumn = 3 // date, day, dawn, sunrise, …
	cells := strings.Split(row, columnGap)
	if len(cells) <= sunriseColumn {
		t.Fatalf("row %q has %d columns, want at least %d", row, len(cells), sunriseColumn+1)
	}

	hours, minutes, found := strings.Cut(cells[sunriseColumn], ":")
	if !found {
		t.Fatalf("sunrise cell %q in row %q is not HH:MM", cells[sunriseColumn], row)
	}
	hour, hourErr := strconv.Atoi(hours)
	minute, minuteErr := strconv.Atoi(minutes)
	if hourErr != nil || minuteErr != nil {
		t.Fatalf("unparsable sunrise cell %q in row %q", cells[sunriseColumn], row)
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

// SUN-27: an event that does not occur renders as the placeholder rather than
// a misleading time. Nothing routinely produces an absent Moment yet — issue
// #7 adds the detection — so this is what proves the renderer will not panic or
// print midnight in year 1 when it does.
func TestRowRendersAbsentEventsAsPlaceholder(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.June, Day: 21}
	zone := time.UTC

	t.Run("every event absent", func(t *testing.T) {
		day := sun.Day{
			Date:    date,
			Dawn:    sun.Never(),
			Sunrise: sun.Never(),
			Sunset:  sun.Never(),
			Dusk:    sun.Never(),
		}

		got := Row(day, zone)
		want := "2026-06-21  Sun  " + Placeholder + "      " + Placeholder + "      " + Placeholder + "      " + Placeholder
		if got != want {
			t.Errorf("Row() =\n%q\nwant\n%q", got, want)
		}
		if strings.Contains(got, "00:00") {
			t.Errorf("an absent event printed a time:\n%q", got)
		}
	})

	t.Run("mixed with present events", func(t *testing.T) {
		day := sun.Day{
			Date:    date,
			Dawn:    sun.Never(),
			Sunrise: at(t, date, "02:30:00", zone),
			Sunset:  at(t, date, "23:30:00", zone),
			Dusk:    sun.Never(),
		}

		got := Row(day, zone)
		want := "2026-06-21  Sun  " + Placeholder + "      02:30  23:30  " + Placeholder
		if got != want {
			t.Errorf("Row() =\n%q\nwant\n%q", got, want)
		}
	})
}

// A placeholder is one character but three bytes; padding it by bytes would
// leave the cell two display columns short and shift every column after it.
// A row with a placeholder must therefore be exactly as wide as one without,
// and the cells that follow must sit at the same offsets.
func TestPlaceholderKeepsColumnsAligned(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	zone := time.UTC
	moment := at(t, date, "05:00:00", zone)

	present := []rune(Row(sun.Day{
		Date: date, Dawn: moment, Sunrise: moment, Sunset: moment, Dusk: moment,
	}, zone))
	absent := []rune(Row(sun.Day{
		Date: date, Dawn: sun.Never(), Sunrise: moment, Sunset: moment, Dusk: moment,
	}, zone))

	if len(absent) != len(present) {
		t.Fatalf("a row with a placeholder is %d columns wide, one without is %d\n%s\n%s",
			len(absent), len(present), string(present), string(absent))
	}

	// Only the dawn cell differs, so the tail of both rows must match rune for
	// rune from the first column the two rows agree on again.
	tail := len(present) - len("05:00  05:00  05:00")
	if got, want := string(absent[tail:]), string(present[tail:]); got != want {
		t.Errorf("columns after the placeholder read %q, want %q", got, want)
	}
}

// SUN-24: Table writes one line per day, in the order given.
func TestTableWritesOneLinePerDay(t *testing.T) {
	zone := time.UTC
	var days []sun.Day
	for day := 24; day <= 26; day++ {
		date := sun.Date{Year: 2026, Month: time.July, Day: day}
		days = append(days, sun.Day{
			Date:    date,
			Dawn:    at(t, date, "06:00:00", zone),
			Sunrise: at(t, date, "07:00:00", zone),
			Sunset:  at(t, date, "17:00:00", zone),
			Dusk:    at(t, date, "18:00:00", zone),
		})
	}

	var out bytes.Buffer
	if err := Table(&out, days, zone); err != nil {
		t.Fatalf("Table returned error %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(days) {
		t.Fatalf("Table wrote %d lines, want %d:\n%s", len(lines), len(days), out.String())
	}
	for index, line := range lines {
		if want := days[index].Date.String(); !strings.HasPrefix(line, want) {
			t.Errorf("line %d = %q, want it to start with %q", index, line, want)
		}
	}
}

// A row is plain text: no ANSI escapes and no border characters until issue #6
// adds the themed table.
func TestRowIsPlainText(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	moment := at(t, date, "07:26:00", time.UTC)
	row := Row(sun.Day{Date: date, Dawn: moment, Sunrise: moment, Sunset: moment, Dusk: moment}, time.UTC)

	for _, unwanted := range []string{"\x1b", "│", "┌", "─"} {
		if strings.Contains(row, unwanted) {
			t.Errorf("row %q contains %q, want plain text", row, unwanted)
		}
	}
	if strings.HasSuffix(row, " ") {
		t.Errorf("row %q ends in trailing whitespace", row)
	}
}

// A write failure is reported rather than swallowed.
func TestTableReportsWriteErrors(t *testing.T) {
	date := sun.Date{Year: 2026, Month: time.July, Day: 24}
	day := sun.Day{Date: date}

	err := Table(failingWriter{}, []sun.Day{day}, time.UTC)
	if !errors.Is(err, errWriteFailed) {
		t.Errorf("Table returned error %v, want %v", err, errWriteFailed)
	}
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }
