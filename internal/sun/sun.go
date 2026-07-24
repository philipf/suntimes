// Package sun computes the daily light events — dawn, sunrise, sunset and dusk
// — for a place and a calendar date.
//
// Everything is derived locally from latitude and longitude, with no network
// request at any point (SUN-19, NFR-2). The astronomy comes from
// github.com/nathan-osman/go-sunrise, a dependency-free package that imports
// only "math" and "time".
//
// The definitions are those of PRD §5: sunrise and sunset are the moments the
// sun's upper edge crosses the horizon, and dawn and dusk are the civil
// twilight threshold, the sun 6° below the horizon (SUN-18).
//
// Every event is a Moment, which may be absent. At extreme latitudes an event
// need not happen at all on a given day — the sun may never rise, or civil
// twilight may never begin (PRD §5, "no-event day").
package sun

import (
	"fmt"
	"time"

	sunrise "github.com/nathan-osman/go-sunrise"
)

// civilTwilightElevation is the sun's elevation, in degrees, that bounds civil
// twilight: 6° below the horizon (SUN-18).
const civilTwilightElevation = -6.0

// Place is a point on the globe, in decimal degrees — the location the user
// configured (SUN-6).
type Place struct {
	// Latitude is degrees north of the equator, -90..90.
	Latitude float64
	// Longitude is degrees east of Greenwich, -180..180.
	Longitude float64
}

// Date is a calendar date with no time and no timezone: the day a row is about.
// It is deliberately not a time.Time, because a time.Time carries a zone, and
// "2026-07-24" is the same square on the calendar whichever zone reads it.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// dateLayout is the written form of a Date: YYYY-MM-DD (SUN-23).
const dateLayout = "2006-01-02"

// Today is the current calendar date as seen from zone. The zone matters: it
// can already be tomorrow in Auckland while it is still today in London.
func Today(zone *time.Location) Date {
	return dateOf(time.Now().In(zone))
}

// ParseDate reads a date written as YYYY-MM-DD; it is the inverse of
// Date.String. The layout is strict, so unpadded fields ("2026-7-1"), other
// separators and impossible dates such as 30 February are all rejected rather
// than guessed at (SUN-16).
func ParseDate(value string) (Date, error) {
	instant, err := time.Parse(dateLayout, value)
	if err != nil {
		// The stdlib message ("parsing time ... day out of range") describes
		// its own machinery, not the user's flag, so it is replaced rather
		// than wrapped.
		return Date{}, fmt.Errorf("%q is not a calendar date in YYYY-MM-DD form", value)
	}
	return dateOf(instant), nil
}

// dateOf takes the calendar date an instant falls on, in its own zone.
func dateOf(instant time.Time) Date {
	year, month, day := instant.Date()
	return Date{Year: year, Month: month, Day: day}
}

// Weekday is the day of the week the date falls on, which depends only on the
// date itself.
func (d Date) Weekday() time.Weekday {
	return d.time().Weekday()
}

// String renders the date as YYYY-MM-DD (SUN-23).
func (d Date) String() string {
	return d.time().Format(dateLayout)
}

// AddDays is the date n days after d; a negative n moves backwards. Month and
// year boundaries are handled by the calendar itself, so 31 December plus one
// day is 1 January of the next year and 28 February 2028 plus one day is the
// 29th.
func (d Date) AddDays(n int) Date {
	return dateOf(d.time().AddDate(0, 0, n))
}

// DaysUntil is the whole number of days from d to other: zero for the same
// date, positive when other is later and negative when it is earlier. Both
// dates are midnight UTC, so no daylight-saving change can make a day count as
// 23 or 25 hours here.
//
// Spans beyond roughly 292 years saturate rather than wrap, because that is
// what time.Time.Sub does; the sign is still right, which is all the callers
// that reject an over-long range need.
func (d Date) DaysUntil(other Date) int {
	return int(other.time().Sub(d.time()) / (24 * time.Hour))
}

// time is midnight on the date in UTC, used only to reach time's calendar
// helpers. It is never treated as an instant the user cares about.
func (d Date) time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// Moment is the instant at which a sun event occurs, or the explicit fact that
// it does not occur at all.
//
// Absence is carried in its own field rather than by a sentinel value: the zero
// time.Time is a real instant (1 January, year 1), so using it to mean "no such
// event" would make a genuine time indistinguishable from a missing one. Every
// consumer must therefore ask, and cannot forget to.
type Moment struct {
	instant time.Time
	occurs  bool
}

// At is the Moment for an event that occurs at instant.
func At(instant time.Time) Moment {
	return Moment{instant: instant, occurs: true}
}

// Never is the Moment for an event that does not occur — a no-event day. It is
// also the zero Moment, so a Day left unfilled reads as "nothing happened"
// rather than as midnight in year 1.
func Never() Moment {
	return Moment{}
}

