package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

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
