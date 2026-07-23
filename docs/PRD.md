# Product Requirements Document — `suntimes`

**Status:** Draft v1
**Date:** 2026-07-24
**Requirements style:** EARS (Easy Approach to Requirements Syntax) — see <https://alistairmavin.com/ears/>

---

## 1. Overview

`suntimes` is a standalone, portable command-line application (written in Go) that
prints a table of daily light-related times — **dawn, sunrise, sunset, dusk** — for a
configurable location and date range. All values are computed locally from the user's
latitude and longitude; the application requires no network connection and ships as a
single static binary.

The primary purpose is to help the user plan outdoor exercise around the available
natural light.

## 2. Goals

- Show, per day: **Date, Day of week, Dawn, Sunrise, Sunset, Dusk**.
- Default to the coming week; allow arbitrary date ranges and single dates.
- Compute everything **offline**, deterministically, from configured coordinates.
- Ship as a **single portable binary** with a small, human-editable config file.

## 3. Non-goals (v1)

The following are explicitly **out of scope** for the first version:

- Weather data of any kind.
- Moon phase, golden hour, or blue hour.
- Interactive TUI.
- Automatic location detection or place-name geocoding.
- Release automation / packaging beyond `go build` and a `Makefile`.

## 4. Users & primary use case

A single technical user runs `suntimes` from a terminal to see, at a glance, when
there will be enough natural light to exercise outdoors over the coming days.

## 5. Definitions

| Term | Definition |
|------|------------|
| **Dawn** | Start of **civil twilight** — the moment the sun is 6° below the horizon before sunrise. Enough natural light for outdoor activity without artificial lighting. |
| **Sunrise** | The moment the upper edge of the sun appears on the horizon. |
| **Sunset** | The moment the upper edge of the sun disappears below the horizon. |
| **Dusk** | End of **civil twilight** — the moment the sun reaches 6° below the horizon after sunset. |
| **No-event day** | A day (at extreme latitudes) on which a given event does not occur, e.g. the sun never rises, or civil twilight never begins/ends. |
| **TTY** | An interactive terminal attached to standard output. |

### EARS pattern legend

| Pattern | Template |
|---------|----------|
| Ubiquitous | The `<system>` shall `<response>`. |
| Event-driven | **When** `<trigger>`, the `<system>` shall `<response>`. |
| State-driven | **While** `<state>`, the `<system>` shall `<response>`. |
| Optional-feature | **Where** `<feature>`, the `<system>` shall `<response>`. |
| Unwanted-behaviour | **If** `<trigger>`, **then** the `<system>` shall `<response>`. |
| Complex | Combination of the above (e.g. **While … When …**). |

Throughout, `<system>` is **`suntimes`**.

---

## 6. Functional requirements (EARS)

### 6.1 Configuration

- **SUN-1 (Ubiquitous):** The `suntimes` application shall read its configuration from a TOML file located at `~/.config/suntimes/config.toml`.
- **SUN-2 (Event-driven):** When the user supplies a `--config <path>` flag, the `suntimes` application shall read its configuration from the file at `<path>` instead of the default location.
- **SUN-3 (Event-driven):** When no configuration file exists at the resolved path on startup, the `suntimes` application shall create a commented sample configuration file at that path and print the path to the user.
- **SUN-4 (Unwanted-behaviour):** If the configuration file exists but cannot be parsed as valid TOML, then the `suntimes` application shall print a descriptive error identifying the file and exit with a non-zero status.
- **SUN-5 (Unwanted-behaviour):** If a required configuration value (`latitude` or `longitude`) is missing or out of valid range (latitude −90..90, longitude −180..180), then the `suntimes` application shall print a descriptive error and exit with a non-zero status.

### 6.2 Location & timezone

- **SUN-6 (Ubiquitous):** The `suntimes` application shall compute all sun and twilight times from the configured `latitude` and `longitude`.
- **SUN-7 (State-driven):** While a valid IANA `timezone` value is configured, the `suntimes` application shall display all times converted to that timezone.
- **SUN-8 (State-driven):** While no `timezone` value is configured, the `suntimes` application shall display all times in the host machine's local timezone.
- **SUN-9 (Unwanted-behaviour):** If the configured `timezone` value is not a recognised IANA timezone, then the `suntimes` application shall print a descriptive error and exit with a non-zero status.
- **SUN-10 (Ubiquitous):** The `suntimes` application shall account for daylight saving transitions when converting computed times into the display timezone.

### 6.3 Date range selection

- **SUN-11 (Event-driven):** When invoked with no date arguments, the `suntimes` application shall display a rolling range beginning on the current date and covering the following six days (seven days total).
- **SUN-12 (Event-driven):** When invoked with `--days N`, the `suntimes` application shall display a range beginning on the current date and covering `N` days total.
- **SUN-13 (Event-driven):** When invoked with `--from <YYYY-MM-DD> --to <YYYY-MM-DD>`, the `suntimes` application shall display every day in the inclusive range from `--from` to `--to`.
- **SUN-14 (Event-driven):** When invoked with `--date <YYYY-MM-DD>`, the `suntimes` application shall display a single row for that date.
- **SUN-15 (Unwanted-behaviour):** If both `--days` and (`--from`/`--to`) are supplied, then the `suntimes` application shall print a usage error and exit with a non-zero status.
- **SUN-16 (Unwanted-behaviour):** If a supplied date is not a valid `YYYY-MM-DD` value, then the `suntimes` application shall print a descriptive error and exit with a non-zero status.
- **SUN-17 (Unwanted-behaviour):** If `--to` is earlier than `--from`, then the `suntimes` application shall print a descriptive error and exit with a non-zero status.

