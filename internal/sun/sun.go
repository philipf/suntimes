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
func Times(place Place, date Date) Day {
	sunriseAt, sunsetAt := sunrise.SunriseSunset(
		place.Latitude, place.Longitude, date.Year, date.Month, date.Day)
	dawnAt, duskAt := sunrise.TimeOfElevation(
		place.Latitude, place.Longitude, civilTwilightElevation,
		date.Year, date.Month, date.Day)

	return Day{
		Date:    date,
		Dawn:    moment(dawnAt),
		Sunrise: moment(sunriseAt),
		Sunset:  moment(sunsetAt),
		Dusk:    moment(duskAt),
	}
}

// moment converts a go-sunrise result into a Moment. The library signals "this
// event does not happen today" by returning the zero time.Time, so that
// sentinel is translated here, at the boundary, and never enters the domain.
//
// This is a guard, not full polar handling: deciding no-event days properly —
// telling midnight sun from polar night, and covering the cases the library
// answers less clearly — is issue #7.
func moment(instant time.Time) Moment {
	if instant.IsZero() {
		return Never()
	}
	return At(instant)
}
