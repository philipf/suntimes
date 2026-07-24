// Package render turns computed sun times into the text the user reads.
//
// The output is the bordered table of PRD §9: a header row naming the six
// columns, box-drawing borders, and — when the caller says the destination can
// take it — a colour theme with today's row picked out (SUN-21, SUN-25,
// SUN-27a, SUN-27b).
//
// Whether to style is an argument, never a guess. The command writes to a
// stream the tests replace with a buffer, so a renderer that sniffed the writer
// it was handed would decide from the wrong thing: it would see a buffer in a
// test and a pipe in production and never learn what the user's terminal
// actually is. The caller inspects the real standard output once, turns the
// answer into a Style, and passes it in.
package render

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"

	"github.com/philipf/suntimes/internal/sun"
)

// Placeholder stands in for an event that does not occur on a given day
// (SUN-27). Absence is decided per event, up in the sun package, so a row may
// hold placeholders beside real times: a polar spring day has a sunrise and a
// sunset but no dawn or dusk, and a polar autumn day the other way round.
//
// It is one character three bytes wide. Column widths are measured in display
// cells by lipgloss, not in bytes, so a placeholder occupies exactly as much of
// a column as any other single character and the grid stays square.
const Placeholder = "—"

// clockLayout is 24-hour HH:MM (SUN-22).
const clockLayout = "15:04"

// weekdayWidth is the length of the three-letter weekday abbreviation (SUN-23).
const weekdayWidth = 3

// headings name the columns, in the order the table presents them (SUN-21).
var headings = []string{"Date", "Day", "Dawn", "Sunrise", "Sunset", "Dusk"}

// Options are the decisions the caller makes about a table, beyond the days
// themselves.
type Options struct {
	// Today is the date to highlight (SUN-25). It is a plain value rather than
	// something the renderer reads off the clock, which keeps rendering a pure
	// function of its arguments and lets the caller resolve "today" once, in
	// the display timezone, for both the range and the highlight.
	//
	// The date need not appear in the table at all — `--date 2028-02-29` and a
	// range that ended last week both leave it out — so it is matched against
	// every row rather than assumed to be the first. The zero Today matches
	// nothing, because month zero is not a month.
	Today sun.Date

	// Style says how much styling the output may carry. The zero Style is
	// plain, so a caller that has not thought about it cannot accidentally
	// write escape codes into a pipe (SUN-26).
	Style Style
}

// Style says how much styling a rendered table may carry.
//
// It holds a terminal colour profile rather than a yes/no, because "style it"
// is not one answer: a terminal that reports 16 colours must be sent 16-colour
// codes, not the 24-bit ones it would print as garbage. lipgloss converts the
// theme's colours down to whatever the profile can express, which is what makes
// the theme degrade rather than break (SUN-27b).
type Style struct {
	// colour records that styling was asked for. Keeping it separate from the
	// profile is what makes the zero Style plain: termenv's profiles start at
	// zero with TrueColor, so a bare Style{} would otherwise mean "full
	// colour" — the one default that must never happen by omission.
	colour bool
	// profile is the terminal's colour capability; meaningful only when colour
	// is set.
	profile termenv.Profile
}

// Plain is the style that emits no ANSI colour or styling escape codes at all —
// what output redirected to a file or a pipe gets, so it stays greppable and
// diffable (SUN-26). It is also the zero Style.
func Plain() Style {
	return Style{}
}

// Themed is the full theme, in full colour, regardless of what any terminal
// reports. It is the honest way to ask for styling when the decision has
// already been made elsewhere — and the way a test exercises the styled path
// without a terminal to run in.
func Themed() Style {
	return Style{colour: true, profile: termenv.TrueColor}
}

// Detect reports the style appropriate for w: the colour profile of the
// terminal on the other end, or Plain when there is no terminal there.
//
// The caller passes the process's real standard output, which is the only
// thing that can answer the question. termenv's detection covers the whole of
// SUN-26, SUN-27a and SUN-27b in one call — a pipe, a terminal that says it
// has no colour, and NO_COLOR all come back as Plain, and a terminal with
// limited colour comes back with the profile it actually supports.
func Detect(w io.Writer) Style {
	profile := termenv.NewOutput(w).Profile
	if profile == termenv.Ascii {
		return Plain()
	}
	return Style{colour: true, profile: profile}
}

