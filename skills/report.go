package skills

import (
	"errors"
	"fmt"
	"path/filepath"
)

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

// Target is one agent-root/relative-path pair and the status Plan or
// PlanUninstall determined for it.
type Target struct {
	Root   string
	Path   string
	Status TargetStatus
}

// RootRelPath joins Root and Path for display.
func (t Target) RootRelPath() string {
	return filepath.Join(t.Root, t.Path)
}

// Plan classifies every (agent root, file) pair in req against the manifest
// at manifestPath and req.Force, without writing anything, mirroring the
// classification Install itself performs - so a caller can report what
// Install would do before, or in place of, actually doing it.
func Plan(manifestPath string, req InstallRequest) ([]Target, error) {
	mode := req.effectiveMode()

	prev, err := Load(manifestPath)
	if err != nil && !errors.Is(err, ErrNotExist) {
		return nil, fmt.Errorf("load existing manifest: %w", err)
	}
	prevIdentities := make(map[string]string, len(prev.Files))
	for _, f := range prev.Files {
		prevIdentities[f.Path] = managedIdentity(f)
	}

	sourceIdentities := make(map[string]string, len(req.Files))
	for _, f := range req.Files {
		id, err := sourceFileIdentity(f, mode)
		if err != nil {
			return nil, fmt.Errorf("inspect source file %s: %w", f.AbsPath, err)
		}
		sourceIdentities[f.RelPath] = id
	}

	var targets []Target
	for _, root := range req.AgentRoots {
		for _, f := range req.Files {
			target := filepath.Join(root, f.RelPath)
			currentIdentity, err := identityIfExists(target, mode)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := installOwnership(prevIdentities[f.RelPath], currentIdentity, sourceIdentities[f.RelPath], mode)
			targets = append(targets, Target{
				Root:   root,
				Path:   f.RelPath,
				Status: installStatus(ownership, sourceIdentities[f.RelPath], currentIdentity, req.Force, f.RelPath),
			})
		}
	}
	return targets, nil
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

// PlanUninstall classifies every manifest-owned path at manifestPath against
// force, without removing anything, mirroring the classification Uninstall
// itself performs. A path Uninstall would leave untouched because nothing
// exists there is omitted, matching Uninstall's own no-op behavior for it.
func PlanUninstall(manifestPath string, force ForcePolicy) ([]Target, error) {
	m, err := Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	var targets []Target
	for _, f := range m.Files {
		for _, root := range m.AgentRoots {
			target := filepath.Join(root, f.Path)
			currentIdentity, err := identityIfExists(target, m.Mode)
			if err != nil {
				return nil, fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := Decide(PathHashes{Installed: managedIdentity(f), Current: currentIdentity}, m.Mode)
			if ownership == OwnershipAbsent {
				continue
			}
			status := StatusRemoved
			if !AllowWrite(ownership, f.Path, force) {
				status = StatusConflicted
			}
			targets = append(targets, Target{Root: root, Path: f.Path, Status: status})
		}
	}
	return targets, nil
}
