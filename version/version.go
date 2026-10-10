// Package version resolves gx's build version. It is a leaf package so both
// cmd and ui/help can reach it without an import cycle.
package version

import "runtime/debug"

// version is set at build time via -ldflags "-X github.com/elentok/gx/version.version=vX.Y.Z"
var version = ""

// Get returns the ldflags-injected version, else the module version from the
// build info, else "dev".
func Get() string {
	return resolve(version, debug.ReadBuildInfo)
}

func resolve(injected string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if injected != "" {
		return injected
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "(devel)" && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}
