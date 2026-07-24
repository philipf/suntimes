package sun

import (
	"math"
	"strings"
	"testing"
	"time"

	sunrise "github.com/nathan-osman/go-sunrise"
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
// midnight-in-year-1 and a missing event would be the same value, and no-event
// detection would have had to reintroduce the sentinel it exists to avoid.
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

// horizonElevation is the elevation go-sunrise treats as sunrise and sunset:
// the sun's upper edge on the horizon, 0.83° below its centre once refraction
// is allowed for. The library does not name it, but its HourAngle uses
// sin(-0.83°) = -0.01449 directly, and the tests below need the number to say
// which polar condition a date is in.
const horizonElevation = -0.83

// longyearbyen is Svalbard, at 78.22°N — far enough north that a single year
// passes through every polar condition there is.
var longyearbyen = Place{Latitude: 78.22, Longitude: 15.63}

// occurrence is which of the four events happen on a day. It is the shape a
// polar condition leaves in a table row.
type occurrence struct {
	dawn    bool
	sunrise bool
	sunset  bool
	dusk    bool
}

// occurrenceOf reads the pattern off a computed day.
func occurrenceOf(day Day) occurrence {
	return occurrence{
		dawn:    day.Dawn.Occurs(),
		sunrise: day.Sunrise.Occurs(),
		sunset:  day.Sunset.Occurs(),
		dusk:    day.Dusk.Occurs(),
	}
}

// mixed reports whether a row holds both real times and placeholders. This is
// the property that separates deciding each event from deciding all four at
// once: a guard that only knew "the calculation gave us nothing" could never
// produce a mixed row.
func (o occurrence) mixed() bool {
	all := o.dawn && o.sunrise && o.sunset && o.dusk
	none := !o.dawn && !o.sunrise && !o.sunset && !o.dusk
	return !all && !none
}

func (o occurrence) String() string {
	name := func(occurs bool, label string) string {
		if occurs {
			return label
		}
		return strings.Repeat("-", len(label))
	}
	return strings.Join([]string{
		name(o.dawn, "dawn"), name(o.sunrise, "sunrise"),
		name(o.sunset, "sunset"), name(o.dusk, "dusk"),
	}, " ")
}

// The named polar conditions, as PRD §5 describes them and as the sun's
// elevation defines them.
const (
	midnightSun      = "midnight sun: the sun never sets"
	polarNight       = "polar night: the sun never rises, and twilight never begins"
	twilightOnly     = "polar night with civil twilight: twilight begins and ends, but the sun never rises"
	continuousCivil  = "continuous civil twilight: the sun rises and sets, but twilight never begins or ends"
	ordinaryDaylight = "an ordinary day: all four events happen"
)

// conditionOn names the polar condition a place is in on a date, from where the
// sun's elevation extremes sit relative to the two thresholds. It is how the
// test knows what each dated case below actually is, rather than taking the
// label's word for it.
func conditionOn(t *testing.T, place Place, date Date) (name string, lowest, highest float64) {
	t.Helper()

	lowest, highest = elevationRange(place, date)
	switch {
	case lowest > horizonElevation:
		name = midnightSun
	case highest < civilTwilightElevation:
		name = polarNight
	case highest < horizonElevation:
		name = twilightOnly
	case lowest > civilTwilightElevation:
		name = continuousCivil
	default:
		name = ordinaryDaylight
	}
	return name, lowest, highest
}

// elevationRange is the lowest and highest the sun gets during the solar day a
// date's calculation is about.
//
// The window is centred on that date's mean solar noon rather than on midnight
// UTC, because that is the day the astronomy is answering about: at longitude
// 15.63°E it runs from roughly 23:00 UTC the evening before. Sampling by the
// minute is finer than the events are reported and far finer than the extremes
// need, since the elevation curve has one maximum and one minimum a day.
func elevationRange(place Place, date Date) (lowest, highest float64) {
	noon := sunrise.JulianDayToTime(
		sunrise.MeanSolarNoon(place.Longitude, date.Year, date.Month, date.Day))

	lowest, highest = math.Inf(1), math.Inf(-1)
	for minute := -12 * 60; minute <= 12*60; minute++ {
		degrees := sunrise.Elevation(place.Latitude, place.Longitude,
			noon.Add(time.Duration(minute)*time.Minute))
		lowest = math.Min(lowest, degrees)
		highest = math.Max(highest, degrees)
	}
	return lowest, highest
}

// SUN-27: at a polar latitude each event is decided on its own, against its own
// threshold, so the four conditions below produce four different patterns —
// including two opposite mixtures of real times and placeholders.
//
// The condition on each date is not asserted from memory: the test measures the
// sun's elevation across that solar day and names the condition from where the
// extremes fall, then checks the calculation agrees. The commented ranges are
// what that measurement reports.
func TestPolarConditionsDecideEachEventIndependently(t *testing.T) {
	tests := []struct {
		date      string
		condition string
		want      occurrence
	}{
		// Elevation -33.8°..-10.2°: never within 6° of the horizon.
		{"2026-01-10", polarNight, occurrence{}},
		// -27.7°..-4.2°: past -6°, never past -0.83°. A mixed row.
		{"2026-02-05", twilightOnly, occurrence{dawn: true, dusk: true}},
		// -19.4°..4.2°: the sun crosses both thresholds, twice each.
		{"2026-03-01", ordinaryDaylight,
			occurrence{dawn: true, sunrise: true, sunset: true, dusk: true}},
		// -3.9°..19.7°: below the horizon but never below -6°. The reverse
		// mixture — real sunrise and sunset, no dawn and no dusk.
		{"2026-04-10", continuousCivil, occurrence{sunrise: true, sunset: true}},
		// 11.7°..35.2°: the sun stays up all day.
		{"2026-06-21", midnightSun, occurrence{}},
		// -12.6°..11.0°: an ordinary day again, on the way back down.
		{"2026-09-25", ordinaryDaylight,
			occurrence{dawn: true, sunrise: true, sunset: true, dusk: true}},
		// -27.4°..-3.9°: the autumn half of the twilight-only window.
		{"2026-11-05", twilightOnly, occurrence{dawn: true, dusk: true}},
		// -35.0°..-11.5°: deep polar night.
		{"2026-12-15", polarNight, occurrence{}},
	}

	var sawMixed bool
	for _, test := range tests {
		t.Run(test.date, func(t *testing.T) {
			date, err := ParseDate(test.date)
			if err != nil {
				t.Fatalf("unparsable test date %q: %v", test.date, err)
			}

			condition, lowest, highest := conditionOn(t, longyearbyen, date)
			if condition != test.condition {
				t.Fatalf("on %s the sun keeps between %.2f° and %.2f°, which is %q, not %q",
					test.date, lowest, highest, condition, test.condition)
			}

			got := occurrenceOf(Times(longyearbyen, date))
			if got != test.want {
				t.Errorf("%s (%s, elevation %.2f°..%.2f°) gave [%v], want [%v]",
					test.date, condition, lowest, highest, got, test.want)
			}
			if got.mixed() {
				sawMixed = true
			}
		})
	}

	if !sawMixed {
		t.Error("no case produced a row mixing real times with placeholders; " +
			"absence is being decided for all four events at once, not per event")
	}
}

// SUN-27: the two mixtures are opposite, which is only possible because civil
// twilight and the horizon are different thresholds that fail at different
// times of year. A single test for "some placeholder somewhere" would pass on a
// guard that could only ever blank the whole row.
func TestMixedRowsRunBothWays(t *testing.T) {
	tests := []struct {
		date string
		want occurrence
	}{
		{"2026-02-05", occurrence{dawn: true, dusk: true}},
		{"2026-04-10", occurrence{sunrise: true, sunset: true}},
	}

	for _, test := range tests {
		date, err := ParseDate(test.date)
		if err != nil {
			t.Fatalf("unparsable test date %q: %v", test.date, err)
		}

		got := occurrenceOf(Times(longyearbyen, date))
		if !got.mixed() {
			t.Errorf("%s gave [%v], which is not a mixed row", test.date, got)
		}
		if got != test.want {
			t.Errorf("%s gave [%v], want [%v]", test.date, got, test.want)
		}
	}
}

// boundaryTolerance is how close to a threshold the day's elevation extremes
// may come before the sweep below stops insisting on an answer.
//
// The oracle and the calculation are two different models. go-sunrise fixes the
// sun's declination at the day's mean solar noon and then treats the day as
// symmetric about it, while sampling Elevation lets the declination move
// through the day. Away from the thresholds they agree completely; within about
// a tenth of a degree of one — which at Longyearbyen means the one or two days
// on which a season turns — they can land on opposite sides. Half a degree
// leaves that disagreement out of the sweep without excusing a real one: it is
// under a day and a half of declination change, against seasons weeks long.
const boundaryTolerance = 0.5

// crosses reports whether the sun passes through an elevation on a day whose
// extremes are lowest and highest, and whether it does so clearly enough for
// the two models to have to agree.
func crosses(lowest, highest, elevation float64) (yes, decisive bool) {
	yes = lowest < elevation && elevation < highest
	decisive = math.Abs(lowest-elevation) > boundaryTolerance &&
		math.Abs(highest-elevation) > boundaryTolerance
	return yes, decisive
}

// Across a whole polar year the calculation agrees with the sun's own
// elevation, day by day: each event happens exactly when the sun crosses that
// event's own threshold. This is the general statement the dated cases above
// are examples of, and it is what would catch a threshold applied to the wrong
// pair of events, or one pair's absence deciding the other's.
func TestOccurrenceFollowsTheSunAcrossAPolarYear(t *testing.T) {
	date := Date{Year: 2026, Month: time.January, Day: 1}

	var mixedDays, absentDays, undecidedDays int
	for range 365 {
		lowest, highest := elevationRange(longyearbyen, date)
		day := Times(longyearbyen, date)

		wantLight, lightDecisive := crosses(lowest, highest, horizonElevation)
		wantTwilight, twilightDecisive := crosses(lowest, highest, civilTwilightElevation)

		for _, event := range []struct {
			name      string
			moment    Moment
			want      bool
			threshold float64
			decisive  bool
		}{
			{"sunrise", day.Sunrise, wantLight, horizonElevation, lightDecisive},
			{"sunset", day.Sunset, wantLight, horizonElevation, lightDecisive},
			{"dawn", day.Dawn, wantTwilight, civilTwilightElevation, twilightDecisive},
			{"dusk", day.Dusk, wantTwilight, civilTwilightElevation, twilightDecisive},
		} {
			if !event.decisive {
				continue
			}
			if got := event.moment.Occurs(); got != event.want {
				t.Errorf("%s: the sun keeps between %.2f° and %.2f°, so %s should occur = %t at %.2f°, but the calculation says %t",
					date, lowest, highest, event.name, event.want, event.threshold, got)
			}
		}

		switch {
		case !lightDecisive || !twilightDecisive:
			undecidedDays++
		case wantLight != wantTwilight:
			mixedDays++
		case !wantLight && !wantTwilight:
			absentDays++
		}
		date = date.AddDays(1)
	}

	// A year that never left the ordinary would pass every assertion above
	// while proving nothing, and a tolerance wide enough to swallow the year
	// would do the same.
	if mixedDays == 0 || absentDays == 0 {
		t.Errorf("the year held %d mixed days and %d fully absent days; want some of each",
			mixedDays, absentDays)
	}
	// Longyearbyen's year turns eight times — into and out of each polar
	// condition at each of the two thresholds — and near a turn the day's
	// elevation extreme moves only a fraction of a degree, so half a degree of
	// tolerance takes about three days out of the sweep each time. Around
	// two dozen is that; a tenth of the year would mean the tolerance had
	// started excusing seasons rather than their edges.
	if undecidedDays > 36 {
		t.Errorf("%d days of the year were too close to a threshold to check; "+
			"the tolerance is meant to cover the turn of a season, not a season",
			undecidedDays)
	}
}

// Wherever events do occur they stay in their natural order: civil twilight
// begins before the sun rises and ends after it sets. A row that mixed a real
// dawn with a real sunrise from a different threshold's calculation would show
// up here.
func TestOccurringEventsKeepTheirOrder(t *testing.T) {
	places := map[string]Place{
		"Longyearbyen": longyearbyen,
		"Auckland":     {Latitude: -36.8485, Longitude: 174.7633},
		"London":       {Latitude: 51.5074, Longitude: -0.1278},
		"Ushuaia":      {Latitude: -54.8019, Longitude: -68.3030},
	}

	for name, place := range places {
		t.Run(name, func(t *testing.T) {
			date := Date{Year: 2026, Month: time.January, Day: 1}
			for range 365 {
				day := Times(place, date)

				ordered := []struct {
					name   string
					moment Moment
				}{
					{"dawn", day.Dawn}, {"sunrise", day.Sunrise},
					{"sunset", day.Sunset}, {"dusk", day.Dusk},
				}

				previous := time.Time{}
				previousName := ""
				for _, event := range ordered {
					instant, occurs := event.moment.Instant()
					if !occurs {
						continue
					}
					if previousName != "" && instant.Before(previous) {
						t.Errorf("%s: %s at %s is before %s at %s",
							date, event.name, instant, previousName, previous)
					}
					previous, previousName = instant, event.name
				}
				date = date.AddDays(1)
			}
		})
	}
}

// Mid-latitude behaviour is unchanged by no-event detection: every day of the
// year has all four events, and every instant lands on or beside its own date.
// Nothing about the polar guard may reach down and blank an ordinary day.
func TestMidLatitudeDaysAlwaysHaveAllFourEvents(t *testing.T) {
	places := map[string]Place{
		"Auckland":  {Latitude: -36.8485, Longitude: 174.7633},
		"London":    {Latitude: 51.5074, Longitude: -0.1278},
		"New York":  {Latitude: 40.7128, Longitude: -74.0060},
		"Nairobi":   {Latitude: -1.2921, Longitude: 36.8219},
		"Cape Town": {Latitude: -33.9249, Longitude: 18.4241},
	}

	for name, place := range places {
		t.Run(name, func(t *testing.T) {
			date := Date{Year: 2026, Month: time.January, Day: 1}
			for range 365 {
				day := Times(place, date)

				for _, event := range []struct {
					name   string
					moment Moment
				}{
					{"dawn", day.Dawn}, {"sunrise", day.Sunrise},
					{"sunset", day.Sunset}, {"dusk", day.Dusk},
				} {
					instant, occurs := event.moment.Instant()
					if !occurs {
						t.Fatalf("%s: %s does not occur, but %s is not a polar latitude",
							date, event.name, name)
					}
					// A solar day is centred on local solar noon, which the
					// longitude can put anywhere in the UTC day, so an event
					// can be up to 36 hours from midnight UTC on its own date
					// and no further. A high-summer dusk just after midnight
					// is the ordinary case of this.
					if drift := instant.Sub(date.time()).Abs(); drift > 36*time.Hour {
						t.Errorf("%s: %s is at %s, %s from the date it belongs to",
							date, event.name, instant, drift)
					}
				}
				date = date.AddDays(1)
			}
		})
	}
}

// SUN-27: nothing derived from a NaN may leave the calculation wearing the shape
// of a time.
//
// A NaN coordinate is not hypothetical — `latitude = nan` is valid TOML, and
// NaN fails no range comparison, so it took a check of its own in config to stop
// one arriving. This pins the second line of defence: go-sunrise's SunriseSunset
// tests its hour angle for the ±MaxFloat64 that means "never rises" and not for
// NaN, so a NaN runs on through its Julian-day conversion and out as a time in
// the year 292277026596, which would print as an ordinary-looking clock time.
func TestNoInstantIsDerivedFromNotANumber(t *testing.T) {
	date := Date{Year: 2026, Month: time.June, Day: 21}

	places := map[string]Place{
		"NaN latitude":       {Latitude: math.NaN(), Longitude: 15.63},
		"NaN longitude":      {Latitude: 78.22, Longitude: math.NaN()},
		"both NaN":           {Latitude: math.NaN(), Longitude: math.NaN()},
		"infinite latitude":  {Latitude: math.Inf(1), Longitude: 15.63},
		"infinite longitude": {Latitude: 78.22, Longitude: math.Inf(-1)},
	}

	for name, place := range places {
		t.Run(name, func(t *testing.T) {
			day := Times(place, date)

			for eventName, moment := range map[string]Moment{
				"dawn": day.Dawn, "sunrise": day.Sunrise,
				"sunset": day.Sunset, "dusk": day.Dusk,
			} {
				if instant, occurs := moment.Instant(); occurs {
					t.Errorf("%s came back as %s, want no event at all", eventName, instant)
				}
			}
		})
	}
}

// The zero time is rejected because it cannot belong to the date, not because
// it is the zero value — the Moment type keeps that distinction, and the
// boundary must not quietly reintroduce the sentinel it exists to avoid.
func TestOnlyInstantsThatCouldBelongToTheDateAreAccepted(t *testing.T) {
	date := Date{Year: 2026, Month: time.July, Day: 24}
	midnight := time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC)

	accepted := map[string]time.Time{
		"midnight itself":     midnight,
		"midday":              midnight.Add(12 * time.Hour),
		"the evening before":  midnight.Add(-3 * time.Hour),
		"a day and a half on": midnight.Add(36 * time.Hour),
		"the far edge":        midnight.Add(plausibleWindow),
		"the near edge":       midnight.Add(-plausibleWindow),
	}
	for name, instant := range accepted {
		if !moment(instant, date).Occurs() {
			t.Errorf("%s (%s) was rejected, but it could be an event on %s", name, instant, date)
		}
	}

	rejected := map[string]time.Time{
		"the zero time":        {},
		"a NaN-derived time":   time.Unix(int64(math.NaN()), 0).UTC(),
		"just past the edge":   midnight.Add(plausibleWindow + time.Second),
		"just before the edge": midnight.Add(-plausibleWindow - time.Second),
	}
	for name, instant := range rejected {
		if moment(instant, date).Occurs() {
			t.Errorf("%s (%s) was accepted as an event on %s", name, instant, date)
		}
	}
}
