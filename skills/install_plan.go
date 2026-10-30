package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type operation struct {
	path   string
	entry  ManagedFile
	data   []byte
	remove bool
}

type installationPlan struct {
	manifest   Manifest
	targets    []Target
	operations []operation
	backupDir  string
}

func (p *installationPlan) conflictError() error {
	conflicts := map[string]Ownership{}
	for _, t := range p.targets {
		if t.Status == StatusConflicted {
			conflicts[t.Path] = t.Ownership
		}
	}
	if len(conflicts) > 0 {
		return &ConflictError{Conflicts: conflicts}
	}
	return nil
}

func desiredEntries(req InstallRequest) ([]ManagedFile, map[string][]byte, error) {
	files := map[string]SourceFile{}
	directories := map[string]string{}
	for _, f := range req.Files {
		if !validRelative(f.RelPath) || !filepath.IsAbs(f.AbsPath) {
			return nil, nil, fmt.Errorf("invalid source path %q: %s", f.RelPath, f.AbsPath)
		}
		if _, ok := files[f.RelPath]; ok {
			return nil, nil, fmt.Errorf("duplicate source path %s", f.RelPath)
		}
		info, err := os.Stat(f.AbsPath)
		if err != nil {
			return nil, nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("source is not a regular file: %s", f.AbsPath)
		}
		files[f.RelPath] = f
		parts := strings.Split(f.RelPath, string(filepath.Separator))
		if len(parts) == 2 && parts[1] == "SKILL.md" && req.effectiveMode() == ModeSymlink {
			marker, err := os.Lstat(f.AbsPath)
			if err != nil {
				return nil, nil, err
			}
			if !marker.Mode().IsRegular() {
				return nil, nil, fmt.Errorf("dev skill SKILL.md must be a real file: %s", f.AbsPath)
			}
			directories[parts[0]] = filepath.Dir(f.AbsPath)
		}
	}
	result := map[string]ManagedFile{}
	data := map[string][]byte{}
	for _, f := range req.Files {
		top := strings.Split(f.RelPath, string(filepath.Separator))[0]
		if dir, ok := directories[top]; ok {
			// All files in a linked skill must come from that same source directory.
			relative, _ := filepath.Rel(top, f.RelPath)
			actual, err := physicalPath(f.AbsPath)
			if err != nil {
				return nil, nil, err
			}
			expected, err := physicalPath(filepath.Join(dir, relative))
			if err != nil {
				return nil, nil, err
			}
			if actual != expected {
				return nil, nil, fmt.Errorf("inconsistent source directory for %s", f.RelPath)
			}
			result[top] = ManagedFile{Path: top, Kind: KindDirectoryLink, LinkTarget: dir}
			continue
		}
		entry := ManagedFile{Path: f.RelPath, Kind: KindCopiedFile}
		if req.effectiveMode() == ModeSymlink {
			entry.Kind = KindFileLink
			entry.LinkTarget = f.AbsPath
		} else {
			bytes, err := os.ReadFile(f.AbsPath)
			if err != nil {
				return nil, nil, err
			}
			entry.Hash = hashBytes(bytes)
			data[f.RelPath] = bytes
		}
		result[entry.Path] = entry
	}
	var entries []ManagedFile
	for _, f := range result {
		entries = append(entries, f)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if err := validateEntries(entries, CurrentSchemaVersion); err != nil {
		return nil, nil, err
	}
	return entries, data, nil
}

func ownedPaths(m Manifest) (map[string]ManagedFile, error) {
	roots, err := canonicalRoots(m.AgentRoots)
	if err != nil {
		return nil, err
	}
	result := map[string]ManagedFile{}
	for _, root := range roots {
		for _, f := range m.Files {
			result[filepath.Join(root, f.Path)] = f
		}
	}
	return result, nil
}

func buildInstallPlan(manifestPath string, req InstallRequest) (*installationPlan, error) {
	if req.effectiveMode() != ModeSymlink && req.effectiveMode() != ModeManagedCopy {
		return nil, fmt.Errorf("invalid install mode %q", req.Mode)
	}
	prev, err := Load(manifestPath)
	if err != nil && !errors.Is(err, ErrNotExist) {
		return nil, err
	}
	_, err = canonicalRoots(append(append([]string{}, req.AgentRoots...), prev.AgentRoots...))
	if err != nil {
		return nil, err
	}
	roots, err := canonicalRoots(req.AgentRoots)
	if err != nil {
		return nil, err
	}
	old, err := ownedPaths(prev)
	if err != nil {
		return nil, err
	}
	entries, data, err := desiredEntries(req)
	if err != nil {
		return nil, err
	}
	backupDir := req.BackupDir
	if backupDir == "" {
		backupDir = prev.BackupDir
	}
	if backupDir == "" {
		backupDir = filepath.Join(filepath.Dir(manifestPath), "backups")
	}
	p := &installationPlan{backupDir: backupDir, manifest: Manifest{SchemaVersion: CurrentSchemaVersion, Mode: req.effectiveMode(), Source: req.Source, AgentRoots: req.AgentRoots, Files: entries, BackupDir: backupDir}}
	retained := map[string]bool{}
	replaced := map[string]bool{}
	modeChange := prev.Mode != "" && prev.Mode != req.effectiveMode()
	retarget := false
	if oldRoot, ok := DevRoot(prev); ok && req.effectiveMode() == ModeSymlink {
		if newRoot, ok := DevRoot(Manifest{Mode: req.effectiveMode(), Source: req.Source}); ok {
			a, err := physicalPath(oldRoot)
			if err != nil {
				return nil, err
			}
			b, err := physicalPath(newRoot)
			if err != nil {
				return nil, err
			}
			retarget = a != b
		}
	}
	protectedSources := []string{}
	for _, f := range req.Files {
		src, err := physicalPath(f.AbsPath)
		if err != nil {
			return nil, err
		}
		protectedSources = append(protectedSources, src)
	}
	// Protect all source files from direct replacement, including an agent root
	// that aliases the checkout itself. Removing a link to a source is safe.
	protect := func(path string) error {
		if err := protectRecordedSources(path, prev); err != nil {
			return err
		}
		for _, src := range protectedSources {
			if within(src, path) {
				return fmt.Errorf("destination %s contains source %s", path, src)
			}
		}
		return nil
	}
	for _, root := range roots {
		// A copy install replacing a directory link must unlink it as one entry
		// before writing its children. Never hash or write through that link.
		converted := map[string]bool{}
		for _, f := range entries {
			if entryKind(f) != KindCopiedFile {
				continue
			}
			parts := strings.Split(f.Path, string(filepath.Separator))
			if len(parts) < 2 {
				continue
			}
			top := parts[0]
			path := filepath.Join(root, top)
			if converted[top] {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if err == nil && !info.IsDir() {
				owner := OwnershipUnrelatedCollision
				if prior, ok := old[path]; ok {
					owner, _, err = entryOwnership(path, prior)
					if err != nil {
						return nil, err
					}
				}
				allowed := req.Force.Allows(top)
				status := StatusConflicted
				if allowed {
					status = StatusUpdated
				}
				p.targets = append(p.targets, Target{Root: root, Path: top, Ownership: owner, Status: status})
				if allowed {
					if err := protect(path); err != nil {
						return nil, err
					}
					p.operations = append(p.operations, operation{path: path, remove: true})
				}
				converted[top] = true
				replaced[path] = true
			}
		}
		for _, f := range entries {
			path := filepath.Join(root, f.Path)
			retained[path] = true
			underReplacement := false
			for parent := range replaced {
				if path != parent && within(path, parent) {
					underReplacement = true
				}
			}
			if !underReplacement {
				if err := checkInstallDirectories(root, f.Path); err != nil {
					return nil, err
				}
			}
			current := ""
			owner := OwnershipAbsent
			if !underReplacement {
				var err error
				owner, current, err = entryOwnership(path, f)
				if err != nil {
					return nil, err
				}
				if prior, ok := old[path]; ok {
					owner, _, err = entryOwnership(path, prior)
					if err != nil {
						return nil, err
					}
					// A changed link target always needs force, even if the old link is intact.
					if f.LinkTarget != "" && managedIdentity(prior) != managedIdentity(f) && current != "" {
						owner = OwnershipWrongSymlinkTarget
					}
				} else if current != "" {
					owner = OwnershipUnrelatedCollision
					if f.LinkTarget != "" && current == managedIdentity(f) {
						owner = OwnershipUnchanged
					}
					if f.Kind == KindDirectoryLink && current == "\x00non-link" {
						owner, err = legacyDirectoryOwnership(path, old)
						if err != nil {
							return nil, err
						}
					}
				}
			}
			status := installStatus(owner, managedIdentity(f), current, req.Force, f.Path)
			if (modeChange || retarget) && !req.Force.Allows(f.Path) && !underReplacement {
				status = StatusConflicted
			}
			p.targets = append(p.targets, Target{Root: root, Path: f.Path, Ownership: owner, Status: status})
			if status == StatusInstalled || status == StatusUpdated {
				if err := protect(path); err != nil {
					return nil, err
				}
				p.operations = append(p.operations, operation{path: path, entry: f, data: data[f.Path]})
				if f.Kind == KindDirectoryLink {
					replaced[path] = true
				}
			}
			// Adoption of a correct link supersedes legacy child records without
			// inspecting or deleting anything inside its target.
			if f.Kind == KindDirectoryLink {
				replaced[path] = true
			}
		}
	}
	// Remove stale entries and retired roots only after the new placement. Physical
	// keys make aliases retained above immune to old-root cleanup.
	keys := make([]string, 0, len(old))
	for path := range old {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		if retained[path] {
			continue
		}
		covered := false
		for parent := range replaced {
			if within(path, parent) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		f := old[path]
		root := strings.TrimSuffix(path, string(filepath.Separator)+f.Path)
		if err := checkInstallDirectories(root, f.Path); err != nil {
			return nil, err
		}
		owner, _, err := entryOwnership(path, f)
		if err != nil {
			return nil, err
		}
		if owner == OwnershipAbsent {
			continue
		}
		status := StatusRemoved
		if !AllowWrite(owner, f.Path, req.Force) {
			status = StatusConflicted
		}
		p.targets = append(p.targets, Target{Root: root, Path: f.Path, Ownership: owner, Status: status})
		if status == StatusRemoved {
			if err := protect(path); err != nil {
				return nil, err
			}
			p.operations = append(p.operations, operation{path: path, remove: true})
		}
	}
	displayTargets(p, req.AgentRoots)
	return p, nil
}

// A v1 manifest owns individual children, never their directory. A real directory
// can be replaced only when every existing leaf is verified as owned and intact.
func legacyDirectoryOwnership(path string, old map[string]ManagedFile) (Ownership, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return OwnershipAbsent, err
	}
	if !info.IsDir() {
		return OwnershipUnrelatedCollision, nil
	}
	found := false
	for child := range old {
		if child != path && within(child, path) {
			found = true
		}
	}
	if !found {
		return OwnershipUnrelatedCollision, nil
	}
	owner := OwnershipUnchanged
	err = filepath.WalkDir(path, func(child string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if child == path {
			return nil
		}
		if d.IsDir() {
			// An empty unrelated subdirectory is content too.
			known := false
			for owned := range old {
				if within(owned, child) {
					known = true
					break
				}
			}
			if !known {
				owner = OwnershipUnrelatedCollision
				return filepath.SkipDir
			}
			return nil
		}
		f, ok := old[child]
		if !ok {
			owner = OwnershipUnrelatedCollision
			return nil
		}
		o, _, err := entryOwnership(child, f)
		if err != nil {
			return err
		}
		if o != OwnershipUnchanged {
			owner = o
		}
		return nil
	})
	return owner, err
}
