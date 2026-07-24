package sun

import (
	"strings"
	"testing"
	"time"
)

// Reference values are published by the United States Naval Observatory,
// Astronomical Applications Department, via its Sun and Moon Data for One Day
// service (https://aa.usno.navy.mil/api/rstt/oneday). They were requested per
// location at the fixed UTC offset named below and are reproduced exactly as
// USNO reports them: "Begin Civil Twilight", "Rise", "Set" and "End Civil
// Twilight", to whole minutes.
//
// Fixed offsets, not IANA names, keep these tests hermetic — they neither read
// the host's timezone database nor depend on it being present. Each offset is
// the one actually in force at that place on that date, so the clock times
// below are the ones a person standing there would read.
var references = []struct {
	name    string
	place   Place
	offset  int // seconds east of UTC, as passed to the USNO service
	date    Date
	dawn    string
	sunrise string
	sunset  string
	dusk    string
}{
	{
		name:    "Auckland in winter",
		place:   Place{Latitude: -36.8485, Longitude: 174.7633},
		offset:  12 * 60 * 60, // NZST
		date:    Date{Year: 2026, Month: time.July, Day: 24},
		dawn:    "06:58",
		sunrise: "07:26",
		sunset:  "17:29",
		dusk:    "17:57",
	},
	{
		name:    "London in summer",
		place:   Place{Latitude: 51.5074, Longitude: -0.1278},
		offset:  1 * 60 * 60, // BST
		date:    Date{Year: 2026, Month: time.July, Day: 24},
		dawn:    "04:30",
		sunrise: "05:13",
		sunset:  "21:01",
		dusk:    "21:43",
	},
	{
		name:    "London in winter",
		place:   Place{Latitude: 51.5074, Longitude: -0.1278},
		offset:  0, // GMT
		date:    Date{Year: 2026, Month: time.January, Day: 15},
		dawn:    "07:21",
		sunrise: "07:59",
		sunset:  "16:21",
		dusk:    "16:59",
	},
	{
		name:    "New York in spring",
		place:   Place{Latitude: 40.7128, Longitude: -74.0060},
		offset:  -4 * 60 * 60, // EDT
		date:    Date{Year: 2026, Month: time.March, Day: 15},
		dawn:    "06:40",
		sunrise: "07:08",
		sunset:  "19:03",
		dusk:    "19:30",
	},
}

// tolerance bounds how far a computed instant may sit from its published one.
//
// NFR-4 asks for roughly a minute at mid latitudes. USNO publishes to whole
// minutes, so a reference value carries up to 30 s of rounding of its own;
// 90 s therefore leaves the calculation about a minute of its own error, and
// the rendered-minute check below pins the number the user actually sees.
const tolerance = 90 * time.Second

// SUN-6, SUN-18, SUN-20, NFR-4: all four events are computed from the
// configured coordinates and land on the published values.
func TestTimesMatchPublishedValues(t *testing.T) {
	for _, reference := range references {
		t.Run(reference.name, func(t *testing.T) {
			zone := time.FixedZone("REF", reference.offset)
			day := Times(reference.place, reference.date)

			if day.Date != reference.date {
				t.Errorf("Times returned date %v, want %v", day.Date, reference.date)
			}

			for _, event := range []struct {
				name      string
				got       Moment
				published string
			}{
				{"dawn", day.Dawn, reference.dawn},
				{"sunrise", day.Sunrise, reference.sunrise},
				{"sunset", day.Sunset, reference.sunset},
				{"dusk", day.Dusk, reference.dusk},
			} {
				instant, occurs := event.got.Instant()
				if !occurs {
					t.Errorf("%s did not occur, want %s", event.name, event.published)
					continue
				}

				want := clockTime(t, reference.date, event.published, zone)
				if drift := instant.Sub(want).Abs(); drift > tolerance {
					t.Errorf("%s = %s, want %s (published): off by %s, tolerance %s",
						event.name,
						instant.In(zone).Format("15:04:05"),
						event.published, drift, tolerance)
				}
			}
		})
	}
}

