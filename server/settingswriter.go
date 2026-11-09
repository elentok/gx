package server

import (
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/settingsapi"
)

// newSettingsWriter is the writer over the global config file and the
// projects the registry lists.
func newSettingsWriter(configPath string, reg *projects.Registry) *settingsapi.Writer {
	return &settingsapi.Writer{Registry: registry.Default(), GlobalPath: configPath, ProjectDir: reg.Dir}
}
