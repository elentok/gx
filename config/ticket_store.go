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
	// CommitDebounce is the quiet period, in seconds, the store commit loop
	// waits after a change before committing.
	CommitDebounce int `json:"commit-debounce"`
}

// DefaultCommitDebounceSeconds is ticket-store.commit-debounce's default.
const DefaultCommitDebounceSeconds = 60

// DefaultTicketStoreConfig puts the store at <data dir>/tickets. If the data
// dir can't be resolved the path is left empty and callers must refuse.
func DefaultTicketStoreConfig() TicketStoreConfig {
	cfg := TicketStoreConfig{CommitDebounce: DefaultCommitDebounceSeconds}
	if dir, err := DataDir(); err == nil {
		cfg.Path = filepath.Join(dir, "tickets")
	}
	return cfg
}

// ProjectFile is a project's project.json. Pointer fields so an absent key
// stays distinguishable from an empty value.
type ProjectFile struct {
	Name *string `json:"name"`
	Repo *string `json:"repo"`
	// VCS is "none" for a repo-less project (the built-in scratch project).
	VCS *string `json:"vcs"`
}

// VCSNone is the project.json vcs value for a project with no repository.
const VCSNone = "none"

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