// Styles reports whether this style may emit escape codes.
func (s Style) Styles() bool {
	return s.colour
}

// profileOrAscii is the profile to render with. Ascii is termenv's "no codes at
// all" profile: every style becomes the string it was given, so a plain table
// is plain by construction rather than by remembering not to set attributes.
func (s Style) profileOrAscii() termenv.Profile {
	if !s.colour {
		return termenv.Ascii
	}
	return s.profile
}

// The theme, as 256-colour indices. lipgloss down-samples them for terminals
// that report fewer colours and drops them entirely under the Ascii profile, so
// these are an upper bound on what is emitted, never a demand.
const (
	// borderColour is a grey that recedes: the grid should frame the times, not
	// compete with them.
	borderColour = lipgloss.Color("240")
	// headingColour is the amber of low sun — the accent that marks the header
	// as a different kind of row from the data (SUN-27a).
	headingColour = lipgloss.Color("214")
)

// theme is the set of cell styles a table is drawn with, all built from one
// renderer so that every one of them shares a colour profile.
type theme struct {
	border  lipgloss.Style
	heading lipgloss.Style
	cell    lipgloss.Style
	today   lipgloss.Style
}

// newTheme builds the styles for a Style.
//
// The renderer writes nowhere: it exists only to carry the colour profile into
// each style. Handing it the real output would make lipgloss re-detect the
// terminal and quietly overrule the decision the caller already made.
func newTheme(style Style) theme {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(style.profileOrAscii())

	// One space either side of every value, which is what turns a grid of
	// characters into a readable table (PRD §9).
	cell := renderer.NewStyle().Padding(0, 1)

	return theme{
		border:  renderer.NewStyle().Foreground(borderColour),
		heading: cell.Bold(true).Foreground(headingColour),
		cell:    cell,
		// Reverse video rather than a colour of its own: it swaps whatever
		// foreground and background the user has chosen, so today's row stands
		// out on a light terminal and a dark one alike, without the renderer
		// having to guess which it is looking at (SUN-25).
		today: cell.Bold(true).Reverse(true),
	}
}

// Table writes the results table: a header row, then one row per day in the
// order given, which the caller keeps ascending by date (SUN-21, SUN-24). Times
// are shown in zone (SUN-7, SUN-8).
func Table(w io.Writer, days []sun.Day, zone *time.Location, opts Options) error {
	_, err := fmt.Fprintln(w, build(days, zone, opts))
	return err
}

// build lays the table out and returns it as a string, with no trailing
// newline. Splitting it from Table is what lets tests and the benchmark measure
// the layout without a writer in the way.
func build(days []sun.Day, zone *time.Location, opts Options) string {
	theme := newTheme(opts.Style)

	grid := table.New().
		// Square corners and thin rules, matching PRD §9. The default is
		// rounded.
		Border(lipgloss.NormalBorder()).
		BorderStyle(theme.border).
		Headers(headings...).
		StyleFunc(func(row, column int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return theme.heading
			// "The row for today, if it is here at all." A range can start
			// tomorrow or end last year, and then nothing is highlighted —
			// which is the requirement, not a gap in it (SUN-25).
			case row >= 0 && row < len(days) && days[row].Date == opts.Today:
				return theme.today
			default:
				return theme.cell
			}
		})

	for _, day := range days {
		grid.Row(Cells(day, zone)...)
	}
	return grid.Render()
}

// Cells renders one day as the six values of a table row: date, weekday, then
// dawn, sunrise, sunset and dusk, in that order (SUN-21).
//
// The values are unpadded. Column widths belong to the table, which sizes each
// column to the widest thing in it — including the heading, so a `Sunrise`
// column is seven characters wide however narrow its times are.
func Cells(day sun.Day, zone *time.Location) []string {
	return []string{
		day.Date.String(),
		weekday(day.Date.Weekday()),
		clock(day.Dawn, zone),
		clock(day.Sunrise, zone),
		clock(day.Sunset, zone),
		clock(day.Dusk, zone),
	}
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
	return day.String()[:weekdayWidth]
}
