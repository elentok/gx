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
	// PushRemote is a git remote of the store pushed to after each commit,
	// best effort. Empty means no push.
	PushRemote string `json:"push-remote"`
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

	// The rest are the whitelisted overrides layered over config.json; see
	// project_overrides.go.
	Trunk          *string              `json:"trunk"`
	Landing        *string              `json:"landing"`
	AutoFFMerge    *bool                `json:"auto-ff-merge"`
	MaxAgents      *int                 `json:"max-agents"`
	Agents         *AgentsConfig        `json:"agents"`
	Skills         *SkillsConfig        `json:"skills"`
	Notifications  *NotificationsConfig `json:"notifications"`
	ExecutionQueue *struct {
		MaxAgentsPerEpic *int `json:"max-agents-per-epic"`
	} `json:"execution-queue"`
}

// LandingOnLanded is the landing policy where a blocker releases its dependents
// once it has landed on trunk. It is the default.
const LandingOnLanded = "on-landed"

// LandingPolicy is the project's landing policy; absent or empty means the default.
func (pf ProjectFile) LandingPolicy() string {
	if pf.Landing == nil || *pf.Landing == "" {
		return LandingOnLanded
	}
	return *pf.Landing
}

// AutoFFMergeEnabled reports whether the project opted in to the server
// fast-forwarding a finished epic onto its target. Absent means off.
func (pf ProjectFile) AutoFFMergeEnabled() bool {
	return pf.AutoFFMerge != nil && *pf.AutoFFMerge
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
	if err := checkProjectKeys(data); err != nil {
		return pf, fmt.Errorf("%s: %w", path, err)
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return pf, fmt.Errorf("parse %s: %w", path, err)
	}
	return pf, nil
}
