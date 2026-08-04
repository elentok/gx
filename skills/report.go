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

// Target is one agent-root/relative-path pair, the Ownership classify (see
// classifyInstall/classifyUninstall) decided for it, and the resulting
// status.
type Target struct {
	Root      string
	Path      string
	Ownership Ownership
	Status    TargetStatus
}

// RootRelPath joins Root and Path for display.
func (t Target) RootRelPath() string {
	return filepath.Join(t.Root, t.Path)
}

// Plan classifies every (agent root, file) pair in req against the manifest
// at manifestPath and req.Force, without writing anything, using the same
// classification Install itself performs - so a caller can report what
// Install would do before, or in place of, actually doing it.
func Plan(manifestPath string, req InstallRequest) ([]Target, error) {
	targets, _, err := classifyInstall(manifestPath, req)
	return targets, err
}

// classifyInstall is the classification pass both Plan and Install run:
// load the manifest, work out what each file's on-disk identity would be
// once installed, and decide every (agent root, file) target's ownership
// and resulting status. Install reuses its output directly instead of
// classifying twice, so the plan it reports and the writes it performs can
// never diverge. It also returns each file's about-to-be-installed
// identity, keyed by RelPath, so Install doesn't need to recompute it when
// writing the manifest.
func classifyInstall(manifestPath string, req InstallRequest) ([]Target, map[string]string, error) {
	mode := req.effectiveMode()

	prev, err := Load(manifestPath)
	if err != nil && !errors.Is(err, ErrNotExist) {
		return nil, nil, fmt.Errorf("load existing manifest: %w", err)
	}
	prevIdentities := make(map[string]string, len(prev.Files))
	for _, f := range prev.Files {
		prevIdentities[f.Path] = managedIdentity(f)
	}

	sourceIdentities := make(map[string]string, len(req.Files))
	for _, f := range req.Files {
		id, err := sourceFileIdentity(f, mode)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect source file %s: %w", f.AbsPath, err)
		}
		sourceIdentities[f.RelPath] = id
	}

	var targets []Target
	for _, root := range req.AgentRoots {
		for _, f := range req.Files {
			target := filepath.Join(root, f.RelPath)
			currentIdentity, err := identityIfExists(target, mode)
			if err != nil {
				return nil, nil, fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := installOwnership(prevIdentities[f.RelPath], currentIdentity, sourceIdentities[f.RelPath], mode)
			targets = append(targets, Target{
				Root:      root,
				Path:      f.RelPath,
				Ownership: ownership,
				Status:    installStatus(ownership, sourceIdentities[f.RelPath], currentIdentity, req.Force, f.RelPath),
			})
		}
	}
	return targets, sourceIdentities, nil
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
// force, without removing anything, using the same classification Uninstall
// itself performs. A path Uninstall would leave untouched because nothing
// exists there is omitted, matching Uninstall's own no-op behavior for it.
func PlanUninstall(manifestPath string, force ForcePolicy) ([]Target, error) {
	targets, _, err := classifyUninstall(manifestPath, force)
	return targets, err
}

// classifyUninstall is the classification pass both PlanUninstall and
// Uninstall run: load the manifest and decide every manifest-owned path's
// ownership and resulting status. Uninstall reuses its output directly
// instead of classifying twice, so the plan it reports and the removals it
// performs can never diverge. It also returns the loaded manifest so
// Uninstall doesn't need to reload it.
func classifyUninstall(manifestPath string, force ForcePolicy) ([]Target, Manifest, error) {
	m, err := Load(manifestPath)
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("load manifest: %w", err)
	}

	var targets []Target
	for _, f := range m.Files {
		for _, root := range m.AgentRoots {
			target := filepath.Join(root, f.Path)
			currentIdentity, err := identityIfExists(target, m.Mode)
			if err != nil {
				return nil, Manifest{}, fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := Decide(PathHashes{Installed: managedIdentity(f), Current: currentIdentity}, m.Mode)
			if ownership == OwnershipAbsent {
				continue
			}
			status := StatusRemoved
			if !AllowWrite(ownership, f.Path, force) {
				status = StatusConflicted
			}
			targets = append(targets, Target{Root: root, Path: f.Path, Ownership: ownership, Status: status})
		}
	}
	return targets, m, nil
}