// Occurs reports whether the event happens at all.
func (m Moment) Occurs() bool {
	return m.occurs
}

// Instant returns when the event occurs, and whether it occurs. The instant is
// only meaningful when the second result is true.
func (m Moment) Instant() (time.Time, bool) {
	return m.instant, m.occurs
}

// Day is the set of light events for one calendar date at one place. The four
// events are held in the order they are presented (SUN-21).
type Day struct {
	Date    Date
	Dawn    Moment
	Sunrise Moment
	Sunset  Moment
	Dusk    Moment
}

// Times computes dawn, sunrise, sunset and dusk for date at place (SUN-20).
//
// The returned instants are absolute; rendering them in a display timezone is
// the caller's job, which is what keeps daylight saving out of the astronomy
// (SUN-10).
//
// # No-event days
//
// The four events come from two separate calculations, each with its own
// threshold: sunrise and sunset are the sun's upper edge at the horizon, which
// go-sunrise takes as an elevation of −0.83°, while dawn and dusk are civil
// twilight at −6° (SUN-18). Each calculation asks whether the sun reaches its
// own elevation on that day, and the two answer independently — which is what
// lets one row hold real times beside placeholders (SUN-27).
//
// At 78.22°N, 15.63°E (Longyearbyen) in 2026 every polar condition occurs, and
// each leaves a different pattern:
//
//   - 13 Nov – 29 Jan, polar night with no twilight: the sun stays below −6° all
//     day, reaching neither threshold, so all four events are absent.
//   - 30 Jan – 15 Feb and 27 Oct – 12 Nov, polar night with civil twilight: the
//     sun climbs past −6° but never past −0.83°, so dawn and dusk are real times
//     while sunrise and sunset are absent.
//   - 5 – 18 Apr and 25 Aug – 7 Sep, continuous civil twilight: the sun sets but
//     never falls below −6°, so sunrise and sunset are real times while dawn and
//     dusk are absent — the reverse mixture.
//   - 19 Apr – 24 Aug, midnight sun: the sun stays above the horizon, crossing
//     neither threshold, so all four events are absent.
//
// Within one threshold the two events stand or fall together, because the model
// places both the same distance either side of that day's solar noon. That is a
// property of the astronomy rather than of this translation: a day on which the
// sun genuinely rises and then never sets is a boundary the model resolves to
// the nearer whole day.
func Times(place Place, date Date) Day {
	sunriseAt, sunsetAt := sunrise.SunriseSunset(
		place.Latitude, place.Longitude, date.Year, date.Month, date.Day)
	dawnAt, duskAt := sunrise.TimeOfElevation(
		place.Latitude, place.Longitude, civilTwilightElevation,
		date.Year, date.Month, date.Day)

	// Every event is converted on its own. Nothing here pairs them up, and no
	// event's absence is allowed to decide another's.
	return Day{
		Date:    date,
		Dawn:    moment(dawnAt, date),
		Sunrise: moment(sunriseAt, date),
		Sunset:  moment(sunsetAt, date),
		Dusk:    moment(duskAt, date),
	}
}

// plausibleWindow is how far either side of midnight UTC on its own date a
// genuine event may fall.
//
// go-sunrise places every event within half a day of that date's mean solar
// noon, and mean solar noon is noon UTC on the date shifted by the longitude,
// at most half a day either way. A real event is therefore never more than 36
// hours from midnight UTC on the date it was computed for. 48 leaves margin
// without coming anywhere near admitting something that is not an event.
const plausibleWindow = 48 * time.Hour

// moment converts one go-sunrise result into a Moment, deciding for that event
// alone whether it happens (SUN-27).
//
// The library has two ways of saying "not today", and they are not the same
// way. TimeOfElevation tests its hour angle for NaN and returns the zero
// time.Time. SunriseSunset instead tests for the ±math.MaxFloat64 that
// HourAngle returns when the sun never rises or never sets — but HourAngle can
// also return NaN, which an equality test cannot catch, and that NaN runs on
// through the Julian-day conversion, through an int64 cast, and out as an
// ordinary-looking time.Time in the year 292277026596. A NaN latitude reaches
// it: `latitude = nan` is valid TOML and NaN fails no range comparison. Config
// rejects that value now, so it cannot arrive from a file; this remains the
// guarantee the domain makes on its own, which is the one SUN-27 rests on — no
// sentinel, no zero, and nothing derived from a NaN leaves here wearing the
// shape of a time.
//
// One question catches both: could this instant belong to this date at all?
// Year 1 cannot, year 292277026596 cannot, and every real event can with a day
// and a half to spare.
func moment(instant time.Time, date Date) Moment {
	midnight := date.time()
	if instant.Before(midnight.Add(-plausibleWindow)) ||
		instant.After(midnight.Add(plausibleWindow)) {
		return Never()
	}
	return At(instant)
}
