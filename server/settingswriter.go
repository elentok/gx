package server

import (
	"github.com/elentok/gx/config/registry"
	"github.com/elentok/gx/server/settingsapi"
)

// newSettingsWriter is the writer over the global config file and the
// projects the store lists.
func (s *Server) newSettingsWriter(configPath string) *settingsapi.Writer {
	return &settingsapi.Writer{Registry: registry.Default(), GlobalPath: configPath, ProjectDir: s.projects.Dir}
}

func (s *Server) pendingRestart() []string { return s.settings.Status().PendingRestart }