### 6.4 Calculation

- **SUN-18 (Ubiquitous):** The `suntimes` application shall calculate dawn and dusk using the civil twilight threshold (sun 6° below the horizon).
- **SUN-19 (Ubiquitous):** The `suntimes` application shall calculate all times locally without any network request.
- **SUN-20 (Ubiquitous):** For each requested date, the `suntimes` application shall calculate dawn, sunrise, sunset, and dusk for the configured location.

### 6.5 Output

- **SUN-21 (Ubiquitous):** The `suntimes` application shall present results as a bordered table with the columns, in order: `Date`, `Day`, `Dawn`, `Sunrise`, `Sunset`, `Dusk`.
- **SUN-22 (Ubiquitous):** The `suntimes` application shall render times in 24-hour `HH:MM` format.
- **SUN-23 (Ubiquitous):** The `suntimes` application shall render the date as `YYYY-MM-DD` and the day of week as a three-letter abbreviation (e.g. `Mon`).
- **SUN-24 (Ubiquitous):** The `suntimes` application shall output one row per day in the requested range, in ascending date order.
- **SUN-25 (State-driven):** While standard output is attached to a TTY, the `suntimes` application shall visually highlight the row corresponding to the current date.
- **SUN-26 (State-driven):** While standard output is not attached to a TTY, the `suntimes` application shall omit ANSI colour and styling escape codes from its output.
- **SUN-27 (Complex — Where + When):** Where an event does not occur on a given day (a no-event day), when that day is rendered, the `suntimes` application shall display a placeholder (`—`) in that event's cell instead of a time.
- **SUN-27a (State-driven):** While standard output is attached to a TTY, the `suntimes` application shall apply a consistent visual theme (styled borders, a distinct header style, and colour) to the results table.
- **SUN-27b (State-driven):** While standard output is attached to a TTY that reports limited or no colour support, the `suntimes` application shall degrade the theme gracefully (reduced or no colour) while keeping the table legible.

### 6.6 CLI behaviour

- **SUN-28 (Event-driven):** When invoked with `--help` (or `-h`), the `suntimes` application shall print usage information describing all flags and exit with status zero.
- **SUN-29 (Event-driven):** When invoked with `--version`, the `suntimes` application shall print its version and exit with status zero.
- **SUN-30 (Event-driven):** When the application completes successfully, the `suntimes` application shall exit with status zero.

---

## 7. Non-functional requirements

- **NFR-1 (Portability):** The `suntimes` application shall build to a single, statically-linked binary via `go build` with no runtime dependencies.
- **NFR-2 (Offline):** The `suntimes` application shall function fully without any network access.
- **NFR-3 (Performance):** The `suntimes` application shall render a seven-day table in under 100 ms on typical hardware.
- **NFR-4 (Accuracy):** The `suntimes` application shall compute times accurate to within approximately one minute for mid-latitude locations.
- **NFR-5 (Build tooling):** The project shall provide a `Makefile` with a `build` target and cross-compilation targets (Linux/macOS/Windows, amd64/arm64).
- **NFR-6 (Module):** The project shall use the Go module path `github.com/philipf/suntimes`.

## 8. Configuration file — reference

```toml
# ~/.config/suntimes/config.toml
latitude  = -36.8485          # required, decimal degrees (−90..90)
longitude = 174.7633          # required, decimal degrees (−180..180)
timezone  = "Pacific/Auckland" # optional IANA name; blank = system local timezone
```

## 9. Example output

```
suntimes --days 3

┌────────────┬─────┬───────┬─────────┬────────┬───────┐
│ Date       │ Day │ Dawn  │ Sunrise │ Sunset │ Dusk  │
├────────────┼─────┼───────┼─────────┼────────┼───────┤
│ 2026-07-24 │ Fri │ 07:03 │ 07:33   │ 17:20  │ 17:50 │  ← today (highlighted)
│ 2026-07-25 │ Sat │ 07:02 │ 07:32   │ 17:21  │ 17:51 │
│ 2026-07-26 │ Sun │ 07:01 │ 07:31   │ 17:22  │ 17:52 │
└────────────┴─────┴───────┴─────────┴────────┴───────┘
```

## 10. Acceptance criteria

1. Running `suntimes` with a valid config and no arguments prints a 7-row table starting today.
2. `--days`, `--from/--to`, and `--date` each produce the documented range; invalid or conflicting combinations exit non-zero with a clear message.
3. First run with no config creates a commented sample and reports its path.
4. Times reflect the configured timezone (or system local when blank) and respect DST.
5. Piping output to a file yields plain, colour-free text; interactive runs highlight today.
6. No-event days show `—` rather than a misleading time.
7. `go build` produces a single working binary; `make build` succeeds.

## 11. Implementation notes (non-binding)

- **CLI framework:** `github.com/spf13/cobra` for command, flags, help, and version.
- **Config:** `github.com/spf13/viper` for loading configuration. Viper supports TOML
  natively, so the config file format remains TOML (see §8).
  - Note: Viper does not preserve comments when *writing* config, so the first-run
    sample file (SUN-3) is emitted from a hand-authored, commented template string
    rather than via `viper.WriteConfig`. Viper is still used for reading it back.
- **Styling & table:** `github.com/charmbracelet/lipgloss` for colour, borders, and
  theming, using `github.com/charmbracelet/lipgloss/table` to render the bordered
  results table. Lipgloss auto-detects terminal colour support and degrades on
  non-TTY output, satisfying SUN-25/SUN-26.
- **Sun/twilight math:** `github.com/sixdouglas/suncalc` (SunCalc.js port), with
  `github.com/nathan-osman/go-sunrise` + manual twilight as a fallback.
