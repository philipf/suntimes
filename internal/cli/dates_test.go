package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/philipf/suntimes/internal/sun"
)

// pinToday freezes the clock resolveDates and run see, so every test asserts
// on exact dates rather than on whatever day it happens to be run.
func pinToday(t *testing.T, value string) sun.Date {
	t.Helper()

	date, err := sun.ParseDate(value)
	if err != nil {
		t.Fatalf("unparsable pinned date %q: %v", value, err)
	}

	previous := currentDate
	currentDate = func(*time.Location) sun.Date { return date }
	t.Cleanup(func() { currentDate = previous })
	return date
}

// dateStrings renders resolved dates for comparison against literals.
func dateStrings(dates []sun.Date) []string {
	rendered := make([]string, 0, len(dates))
	for _, date := range dates {
		rendered = append(rendered, date.String())
	}
	return rendered
}

// SUN-11, SUN-12, SUN-13, SUN-14: each of the four ways of asking for dates
// resolves to exactly the days it names.
func TestResolveDatesForEachRangeForm(t *testing.T) {
	today := sun.Date{Year: 2026, Month: time.July, Day: 25}

	tests := map[string]struct {
		opts Options
		want []string
	}{
		// SUN-11: today plus the following six days, seven rows.
		"no date flags": {
			opts: Options{},
			want: []string{
				"2026-07-25", "2026-07-26", "2026-07-27", "2026-07-28",
				"2026-07-29", "2026-07-30", "2026-07-31",
			},
		},
		// SUN-12: N days counted from today, today included.
		"days": {
			opts: Options{Days: 3, DaysSet: true},
			want: []string{"2026-07-25", "2026-07-26", "2026-07-27"},
		},
		"days of one": {
			opts: Options{Days: 1, DaysSet: true},
			want: []string{"2026-07-25"},
		},
		// SUN-13: both ends of the range are included.
		"from and to": {
			opts: Options{From: "2026-03-01", To: "2026-03-04"},
			want: []string{"2026-03-01", "2026-03-02", "2026-03-03", "2026-03-04"},
		},
		"from and to on the same day": {
			opts: Options{From: "2026-03-01", To: "2026-03-01"},
			want: []string{"2026-03-01"},
		},
		// SUN-14: one row, and no dependence on today.
		"date": {
			opts: Options{Date: "2019-11-08"},
			want: []string{"2019-11-08"},
		},
		// The calendar does the arithmetic, so months and years roll over.
		"range across a year boundary": {
			opts: Options{From: "2026-12-30", To: "2027-01-02"},
			want: []string{"2026-12-30", "2026-12-31", "2027-01-01", "2027-01-02"},
		},
		"days across a month boundary": {
			opts: Options{Days: 4, DaysSet: true},
			want: []string{"2026-07-25", "2026-07-26", "2026-07-27", "2026-07-28"},
		},
		// 2028 is a leap year: the range must contain 29 February.
		"range across a leap day": {
			opts: Options{From: "2028-02-27", To: "2028-03-01"},
			want: []string{"2028-02-27", "2028-02-28", "2028-02-29", "2028-03-01"},
		},
		// 2027 is not: 28 February is followed by 1 March.
		"range across a common-year February": {
			opts: Options{From: "2027-02-27", To: "2027-03-01"},
			want: []string{"2027-02-27", "2027-02-28", "2027-03-01"},
		},
		"single leap day": {
			opts: Options{Date: "2028-02-29"},
			want: []string{"2028-02-29"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dates, err := resolveDates(&test.opts, today)
			if err != nil {
				t.Fatalf("resolveDates(%+v) returned error %v, want nil", test.opts, err)
			}

			got := dateStrings(dates)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Errorf("resolveDates(%+v) = %v, want %v", test.opts, got, test.want)
			}
		})
	}
}

// A rolling window that starts on a leap day still counts calendar days.
func TestResolveDatesCountsDaysAcrossALeapDay(t *testing.T) {
	dates, err := resolveDates(
		&Options{Days: 3, DaysSet: true},
		sun.Date{Year: 2028, Month: time.February, Day: 28})
	if err != nil {
		t.Fatalf("resolveDates returned error %v, want nil", err)
	}

	want := []string{"2028-02-28", "2028-02-29", "2028-03-01"}
	if got := dateStrings(dates); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("resolved %v, want %v", got, want)
	}
}

// SUN-24: dates come out ascending, one day apart, with no repeats or gaps —
// over a span long enough to cross several months and a year end.
func TestResolveDatesAreAscendingAndContiguous(t *testing.T) {
	dates, err := resolveDates(
		&Options{From: "2026-11-15", To: "2027-02-15"},
		sun.Date{Year: 2026, Month: time.July, Day: 25})
	if err != nil {
		t.Fatalf("resolveDates returned error %v, want nil", err)
	}

	if want := 93; len(dates) != want {
		t.Fatalf("resolved %d dates, want %d (inclusive of both ends)", len(dates), want)
	}
	for i := 1; i < len(dates); i++ {
		if step := dates[i-1].DaysUntil(dates[i]); step != 1 {
			t.Fatalf("%s follows %s by %d days, want exactly 1",
				dates[i], dates[i-1], step)
		}
	}
	if first, last := dates[0].String(), dates[len(dates)-1].String(); first != "2026-11-15" || last != "2027-02-15" {
		t.Errorf("range runs %s..%s, want 2026-11-15..2027-02-15", first, last)
	}
}

