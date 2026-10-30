package skills

import (
	"fmt"
	"reflect"
)

// SourceFile is one skill file to install: RelPath is where it is written,
// relative to both the manifest's Files entries and every agent root;
// AbsPath is where its content currently lives on disk.
type SourceFile struct {
	RelPath string
	AbsPath string
}

// InstallRequest describes a skill installation or upgrade attempt.
type InstallRequest struct {
	// Source is a human-readable description of the skill's origin, stored
	// in the manifest.
	Source string
	// AgentRoots are the agent skill roots to install into. Missing roots
	// are created.
	AgentRoots []string
	// Files are the skill's files to install.
	Files []SourceFile
	// Force authorizes overwriting specific conflicting paths.
	Force ForcePolicy
	// Mode selects how files are placed on disk. The zero value is
	// ModeManagedCopy.
	Mode InstallMode
	// BackupDir retains dated recovery snapshots. Defaults beside the manifest.
	BackupDir string
}

// effectiveMode returns r.Mode, defaulting to ModeManagedCopy when unset.
func (r InstallRequest) effectiveMode() InstallMode {
	if r.Mode == "" {
		return ModeManagedCopy
	}
	return r.Mode
}

// ConflictError reports the paths an install or uninstall refused to
// touch because they're locally modified or hold unrelated content, none
// of which the request's Force policy authorized.
type ConflictError struct {
	Conflicts map[string]Ownership
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("refused %d conflicting path(s), use force to override", len(e.Conflicts))
}

// Install plans the complete update before changing any destination, retains
// recoverable backups, and publishes the manifest after all writes succeed.
func Install(manifestPath string, req InstallRequest) ([]Target, error) {
	unlock, err := lockInstallation(manifestPath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := buildInstallPlan(manifestPath, req)
	if err != nil {
		return nil, err
	}
	if err := p.conflictError(); err != nil {
		return p.targets, err
	}
	if len(p.operations) == 0 {
		prev, err := Load(manifestPath)
		if err == nil && reflect.DeepEqual(prev, p.manifest) {
			return p.targets, nil
		}
	}
	backup, err := executePlan(p, manifestPath, func() error { return Save(manifestPath, p.manifest) })
	for i := range p.targets {
		p.targets[i].BackupDir = backup
	}
	return p.targets, err
}
