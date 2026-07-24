package config

// SampleFile is the commented configuration written on first run (SUN-3).
//
// It is a hand-authored template rather than Viper output because Viper does
// not preserve comments when writing (PRD §11). The values are a working
// example — Auckland, New Zealand — so the file loads as-is; the user is told
// to edit it to their own location.
const SampleFile = `# suntimes configuration
#
# Times are computed offline from the coordinates below. Edit them to your own
# location: decimal degrees, not degrees/minutes/seconds. Both values are
# required.

# Latitude in decimal degrees, from -90 (south pole) to 90 (north pole).
# Positive is north of the equator, negative is south.
latitude = -36.8485

# Longitude in decimal degrees, from -180 to 180.
# Positive is east of Greenwich, negative is west.
longitude = 174.7633

# Timezone as an IANA name, for example "Pacific/Auckland", "Europe/London" or
# "America/New_York". Optional: leave it empty ("") or delete the line to use
# this machine's local timezone.
timezone = "Pacific/Auckland"
`
