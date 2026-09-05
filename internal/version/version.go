// Package version reports the TempestKeep build version.
package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

// version is injected at build time via
//
//	-ldflags "-X github.com/lennrt/tempestkeep/internal/version.version=v1.2.3"
//
// (the Makefile and goreleaser both do this). A plain `go build` leaves it empty.
var version string

//go:embed VERSION
var releaseVersion string

// String returns the best available version: the ldflags-injected one, else the
// module version stamped by `go install module@version`, else the next release
// with a "-dev" suffix.
func String() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "v" + strings.TrimSpace(releaseVersion) + "-dev"
}