// SUN-15, SUN-16, SUN-17: every way of asking for something unanswerable is
// refused, with a message that names what to fix. Flag combinations are usage
// errors; unusable values are descriptive errors that stand on their own.
func TestResolveDatesRejectsImpossibleRequests(t *testing.T) {
	today := sun.Date{Year: 2026, Month: time.July, Day: 25}

	tests := map[string]struct {
		opts      Options
		wantUsage bool
		wants     []string
	}{
		// SUN-15, the case the PRD names.
		"days with from and to": {
			opts:      Options{Days: 3, DaysSet: true, From: "2026-07-01", To: "2026-07-07"},
			wantUsage: true,
			wants:     []string{"--days", "--from"},
		},
		"days with from alone": {
			opts:      Options{Days: 3, DaysSet: true, From: "2026-07-01"},
			wantUsage: true,
			wants:     []string{"--days", "--from"},
		},
		"days with to alone": {
			opts:      Options{Days: 3, DaysSet: true, To: "2026-07-07"},
			wantUsage: true,
			wants:     []string{"--days", "--to"},
		},
		// Unstated by the PRD: a single date and a range are two answers to
		// the same question.
		"date with days": {
			opts:      Options{Date: "2026-07-25", Days: 3, DaysSet: true},
			wantUsage: true,
			wants:     []string{"--date", "--days"},
		},
		"date with a range": {
			opts:      Options{Date: "2026-07-25", From: "2026-07-01", To: "2026-07-07"},
			wantUsage: true,
			wants:     []string{"--date", "--from"},
		},
		// Unstated by the PRD: half a range is not a range.
		"from without to": {
			opts:      Options{From: "2026-07-01"},
			wantUsage: true,
			wants:     []string{"--from", "--to"},
		},
		"to without from": {
			opts:      Options{To: "2026-07-07"},
			wantUsage: true,
			wants:     []string{"--to", "--from"},
		},
		// SUN-16, for each date-valued flag.
		"unreadable date": {
			opts:  Options{Date: "25 July 2026"},
			wants: []string{"--date", "25 July 2026", "YYYY-MM-DD"},
		},
		"date in the wrong order": {
			opts:  Options{Date: "25-07-2026"},
			wants: []string{"--date", "YYYY-MM-DD"},
		},
		"unpadded date": {
			opts:  Options{Date: "2026-7-1"},
			wants: []string{"--date", "YYYY-MM-DD"},
		},
		"date that is not on the calendar": {
			opts:  Options{Date: "2026-02-30"},
			wants: []string{"--date", "2026-02-30"},
		},
		"29 February in a common year": {
			opts:  Options{Date: "2027-02-29"},
			wants: []string{"--date", "2027-02-29"},
		},
		"unreadable from": {
			opts:  Options{From: "yesterday", To: "2026-07-07"},
			wants: []string{"--from", "yesterday"},
		},
		"unreadable to": {
			opts:  Options{From: "2026-07-01", To: "tomorrow"},
			wants: []string{"--to", "tomorrow"},
		},
		// SUN-17.
		"to earlier than from": {
			opts:  Options{From: "2026-07-07", To: "2026-07-01"},
			wants: []string{"--to", "2026-07-01", "--from", "2026-07-07", "earlier"},
		},
		"to one day earlier than from": {
			opts:  Options{From: "2026-07-02", To: "2026-07-01"},
			wants: []string{"earlier"},
		},
		// Unstated by the PRD: a window has to hold at least one day.
		"zero days": {
			opts:  Options{Days: 0, DaysSet: true},
			wants: []string{"--days", "at least 1"},
		},
		"negative days": {
			opts:  Options{Days: -3, DaysSet: true},
			wants: []string{"--days", "at least 1", "-3"},
		},
		// Unstated by the PRD: an implausibly long range is refused rather
		// than printed.
		"more days than the maximum": {
			opts:  Options{Days: maxRangeDays + 1, DaysSet: true},
			wants: []string{"--days", "maximum"},
		},
		"range longer than the maximum": {
			opts:  Options{From: "2026-01-01", To: "2999-12-31"},
			wants: []string{"maximum"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dates, err := resolveDates(&test.opts, today)
			if err == nil {
				t.Fatalf("resolveDates(%+v) succeeded with %v, want an error", test.opts, dateStrings(dates))
			}

			var usage *usageError
			if got := errors.As(err, &usage); got != test.wantUsage {
				t.Errorf("error %q is a usage error = %v, want %v", err, got, test.wantUsage)
			}
			for _, want := range test.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

// The maximum is a boundary, not a limit one short of itself.
func TestResolveDatesAllowsExactlyTheMaximum(t *testing.T) {
	today := sun.Date{Year: 2026, Month: time.July, Day: 25}

	dates, err := resolveDates(&Options{Days: maxRangeDays, DaysSet: true}, today)
	if err != nil {
		t.Fatalf("resolveDates with the maximum day count returned error %v, want nil", err)
	}
	if len(dates) != maxRangeDays {
		t.Errorf("resolved %d dates, want %d", len(dates), maxRangeDays)
	}

	last := today.AddDays(maxRangeDays - 1)
	dates, err = resolveDates(&Options{From: today.String(), To: last.String()}, today)
	if err != nil {
		t.Fatalf("resolveDates with a maximum-length range returned error %v, want nil", err)
	}
	if len(dates) != maxRangeDays {
		t.Errorf("resolved %d dates, want %d", len(dates), maxRangeDays)
	}
}

// dateColumn is the first column of each printed row: the date the row is for.
func dateColumn(t *testing.T, out string) []string {
	t.Helper()

	rows := resultRows(t, out)
	dates := make([]string, 0, len(rows))
	for _, row := range rows {
		dates = append(dates, row[0])
	}
	return dates
}

// The same four forms, end to end through the command: the rows the user sees
// are for the days they asked for, in ascending order (SUN-24).
func TestRunPrintsTheRequestedDays(t *testing.T) {
	tests := map[string]struct {
		args []string
		want []string
	}{
		"no date flags": {
			args: nil,
			want: []string{
				"2026-07-25", "2026-07-26", "2026-07-27", "2026-07-28",
				"2026-07-29", "2026-07-30", "2026-07-31",
			},
		},
		"days": {
			args: []string{"--days", "2"},
			want: []string{"2026-07-25", "2026-07-26"},
		},
		"from and to": {
			args: []string{"--from", "2026-12-30", "--to", "2027-01-01"},
			want: []string{"2026-12-30", "2026-12-31", "2027-01-01"},
		},
		"date": {
			args: []string{"--date", "2028-02-29"},
			want: []string{"2028-02-29"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fakeHome(t)
			pinToday(t, "2026-07-25")
			path := writeConfig(t, "latitude = -36.8485\nlongitude = 174.7633\n")

			out, err := execute(t, append([]string{"--config", path}, test.args...)...)
			if err != nil {
				t.Fatalf("run returned error %v, want nil\n---\n%s", err, out)
			}

			got := dateColumn(t, out)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Errorf("printed rows for %v, want %v\n---\n%s", got, test.want, out)
			}
		})
	}
}

// SUN-15: conflicting flags exit non-zero and print usage, because the mistake
// is in how the command was invoked.
func TestRunPrintsUsageForConflictingFlags(t *testing.T) {
	fakeHome(t)
	pinToday(t, "2026-07-25")
	path := writeConfig(t, "latitude = 0\nlongitude = 0\n")

	out, err := execute(t, "--config", path, "--days", "3", "--from", "2026-07-01", "--to", "2026-07-07")
	if err == nil {
		t.Fatalf("conflicting flags succeeded, want a non-zero exit\n---\n%s", out)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("a usage error did not print usage:\n%s", out)
	}
	for _, want := range []string{"--days", "--from"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// SUN-16, SUN-17: a value that cannot be used exits non-zero with a message
// about the value. Usage would be noise — the invocation was well formed.
func TestRunPrintsDescriptiveErrorsForBadDateValues(t *testing.T) {
	tests := map[string]struct {
		args  []string
		wants []string
	}{
		"malformed date": {
			args:  []string{"--date", "2026-13-01"},
			wants: []string{"--date", "2026-13-01", "YYYY-MM-DD"},
		},
		"inverted range": {
			args:  []string{"--from", "2026-07-07", "--to", "2026-07-01"},
			wants: []string{"--to", "earlier", "--from"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fakeHome(t)
			pinToday(t, "2026-07-25")
			path := writeConfig(t, "latitude = 0\nlongitude = 0\n")

			out, err := execute(t, append([]string{"--config", path}, test.args...)...)
			if err == nil {
				t.Fatalf("%s succeeded, want a non-zero exit\n---\n%s", name, out)
			}
			for _, want := range test.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if strings.Contains(out, "Usage:") {
				t.Errorf("a value error printed usage:\n%s", out)
			}
		})
	}
}

// --days is only "not supplied" when it was really not supplied: `--days 0` is
// an explicit request for nothing, and is reported rather than turned into the
// default week.
func TestRunRejectsAnExplicitZeroDays(t *testing.T) {
	fakeHome(t)
	pinToday(t, "2026-07-25")
	path := writeConfig(t, "latitude = 0\nlongitude = 0\n")

	out, err := execute(t, "--config", path, "--days", "0")
	if err == nil {
		t.Fatalf("--days 0 succeeded, want a non-zero exit\n---\n%s", out)
	}
	if !strings.Contains(err.Error(), "--days") {
		t.Errorf("error %q does not name --days", err)
	}
}
