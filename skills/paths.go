package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validRelative(path string) bool {
	return path != "." && filepath.IsLocal(path) && filepath.Clean(path) == path
}

// physicalPath resolves root aliases, including roots not yet created. It never
// creates or replaces the user's discovery-root links.
func physicalPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		return physicalPath(target)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	parent, err = physicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && (rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."))
}

func canonicalRoots(roots []string) ([]string, error) {
	seen := map[string]bool{}
	var result []string
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("agent root must be absolute: %q", root)
		}
		abs, err := physicalPath(root)
		if err != nil {
			return nil, fmt.Errorf("resolve agent root %s: %w", root, err)
		}
		if !seen[abs] {
			seen[abs] = true
			result = append(result, abs)
		}
	}
	// Nested roots would let one operation invalidate another's checked parents.
	for i, a := range result {
		for j, b := range result {
			if i != j && within(a, b) {
				return nil, fmt.Errorf("overlapping agent roots %s and %s", a, b)
			}
		}
	}
	return result, nil
}

// Below a resolved root, every parent must be a real directory. Even force
// cannot authorize traversal into a symlink target.
func checkInstallDirectories(root, rel string) error {
	for dir := filepath.Dir(rel); dir != "."; dir = filepath.Dir(dir) {
		path := filepath.Join(root, dir)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("refusing to traverse non-directory or directory symlink %s", path)
		}
	}
	return nil
}

func entryKind(f ManagedFile) EntryKind {
	if f.Kind != "" {
		return f.Kind
	}
	if f.LinkTarget != "" {
		return KindFileLink
	}
	return KindCopiedFile
}

// entryIdentity inspects the entry itself, never its symlink target. A copied
// file replaced by a symlink is a conflict even if the target bytes match.
func entryIdentity(path string, f ManagedFile) (string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	kind := entryKind(f)
	if kind == KindDirectoryLink || kind == KindFileLink {
		if info.Mode()&os.ModeSymlink == 0 {
			return "\x00non-link", nil
		}
		link, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		resolved, err := physicalPath(link)
		if err != nil {
			return filepath.Clean(link), nil
		}
		return resolved, nil
	}
	if !info.Mode().IsRegular() {
		return "\x00non-file", nil
	}
	return hashFile(path)
}

func entryOwnership(path string, f ManagedFile) (Ownership, string, error) {
	current, err := entryIdentity(path, f)
	mode := ModeManagedCopy
	if entryKind(f) != KindCopiedFile {
		mode = ModeSymlink
	}
	return Decide(PathHashes{Installed: managedIdentity(f), Current: current}, mode), current, err
}

// Reports retain the caller's discovery-root spelling while writes use physical
// paths. This also makes an aliased-root report point at a recognizable location.
func displayTargets(p *installationPlan, roots []string) {
	for i := range p.targets {
		for _, root := range roots {
			physical, err := physicalPath(root)
			if err == nil && p.targets[i].Root == physical {
				p.targets[i].Root = root
				break
			}
		}
	}
}

// Source targets remain protected when the requested bundle comes from a
// temporary extraction, or when uninstall sees a root alias into a checkout.
func protectRecordedSources(path string, m Manifest) error {
	for _, f := range m.Files {
		if f.LinkTarget == "" {
			continue
		}
		source, err := physicalPath(f.LinkTarget)
		if err != nil {
			return err
		}
		if within(source, path) || (entryKind(f) == KindDirectoryLink && within(path, source)) {
			return fmt.Errorf("destination %s overlaps source %s", path, source)
		}
	}
	if checkout, ok := DevRoot(m); ok {
		source, err := physicalPath(filepath.Join(checkout, "skills"))
		if err != nil {
			return err
		}
		if within(source, path) || within(path, source) {
			return fmt.Errorf("destination %s overlaps source checkout %s", path, source)
		}
	}
	return nil
}
