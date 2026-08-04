package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// SourceFile is one skill file to install: RelPath is where it lands,
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

// Install installs or upgrades a skill's managed-copy files into every
// AgentRoot in req, recording the result in the manifest at manifestPath.
// It returns the same per-target classification Plan would have produced -
// classified once, via classifyInstall, and reused for both the writes and
// the returned report - so a caller never needs a separate Plan call just
// to report what Install did.
//
// A path whose on-disk content is locally modified, or that holds content
// unrelated to any prior install of this skill, is left untouched unless
// req.Force explicitly authorizes it; Install then writes nothing at all
// and returns the classification alongside a *ConflictError, so a refused
// install never leaves behind a manifest claiming it succeeded. The same
// holds for any other failure encountered while copying: the manifest is
// only written after every file has been copied to every agent root.
func Install(manifestPath string, req InstallRequest) ([]Target, error) {
	mode := req.effectiveMode()

	targets, sourceIdentities, err := classifyInstall(manifestPath, req)
	if err != nil {
		return nil, err
	}

	conflicts := map[string]Ownership{}
	for _, t := range targets {
		if t.Status == StatusConflicted {
			conflicts[t.Path] = t.Ownership
		}
	}
	if len(conflicts) > 0 {
		return targets, &ConflictError{Conflicts: conflicts}
	}

	for _, root := range req.AgentRoots {
		if err := os.MkdirAll(root, 0755); err != nil {
			return targets, fmt.Errorf("create agent root %s: %w", root, err)
		}
		for _, f := range req.Files {
			target := filepath.Join(root, f.RelPath)
			if err := placeFile(f.AbsPath, target, mode); err != nil {
				return targets, fmt.Errorf("install %s: %w", target, err)
			}
		}
	}

	files := make([]ManagedFile, 0, len(req.Files))
	for _, f := range req.Files {
		mf := ManagedFile{Path: f.RelPath}
		if mode == ModeSymlink {
			mf.LinkTarget = sourceIdentities[f.RelPath]
		} else {
			mf.Hash = sourceIdentities[f.RelPath]
		}
		files = append(files, mf)
	}
	newManifest := Manifest{
		Mode:       mode,
		Source:     req.Source,
		AgentRoots: req.AgentRoots,
		Files:      files,
	}
	if err := Save(manifestPath, newManifest); err != nil {
		return targets, fmt.Errorf("save manifest: %w", err)
	}
	return targets, nil
}
