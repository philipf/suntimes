// Package buildinfo carries build-time metadata about the suntimes binary.
//
// Values here are overridden at link time by the Makefile, e.g.
//
//	go build -ldflags "-X github.com/philipf/suntimes/internal/buildinfo.Version=v1.2.3"
package buildinfo

// Version is the application version. It defaults to "dev" for local,
// unstamped builds and is replaced at link time for released builds.
var Version = "dev"
