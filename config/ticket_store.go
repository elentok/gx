package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectFileName is the file that makes a ticket-store directory a project.
const ProjectFileName = "project.json"

// TicketStoreConfig locates the global ticket store.
type TicketStoreConfig struct {
	Path string `json:"path"`
}

// DefaultTicketStoreConfig puts the store at <data dir>/tickets. If the data
// dir can't be resolved the path is left empty and callers must refuse.
func DefaultTicketStoreConfig() TicketStoreConfig {
	dir, err := DataDir()
	if err != nil {
		return TicketStoreConfig{}
	}
	return TicketStoreConfig{Path: filepath.Join(dir, "tickets")}
}

// ProjectFile is a project's project.json. Pointer fields so an absent key
// stays distinguishable from an empty value.
type ProjectFile struct {
	Name *string `json:"name"`
	Repo *string `json:"repo"`
}

// ReadProjectFile reads <projectDir>/project.json.
func ReadProjectFile(projectDir string) (ProjectFile, error) {
	path := filepath.Join(projectDir, ProjectFileName)
	var pf ProjectFile
	data, err := os.ReadFile(path)
	if err != nil {
		return pf, err
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return pf, fmt.Errorf("parse %s: %w", path, err)
	}
	return pf, nil
}