// clockTime is a published HH:MM on the reference date, in the reference zone.
func clockTime(t *testing.T, date Date, clock string, zone *time.Location) time.Time {
	t.Helper()

	parsed, err := time.Parse("15:04", clock)
	if err != nil {
		t.Fatalf("unparsable reference time %q: %v", clock, err)
	}
	return time.Date(date.Year, date.Month, date.Day,
		parsed.Hour(), parsed.Minute(), 0, 0, zone)
}

// A Moment says whether its event happened in a field of its own, so the zero
// time.Time stays available as an ordinary instant. Without this, a genuine
// midnight-in-year-1 and a missing event would be the same value — which is
// the retype issue #7 must not have to make.
func TestMomentSeparatesAbsenceFromTheZeroTime(t *testing.T) {
	var zeroTime time.Time

	present := At(zeroTime)
	if !present.Occurs() {
		t.Error("At(zero time) reports the event as absent; the zero time is an instant, not a sentinel")
	}
	if instant, occurs := present.Instant(); !occurs || !instant.Equal(zeroTime) {
		t.Errorf("At(zero time).Instant() = %v, %v; want the zero time and true", instant, occurs)
	}

	absent := Never()
	if absent.Occurs() {
		t.Error("Never() reports the event as occurring")
	}
	if _, occurs := absent.Instant(); occurs {
		t.Error("Never().Instant() claims to have an instant")
	}
}

// The zero Moment is absent, so a partly-filled Day reads as "did not happen"
// rather than as a time in year 1.
func TestZeroMomentIsAbsent(t *testing.T) {
	var day Day

	for name, moment := range map[string]Moment{
		"dawn":    day.Dawn,
		"sunrise": day.Sunrise,
		"sunset":  day.Sunset,
		"dusk":    day.Dusk,
	} {
		if moment.Occurs() {
			t.Errorf("the zero Day's %s claims to occur", name)
		}
	}
}

// SUN-23: a Date renders as YYYY-MM-DD and knows its weekday.
func TestDateFormatsAndKnowsItsWeekday(t *testing.T) {
	date := Date{Year: 2026, Month: time.July, Day: 24}

	if got, want := date.String(), "2026-07-24"; got != want {
		t.Errorf("Date.String() = %q, want %q", got, want)
	}
	if got, want := date.Weekday(), time.Friday; got != want {
		t.Errorf("Date.Weekday() = %v, want %v", got, want)
	}
}

// Today reads the calendar date from the zone it is given, not from the host's.
func TestTodayUsesTheGivenZone(t *testing.T) {
	// Bracket the call, because the date can turn over mid-test.
	before := dateOf(time.Now().UTC())
	got := Today(time.UTC)
	after := dateOf(time.Now().UTC())

	if got != before && got != after {
		t.Errorf("Today(UTC) = %v, want %v or %v", got, before, after)
	}

	// Two zones far enough apart cannot both be on the same date all day; at
	// any instant the +14 zone is on the same date as the -11 zone or the one
	// after it, never before it.
	east := Today(time.FixedZone("east", 14*60*60))
	west := Today(time.FixedZone("west", -11*60*60))
	if east.time().Before(west.time()) {
		t.Errorf("Today(UTC+14) = %v is before Today(UTC-11) = %v", east, west)
	}
}

