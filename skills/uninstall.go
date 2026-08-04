package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// Uninstall removes a skill's manifest-owned files from every agent root
// recorded in the manifest at manifestPath, then removes the manifest
// itself.
//
// A managed file whose on-disk content is locally modified is preserved
// rather than removed unless force explicitly authorizes its path; any
// preserved files are recorded in a rewritten manifest, so a later Install
// or Uninstall still recognizes them as owned. Uninstall never touches
// paths it doesn't own, and it only removes the manifest once every file
// it owns has either been removed or explicitly preserved - a failure
// midway leaves the manifest describing the pre-uninstall state rather
// than claiming an incomplete uninstall succeeded.
func Uninstall(manifestPath string, force ForcePolicy) error {
	m, err := Load(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	remaining := make([]ManagedFile, 0, len(m.Files))
	for _, f := range m.Files {
		preserved := false
		for _, root := range m.AgentRoots {
			target := filepath.Join(root, f.Path)
			currentIdentity, err := identityIfExists(target, m.Mode)
			if err != nil {
				return fmt.Errorf("inspect %s: %w", target, err)
			}
			ownership := Decide(PathHashes{Installed: managedIdentity(f), Current: currentIdentity}, m.Mode)
			if ownership == OwnershipAbsent {
				continue
			}
			if !AllowWrite(ownership, f.Path, force) {
				preserved = true
				continue
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", target, err)
			}
		}
		if preserved {
			remaining = append(remaining, f)
		}
	}

	if len(remaining) == 0 {
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove manifest: %w", err)
		}
		return nil
	}

	m.Files = remaining
	if err := Save(manifestPath, m); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	return nil
}
