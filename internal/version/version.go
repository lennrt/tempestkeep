// Package version reports the TempestKeep build version, shared by the tempest
// and tempest-mcp binaries.
package version

import "runtime/debug"

// version is injected at build time via
//
//	-ldflags "-X github.com/lennrt/tempestkeep/internal/version.version=v1.2.3"
//
// (the Makefile and goreleaser both do this). A plain `go build` leaves it empty.
var version string

// String returns the best available version: the ldflags-injected one, else the
// module version stamped by `go install module@version`, else "dev".
func String() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
