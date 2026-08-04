package skills

import (
	"errors"
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
//
// A path whose on-disk content is locally modified, or that holds content
// unrelated to any prior install of this skill, is left untouched unless
// req.Force explicitly authorizes it; Install then writes nothing at all
// and returns a *ConflictError, so a refused install never leaves behind a
// manifest claiming it succeeded. The same holds for any other failure
// encountered while copying: the manifest is only written after every file
// has been copied to every agent root.
func Install(manifestPath string, req InstallRequest) error {
	prev, err := Load(manifestPath)
	if err != nil && !errors.Is(err, ErrNotExist) {
		return fmt.Errorf("load existing manifest: %w", err)
	}
	prevHashes := make(map[string]string, len(prev.Files))
	for _, f := range prev.Files {
		prevHashes[f.Path] = f.Hash
	}

	sourceHashes := make(map[string]string, len(req.Files))
	for _, f := range req.Files {
		h, err := hashFile(f.AbsPath)
		if err != nil {
			return fmt.Errorf("hash source file %s: %w", f.AbsPath, err)
		}
		sourceHashes[f.RelPath] = h
	}

	conflicts := map[string]Ownership{}
	for _, root := range req.AgentRoots {
		for _, f := range req.Files {
			target := filepath.Join(root, f.RelPath)
			currentHash, err := hashIfExists(target)
			if err != nil {
				return fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := Decide(PathHashes{Installed: prevHashes[f.RelPath], Current: currentHash}, ModeManagedCopy)
			if !AllowWrite(ownership, f.RelPath, req.Force) {
				conflicts[f.RelPath] = ownership
			}
		}
	}
	if len(conflicts) > 0 {
		return &ConflictError{Conflicts: conflicts}
	}

	for _, root := range req.AgentRoots {
		if err := os.MkdirAll(root, 0755); err != nil {
			return fmt.Errorf("create agent root %s: %w", root, err)
		}
		for _, f := range req.Files {
			target := filepath.Join(root, f.RelPath)
			if err := copyFile(f.AbsPath, target); err != nil {
				return fmt.Errorf("install %s: %w", target, err)
			}
		}
	}

	files := make([]ManagedFile, 0, len(req.Files))
	for _, f := range req.Files {
		files = append(files, ManagedFile{Path: f.RelPath, Hash: sourceHashes[f.RelPath]})
	}
	newManifest := Manifest{
		Mode:       ModeManagedCopy,
		Source:     req.Source,
		AgentRoots: req.AgentRoots,
		Files:      files,
	}
	if err := Save(manifestPath, newManifest); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	return nil
}
