package skills

// PathHashes carries what's known about a single managed path's identity
// before an install or uninstall operation decides whether it's safe to
// write there. Installed is the hash or link target the manifest recorded
// for this path, empty if the manifest never owned it. Current is the same
// kind of value read from disk right now, empty if nothing exists there.
type PathHashes struct {
	Installed string
	Current   string
}

// Ownership classifies a single managed path against the manifest's record
// of it.
type Ownership int

const (
	// OwnershipAbsent means there is nothing on disk to protect: either the
	// manifest never owned this path, or it did but the path no longer
	// exists. Safe to create.
	OwnershipAbsent Ownership = iota
	// OwnershipUnchanged means the manifest owns this path and its on-disk
	// content still matches what gx installed. Safe to overwrite or remove.
	OwnershipUnchanged
	// OwnershipModified means the manifest owns this path (managed-copy
	// mode) but its on-disk content diverged from what gx installed.
	OwnershipModified
	// OwnershipWrongSymlinkTarget means the manifest owns this path (symlink
	// mode) but the on-disk symlink no longer points at the source gx
	// recorded.
	OwnershipWrongSymlinkTarget
	// OwnershipUnrelatedCollision means the manifest has never owned this
	// path, but something already exists there.
	OwnershipUnrelatedCollision
)

func (o Ownership) String() string {
	switch o {
	case OwnershipAbsent:
		return "absent"
	case OwnershipUnchanged:
		return "unchanged"
	case OwnershipModified:
		return "modified"
	case OwnershipWrongSymlinkTarget:
		return "wrong-symlink-target"
	case OwnershipUnrelatedCollision:
		return "unrelated-collision"
	default:
		return "unknown"
	}
}

// Decide classifies a single path given what the manifest recorded for it
// and what's actually on disk now, under the mode this operation is
// installing (or was installed) under. mode only matters when the path is
// owned and its content diverged: managed-copy divergence is a local
// modification, symlink divergence is a wrong link target.
func Decide(hashes PathHashes, mode InstallMode) Ownership {
	owned := hashes.Installed != ""
	exists := hashes.Current != ""

	switch {
	case !owned && !exists:
		return OwnershipAbsent
	case !owned && exists:
		return OwnershipUnrelatedCollision
	case owned && !exists:
		return OwnershipAbsent
	case hashes.Installed == hashes.Current:
		return OwnershipUnchanged
	case mode == ModeSymlink:
		return OwnershipWrongSymlinkTarget
	default:
		return OwnershipModified
	}
}

// Evaluate classifies every path mentioned in installedHashes or
// currentHashes - the union of what the manifest owns and what exists on
// disk - under mode.
func Evaluate(installedHashes, currentHashes map[string]string, mode InstallMode) map[string]Ownership {
	result := make(map[string]Ownership, len(installedHashes)+len(currentHashes))
	for path := range installedHashes {
		if _, done := result[path]; done {
			continue
		}
		result[path] = Decide(PathHashes{Installed: installedHashes[path], Current: currentHashes[path]}, mode)
	}
	for path := range currentHashes {
		if _, done := result[path]; done {
			continue
		}
		result[path] = Decide(PathHashes{Installed: installedHashes[path], Current: currentHashes[path]}, mode)
	}
	return result
}

// ForcePolicy authorizes overwriting or removing specific paths that would
// otherwise be refused because they're locally modified, unrelated, or
// point at the wrong symlink target. Each path must be named explicitly, so
// force can't turn into a broad deletion.
type ForcePolicy struct {
	paths map[string]bool
}

// NewForcePolicy builds a ForcePolicy that authorizes exactly the given
// paths.
func NewForcePolicy(paths ...string) ForcePolicy {
	m := make(map[string]bool, len(paths))
	for _, p := range paths {
		m[p] = true
	}
	return ForcePolicy{paths: m}
}

// Allows reports whether force authorizes writing to path.
func (f ForcePolicy) Allows(path string) bool {
	return f.paths[path]
}

// AllowWrite reports whether it's safe to write to path given its ownership
// classification and the operation's force policy. OwnershipAbsent and
// OwnershipUnchanged are always safe; every other classification requires
// the force policy to explicitly authorize the path.
func AllowWrite(ownership Ownership, path string, force ForcePolicy) bool {
	switch ownership {
	case OwnershipAbsent, OwnershipUnchanged:
		return true
	default:
		return force.Allows(path)
	}
}
