package cli

import (
	"fmt"

	"github.com/philipf/suntimes/internal/sun"
)

// defaultDays is the rolling window shown when no date flags are given: today
// and the following six days (SUN-11).
const defaultDays = 7

// maxRangeDays caps how many rows one invocation may ask for — about ten
// years. The PRD sets no limit, but a range is far more often a typo than a
// genuine request (`--days 100000`, `--to 9999-12-31`), and a clear refusal
// beats scrolling a decade of output off the top of the terminal.
const maxRangeDays = 3660

// dateExample is shown when a date cannot be read, so the message demonstrates
// the form it is asking for rather than only naming it.
const dateExample = "2026-07-25"

// currentDate reports today's date in a display zone. It is a variable purely
// so tests can pin the clock and assert on exact dates; nothing outside tests
// ever reassigns it.
var currentDate = sun.Today

// usageError is a mistake in how the command was invoked — flags that cannot
// be combined, or one half of a pair — as opposed to a failure while running
// it. The root command prints usage alongside these and only these (SUN-15),
// which matches how cobra itself reports an unknown flag.
type usageError struct {
	message string
}

func (e *usageError) Error() string {
	return e.message
}

// usagef builds a usageError from a format string.
func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// resolveDates turns the date flags into the days to display, in ascending
// order (SUN-24). Exactly one of four forms is honoured: a single --date
// (SUN-14), an explicit --from/--to range (SUN-13), a rolling --days window
// (SUN-12), or, with no date flags at all, the default week (SUN-11).
//
// The division of errors is deliberate: which flags were supplied together is
// a usage error (SUN-15), while a value inside a flag that cannot mean what it
// says — an unreadable date (SUN-16), an inverted (SUN-17) or over-long range,
// a non-positive day count — is a descriptive error on its own.
func resolveDates(opts *Options, today sun.Date) ([]sun.Date, error) {
	hasRange := opts.From != "" || opts.To != ""

	// The PRD only names the --days/--from/--to clash (SUN-15). The others are
	// the same mistake — asking for two ranges at once, or half of one — and a
	// guess about which the user meant would be worse than a refusal.
	switch {
	case opts.Date != "" && (opts.DaysSet || hasRange):
		return nil, usagef("--date selects a single day, so it cannot be combined with --days, --from or --to")
	case opts.DaysSet && hasRange:
		return nil, usagef("--days counts forward from today, so it cannot be combined with --from or --to")
	case opts.From != "" && opts.To == "":
		return nil, usagef("--from needs a matching --to to close the range")
	case opts.To != "" && opts.From == "":
		return nil, usagef("--to needs a matching --from to open the range")
	}

	switch {
	case opts.Date != "":
		date, err := parseDate("--date", opts.Date)
		if err != nil {
			return nil, err
		}
		return []sun.Date{date}, nil

	case hasRange:
		return explicitRange(opts.From, opts.To)

	default:
		return rollingWindow(opts, today)
	}
}

// explicitRange is every day from one date to another, inclusive (SUN-13).
func explicitRange(fromValue, toValue string) ([]sun.Date, error) {
	from, err := parseDate("--from", fromValue)
	if err != nil {
		return nil, err
	}
	to, err := parseDate("--to", toValue)
	if err != nil {
		return nil, err
	}

	// SUN-17: a backwards range is rejected rather than quietly swapped, since
	// the swap would hide the typo that caused it.
	if from.DaysUntil(to) < 0 {
		return nil, fmt.Errorf("--to %s is earlier than --from %s, so the range runs backwards", to, from)
	}

	count := from.DaysUntil(to) + 1 // inclusive of both ends
	if count > maxRangeDays {
		return nil, fmt.Errorf("the range %s to %s covers %d days, more than the %d-day maximum",
			from, to, count, maxRangeDays)
	}
	return span(from, count), nil
}

// rollingWindow is the window that starts today: --days N days of it (SUN-12),
// or a week when --days was not given (SUN-11).
func rollingWindow(opts *Options, today sun.Date) ([]sun.Date, error) {
	count := defaultDays
	if opts.DaysSet {
		count = opts.Days
	}

	// Zero days would print nothing and a negative count cannot mean anything;
	// both are far likelier to be a mistake than a request.
	if count < 1 {
		return nil, fmt.Errorf("--days must be at least 1, not %d", count)
	}
	if count > maxRangeDays {
		return nil, fmt.Errorf("--days %d is more than the %d-day maximum", count, maxRangeDays)
	}
	return span(today, count), nil
}

// span is count consecutive dates beginning at first, ascending (SUN-24).
func span(first sun.Date, count int) []sun.Date {
	dates := make([]sun.Date, 0, count)
	for offset := range count {
		dates = append(dates, first.AddDays(offset))
	}
	return dates
}

// parseDate reads one date-valued flag, naming the flag in any error so the
// user knows which of --date, --from and --to to correct (SUN-16).
func parseDate(name, value string) (sun.Date, error) {
	date, err := sun.ParseDate(value)
	if err != nil {
		return sun.Date{}, fmt.Errorf("%s: %w (for example %s)", name, err, dateExample)
	}
	return date, nil
}
