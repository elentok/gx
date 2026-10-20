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

const ProjectSchemaURL = "https://raw.githubusercontent.com/elentok/gx/main/docs/project-schema.json"

// ProjectFile is a project's project.json. Pointer fields so an absent key
// stays distinguishable from an empty value; omitempty so an absent key is
// written absent, not as a null the schema rejects.
type ProjectFile struct {
	Schema *string `json:"$schema,omitempty"`
	Name   *string `json:"name,omitempty"`
	Repo   *string `json:"repo,omitempty"`
	// VCS is "none" for a repo-less project (the built-in scratch project).
	VCS *string `json:"vcs,omitempty"`

	// The rest are the whitelisted overrides layered over config.json; see
	// project_overrides.go.
	Trunk         *string              `json:"trunk,omitempty"`
	Landing       *string              `json:"landing,omitempty"`
	AutoFFMerge   *bool                `json:"auto-ff-merge,omitempty"`
	MaxAgents     *int                 `json:"max-agents,omitempty"`
	Agents        *AgentsConfig        `json:"agents,omitempty"`
	Skills        *SkillsConfig        `json:"skills,omitempty"`
	Notifications *NotificationsConfig `json:"notifications,omitempty"`
	// Evolve false opts the project out of evolve; absent or true follows the
	// global mode.
	Evolve         *bool `json:"evolve,omitempty"`
	ExecutionQueue *struct {
		MaxAgentsPerEpic *int `json:"max-agents-per-epic,omitempty"`
		MaxOneOffAgents  *int `json:"max-one-off-agents,omitempty"`
	} `json:"execution-queue,omitempty"`
}

// LandingOnLanded is the landing policy where a blocker releases its dependents
// once it has landed on trunk. It is the default.
const LandingOnLanded = "on-landed"

// LandingOnDone is the landing policy where a blocker releases its dependents
// once it is done, before it lands; they base on the blocker's branch.
const LandingOnDone = "on-done"

// LandingPolicy is the project's landing policy; absent, empty or unknown
// means the default.
func (pf ProjectFile) LandingPolicy() string {
	if pf.Landing != nil && *pf.Landing == LandingOnDone {
		return LandingOnDone
	}
	return LandingOnLanded
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
	pf, err = ParseProjectFile(data)
	if err != nil {
		return pf, fmt.Errorf("%s: %w", path, err)
	}
	return pf, nil
}

// ParseProjectFile is the project.json loader: key whitelist and types only.
// Unknown enum values load and fall back to defaults, so one typo cannot make
// a project — or a store scan over every project — unreadable.
func ParseProjectFile(data []byte) (ProjectFile, error) {
	var pf ProjectFile
	if err := checkProjectKeys(data); err != nil {
		return pf, err
	}
	if err := json.Unmarshal(data, &pf); err != nil {
		return pf, fmt.Errorf("parse: %w", err)
	}
	return pf, nil
}

// ValidateProjectFile is the stricter check a person's edit must pass.
func ValidateProjectFile(pf ProjectFile) error {
	if pf.VCS != nil && *pf.VCS != VCSNone {
		return fmt.Errorf("vcs: %q is not allowed (want %q or absent)", *pf.VCS, VCSNone)
	}
	if pf.Landing != nil && *pf.Landing != "" && *pf.Landing != LandingOnLanded && *pf.Landing != LandingOnDone {
		return fmt.Errorf("landing: %q is not allowed (want %q or %q)", *pf.Landing, LandingOnLanded, LandingOnDone)
	}
	return nil
}
