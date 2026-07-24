// Package render turns computed sun times into the text the user reads.
//
// This version emits plain, aligned rows with no header, borders or colour.
// The bordered, themed table of SUN-21 and SUN-25..SUN-27b is issue #6; it
// replaces the body of Table and Row without changing their signatures.
package render

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/philipf/suntimes/internal/sun"
)

// Placeholder stands in for an event that does not occur on a given day
// (SUN-27). Nothing routinely produces one yet — issue #7 adds the no-event
// detection — but every cell is rendered through the same path, so the case is
// already handled rather than waiting to be discovered.
const Placeholder = "—"

// Column widths, in display cells. Every value in a column is a fixed width, so
// rows line up without measuring the whole set first.
const (
	dateWidth = 10 // YYYY-MM-DD
	dayWidth  = 3  // Mon
	timeWidth = 5  // HH:MM

	columnGap = "  "
)

// clockLayout is 24-hour HH:MM (SUN-22).
const clockLayout = "15:04"

// Table writes one row per day, in the order given, which the caller keeps
// ascending by date (SUN-24). Times are shown in zone (SUN-7, SUN-8).
func Table(w io.Writer, days []sun.Day, zone *time.Location) error {
	for _, day := range days {
		if _, err := fmt.Fprintln(w, Row(day, zone)); err != nil {
			return err
		}
	}
	return nil
}

// Row renders one day as a single line: date, weekday, then dawn, sunrise,
// sunset and dusk, in that order (SUN-21).
func Row(day sun.Day, zone *time.Location) string {
	cells := []string{
		pad(day.Date.String(), dateWidth),
		pad(weekday(day.Date.Weekday()), dayWidth),
		pad(clock(day.Dawn, zone), timeWidth),
		pad(clock(day.Sunrise, zone), timeWidth),
		pad(clock(day.Sunset, zone), timeWidth),
		pad(clock(day.Dusk, zone), timeWidth),
	}
	// The final column's padding would otherwise be invisible trailing space.
	return strings.TrimRight(strings.Join(cells, columnGap), " ")
}

// clock renders one time cell, or the placeholder when the event never
// happens. The caller cannot skip the absent case: the Moment will not hand
// over an instant without also saying whether it means anything.
func clock(moment sun.Moment, zone *time.Location) string {
	instant, occurs := moment.Instant()
	if !occurs {
		return Placeholder
	}
	// Rounded, not truncated. Published almanacs round to the nearest minute,
	// and rounding halves the worst-case error a minute-resolution display can
	// introduce, which is what keeps the printed value inside NFR-4.
	return instant.In(zone).Round(time.Minute).Format(clockLayout)
}

// weekday abbreviates a day name to three letters, e.g. "Fri" (SUN-23).
func weekday(day time.Weekday) string {
	return day.String()[:dayWidth]
}

// pad right-pads a cell to width, counting runes rather than bytes. The
// placeholder is one character but three bytes, so a byte-width format verb
// would pad it two columns too far and break the alignment.
func pad(cell string, width int) string {
	if gap := width - utf8.RuneCountInString(cell); gap > 0 {
		return cell + strings.Repeat(" ", gap)
	}
	return cell
}
