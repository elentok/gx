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

// installOwnership classifies a single install target given what the
// manifest previously recorded for it (prevIdentity), what's on disk now
// (currentIdentity), and what this install is about to write
// (sourceIdentity).
//
// Under ModeManagedCopy it defers to Decide: safe to overwrite as long as
// disk content matches what was last installed there, regardless of
// whether the new source content differs (that's just a version upgrade).
//
// Under ModeSymlink the safety question is different: relinking a path to
// a *different* target is a materially riskier action than upgrading
// bytes, so the check compares disk against the *new* source target rather
// than the prior manifest record. A path already linked at the intended
// target is unchanged and always safe (idempotent re-run from the same
// checkout); linked anywhere else - including where a prior install of
// this same path left it, e.g. a different checkout's copy of this skill -
// requires force. That's what stops a symlink install run from a second
// checkout from silently retargeting links a first checkout created.
func installOwnership(prevIdentity, currentIdentity, sourceIdentity string, mode InstallMode) Ownership {
	if mode != ModeSymlink {
		return Decide(PathHashes{Installed: prevIdentity, Current: currentIdentity}, mode)
	}
	owned := prevIdentity != ""
	exists := currentIdentity != ""
	switch {
	case !exists:
		return OwnershipAbsent
	case currentIdentity == sourceIdentity:
		return OwnershipUnchanged
	case !owned:
		return OwnershipUnrelatedCollision
	default:
		return OwnershipWrongSymlinkTarget
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
// point at the wrong symlink target. Each path must be named explicitly
// unless the policy is built with ForceAll, so a plain force list can't turn
// into a broad deletion by accident.
type ForcePolicy struct {
	paths map[string]bool
	all   bool
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

// ForceAll builds a ForcePolicy that authorizes every path.
func ForceAll() ForcePolicy {
	return ForcePolicy{all: true}
}

// Allows reports whether force authorizes writing to path.
func (f ForcePolicy) Allows(path string) bool {
	return f.all || f.paths[path]
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