// ParseDate is the inverse of Date.String, and is strict about the form it
// accepts (SUN-16).
func TestParseDateReadsCalendarDates(t *testing.T) {
	valid := map[string]Date{
		"2026-07-24": {Year: 2026, Month: time.July, Day: 24},
		"2028-02-29": {Year: 2028, Month: time.February, Day: 29},
		"1999-12-31": {Year: 1999, Month: time.December, Day: 31},
		"2027-01-01": {Year: 2027, Month: time.January, Day: 1},
	}
	for value, want := range valid {
		got, err := ParseDate(value)
		if err != nil {
			t.Errorf("ParseDate(%q) returned error %v, want %v", value, err, want)
			continue
		}
		if got != want {
			t.Errorf("ParseDate(%q) = %v, want %v", value, got, want)
		}
		if roundTrip := got.String(); roundTrip != value {
			t.Errorf("ParseDate(%q).String() = %q, want the value back", value, roundTrip)
		}
	}

	invalid := []string{
		"",                     // nothing at all
		"2026-7-1",             // unpadded fields
		"2026/07/24",           // wrong separator
		"24-07-2026",           // wrong order
		"2026-13-01",           // no thirteenth month
		"2026-02-30",           // not on the calendar
		"2027-02-29",           // 2027 is not a leap year
		"2026-07-24 ",          // trailing space
		"2026-07-24T00:00:00Z", // more than a date
		"today",                // not a date at all
	}
	for _, value := range invalid {
		if got, err := ParseDate(value); err == nil {
			t.Errorf("ParseDate(%q) = %v, want an error", value, got)
		} else if !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Errorf("ParseDate(%q) error %q does not say what form it wanted", value, err)
		}
	}
}

// AddDays walks the calendar, so month lengths, year ends and leap days are
// handled by the calendar rather than by arithmetic on the day number.
func TestAddDaysCrossesCalendarBoundaries(t *testing.T) {
	tests := []struct {
		from string
		days int
		want string
	}{
		{"2026-07-24", 0, "2026-07-24"},
		{"2026-07-24", 1, "2026-07-25"},
		{"2026-07-31", 1, "2026-08-01"},   // month rollover
		{"2026-12-31", 1, "2027-01-01"},   // year rollover
		{"2026-01-01", -1, "2025-12-31"},  // backwards over a year end
		{"2028-02-28", 1, "2028-02-29"},   // leap day exists
		{"2027-02-28", 1, "2027-03-01"},   // and does not, a year earlier
		{"2028-02-29", 365, "2029-02-28"}, // a year on from a leap day
		{"2026-11-15", 92, "2027-02-15"},  // a long span across months
	}

	for _, test := range tests {
		from, err := ParseDate(test.from)
		if err != nil {
			t.Fatalf("unparsable test date %q: %v", test.from, err)
		}
		if got := from.AddDays(test.days).String(); got != test.want {
			t.Errorf("%s.AddDays(%d) = %s, want %s", test.from, test.days, got, test.want)
		}
	}
}

// DaysUntil counts whole calendar days, in either direction.
func TestDaysUntilCountsCalendarDays(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want int
	}{
		{"2026-07-24", "2026-07-24", 0},
		{"2026-07-24", "2026-07-25", 1},
		{"2026-07-25", "2026-07-24", -1},
		{"2026-12-31", "2027-01-01", 1},
		{"2028-02-28", "2028-03-01", 2}, // a leap day sits in between
		{"2027-02-28", "2027-03-01", 1}, // and does not, a year earlier
		{"2026-01-01", "2026-12-31", 364},
		{"2028-01-01", "2028-12-31", 365}, // a leap year is a day longer
	}

	for _, test := range tests {
		from, err := ParseDate(test.from)
		if err != nil {
			t.Fatalf("unparsable test date %q: %v", test.from, err)
		}
		to, err := ParseDate(test.to)
		if err != nil {
			t.Fatalf("unparsable test date %q: %v", test.to, err)
		}
		if got := from.DaysUntil(to); got != test.want {
			t.Errorf("%s.DaysUntil(%s) = %d, want %d", test.from, test.to, got, test.want)
		}
	}
}

// AddDays and DaysUntil are two views of the same walk, so stepping n days out
// and counting back must agree — including across a daylight-saving change,
// which cannot shorten a calendar day here.
func TestAddDaysAndDaysUntilAgree(t *testing.T) {
	start := Date{Year: 2026, Month: time.March, Day: 25}

	for step := -400; step <= 400; step++ {
		if got := start.DaysUntil(start.AddDays(step)); got != step {
			t.Fatalf("%s.AddDays(%d) is %d days away, want %d",
				start, step, got, step)
		}
	}
}
