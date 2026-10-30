package skills

import "path/filepath"

// TargetStatus classifies what Install or Uninstall did, or would do, to a
// single (agent root, relative path) target.
type TargetStatus string

const (
	StatusInstalled  TargetStatus = "installed"
	StatusUpdated    TargetStatus = "updated"
	StatusSkipped    TargetStatus = "skipped"
	StatusConflicted TargetStatus = "conflicted"
	StatusRemoved    TargetStatus = "removed"
)

// Target reports one discovery-root entry and the result of its preflight.
type Target struct {
	Root      string
	Path      string
	Ownership Ownership
	Status    TargetStatus
	BackupDir string
}

// RootRelPath joins Root and Path for display.
func (t Target) RootRelPath() string {
	return filepath.Join(t.Root, t.Path)
}

// Plan reports the same preflight as Install without changing the installation.
func Plan(manifestPath string, req InstallRequest) ([]Target, error) {
	p, err := buildInstallPlan(manifestPath, req)
	if err != nil {
		return nil, err
	}
	return p.targets, nil
}

func installStatus(ownership Ownership, sourceHash, currentHash string, force ForcePolicy, relPath string) TargetStatus {
	if !AllowWrite(ownership, relPath, force) {
		return StatusConflicted
	}
	if ownership == OwnershipAbsent {
		return StatusInstalled
	}
	if sourceHash == currentHash {
		return StatusSkipped
	}
	return StatusUpdated
}

// PlanUninstall reports owned paths that can be removed or must be preserved.
func PlanUninstall(manifestPath string, force ForcePolicy) ([]Target, error) {
	p, err := buildUninstallPlan(manifestPath, force)
	if err != nil {
		return nil, err
	}
	return p.targets, nil
}
