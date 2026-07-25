# suntimes

[![CI](https://github.com/philipf/suntimes/actions/workflows/ci.yml/badge.svg)](https://github.com/philipf/suntimes/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A portable Go CLI that prints dawn, sunrise, sunset and dusk for a configured
location and date range — computed entirely offline.

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

## Purpose

`suntimes` helps you plan outdoor activity (its original motivation: exercise)
around the available natural light. For each day it shows:

- **Dawn** — start of civil twilight (sun 6° below the horizon): enough light
  to be outdoors without artificial lighting.
- **Sunrise / Sunset** — the sun's upper edge crossing the horizon.
- **Dusk** — end of civil twilight.

Everything is calculated locally from your latitude and longitude — no network
access, no API keys, deterministic output. The app ships as a single
statically-linked binary with a small, human-editable TOML config file.

At extreme latitudes, days where an event never occurs (e.g. the sun never
rises) show a `—` placeholder instead of a misleading time. When output is
piped or redirected, colour and styling are dropped automatically so the table
stays plain text.

See [docs/PRD.md](docs/PRD.md) for the full requirements.

## Install

Requires Go 1.25+ to build. There are no runtime dependencies.

### With `go install`

```sh
go install github.com/philipf/suntimes@latest
```

### From source

```sh
git clone https://github.com/philipf/suntimes.git
cd suntimes
make build      # builds ./suntimes for the host platform
make install    # or: install into your GOBIN
```

To cross-compile for other platforms (Linux/macOS/Windows, amd64/arm64):

```sh
make cross                    # all platforms, into dist/
make build-linux-arm64        # or a single target
```

## Usage

On first run, `suntimes` creates a commented sample config at
`~/.config/suntimes/config.toml` and prints its path. Edit it with your
coordinates:

```toml
# ~/.config/suntimes/config.toml
latitude  = -36.8485           # required, decimal degrees (−90..90)
longitude = 174.7633           # required, decimal degrees (−180..180)
timezone  = "Pacific/Auckland" # optional IANA name; blank = system local timezone
```

Then run it:

```sh
suntimes                              # the coming week (today + 6 days)
suntimes --days 3                     # today + the next 2 days
suntimes --date 2026-12-21            # a single date
suntimes --from 2026-12-20 --to 2026-12-27   # an inclusive range
suntimes --config ./other.toml        # use an alternative config file
```

Times are shown in 24-hour `HH:MM` format, converted to the configured
timezone (or the system's local timezone if none is set), with daylight-saving
transitions handled. In an interactive terminal, today's row is highlighted.

Run `suntimes --help` for the full flag reference and `suntimes --version` for
the build version.

## Contributing

Contributions are welcome. Bugs and feature requests are tracked in
[GitHub Issues](https://github.com/philipf/suntimes/issues).

To work on the code:

1. Fork and clone the repository, then create a branch for your change.
2. Make your change. The layout is conventional Go: `main.go` at the root and
   the implementation under `internal/` (`cli`, `config`, `sun`, `render`,
   `buildinfo`).
3. Before submitting, run the full check suite:

   ```sh
   make check    # gofmt check, go vet, and tests
   ```

   GitHub Actions runs these same checks on every pull request, so a red build
   means one of them failed.

4. Open a pull request describing what changed and why. New behaviour should
   line up with the requirements in [docs/PRD.md](docs/PRD.md) — if your change
   goes beyond them, open an issue first to discuss it.

`make help` lists all available build targets.

## Licence

MIT — see [LICENSE](LICENSE). Copyright © 2026 Philip Fourie.
