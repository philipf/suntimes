// Package config resolves, creates and loads the suntimes configuration file.
//
// The file is TOML (see docs/PRD.md §8) and lives at
// ~/.config/suntimes/config.toml unless the user names another path with
// --config. Reading is done with Viper; the first-run sample is written from
// the hand-authored template in sample.go, because Viper drops comments when
// it writes.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config is the validated contents of a configuration file.
type Config struct {
	// Latitude is decimal degrees north of the equator, -90..90 (SUN-5).
	Latitude float64
	// Longitude is decimal degrees east of Greenwich, -180..180 (SUN-5).
	Longitude float64
	// Timezone is an optional IANA name, e.g. "Pacific/Auckland". Empty means
	// the host machine's local timezone (SUN-7, SUN-8). Validating the name and
	// converting times is later work; it is only carried here.
	Timezone string
	// Path is the file the values were read from, for use in messages.
	Path string
}

// Configuration keys, as they appear in the TOML file.
const (
	keyLatitude  = "latitude"
	keyLongitude = "longitude"
	keyTimezone  = "timezone"
)

// Latitude and longitude bounds, inclusive (SUN-5).
const (
	minLatitude  = -90.0
	maxLatitude  = 90.0
	minLongitude = -180.0
	maxLongitude = 180.0
)

// File and directory permissions for a created sample. The configuration is
// personal, so it is readable by its owner only.
const (
	sampleFileMode fs.FileMode = 0o600
	sampleDirMode  fs.FileMode = 0o700
)

// DefaultPath is the configuration file location used when --config is not
// supplied: ~/.config/suntimes/config.toml (SUN-1).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine your home directory: %w", err)
	}
	return filepath.Join(home, ".config", "suntimes", "config.toml"), nil
}

// Resolve returns the configuration path to use. A non-empty flagPath (from
// --config) wins (SUN-2); otherwise the default location is used (SUN-1).
func Resolve(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	return DefaultPath()
}

// CreateSampleIfMissing writes the commented sample configuration to path when
// nothing exists there, creating parent directories as needed (SUN-3). It
// reports whether it created the file; an existing file is never overwritten.
func CreateSampleIfMissing(path string) (bool, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, sampleDirMode); err != nil {
			return false, fmt.Errorf("cannot create configuration directory %s: %w", dir, err)
		}
	}

	// O_EXCL makes "does it exist?" and "create it" a single step, so a file
	// that appears in between is left alone rather than clobbered.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sampleFileMode)
	switch {
	case errors.Is(err, fs.ErrExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("cannot create configuration file %s: %w", path, err)
	}
	defer file.Close()

	if _, err := file.WriteString(SampleFile); err != nil {
		return false, fmt.Errorf("cannot write configuration file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("cannot write configuration file %s: %w", path, err)
	}
	return true, nil
}

// Load reads and validates the configuration file at path. Parse failures
// (SUN-4) and missing or out-of-range coordinates (SUN-5) are returned as
// errors naming the file, for the caller to report before exiting non-zero.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	// The format is TOML regardless of the file's extension, which matters
	// when the user points --config at, say, suntimes.conf.
	v.SetConfigType("toml")

	if err := v.ReadInConfig(); err != nil {
		var parseErr viper.ConfigParseError
		if errors.As(err, &parseErr) {
			return nil, fmt.Errorf("%s is not valid TOML: %w", path, parseErr)
		}
		return nil, fmt.Errorf("cannot read configuration file %s: %w", path, err)
	}

	latitude, err := coordinate(v, path, keyLatitude, minLatitude, maxLatitude)
	if err != nil {
		return nil, err
	}
	longitude, err := coordinate(v, path, keyLongitude, minLongitude, maxLongitude)
	if err != nil {
		return nil, err
	}

	timezone, err := optionalString(v, path, keyTimezone)
	if err != nil {
		return nil, err
	}

	return &Config{
		Latitude:  latitude,
		Longitude: longitude,
		Timezone:  timezone,
		Path:      path,
	}, nil
}

// coordinate reads a required decimal-degrees value and checks it against its
// inclusive bounds (SUN-5). Presence is decided by the key being set, never by
// the value being zero: latitude 0, longitude 0 is a real place.
func coordinate(v *viper.Viper, path, key string, lowest, highest float64) (float64, error) {
	raw := v.Get(key)
	if raw == nil {
		return 0, fmt.Errorf("%s is missing the required %s value (decimal degrees, %g..%g)",
			path, key, lowest, highest)
	}

	degrees, ok := toDegrees(raw)
	if !ok {
		return 0, fmt.Errorf("%s has an invalid %s value %#v: expected a number in decimal degrees (%g..%g)",
			path, key, raw, lowest, highest)
	}
	if degrees < lowest || degrees > highest {
		return 0, fmt.Errorf("%s has %s %g, which is outside the valid range %g..%g",
			path, key, degrees, lowest, highest)
	}
	return degrees, nil
}

// toDegrees converts a decoded TOML value to decimal degrees. Only numbers
// qualify: coercing a string or a boolean would turn a typo into a location.
func toDegrees(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int64:
		return float64(value), true
	case int:
		return float64(value), true
	case int32:
		return float64(value), true
	default:
		return 0, false
	}
}

// optionalString reads a key that may be absent, rejecting a value of the
// wrong type rather than silently coercing it.
func optionalString(v *viper.Viper, path, key string) (string, error) {
	raw := v.Get(key)
	if raw == nil {
		return "", nil
	}

	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s has an invalid %s value %#v: expected a quoted string", path, key, raw)
	}
	return value, nil
}
