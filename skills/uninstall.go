package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// Uninstall removes a skill's manifest-owned files from every agent root
// recorded in the manifest at manifestPath, then removes the manifest
// itself. It returns the same per-target classification PlanUninstall would
// have produced - classified once, via classifyUninstall, and reused for
// both the removals and the returned report - so a caller never needs a
// separate PlanUninstall call just to report what Uninstall did.
//
// A managed file whose on-disk content is locally modified is preserved
// rather than removed unless force explicitly authorizes its path; any
// preserved files are recorded in a rewritten manifest, so a later Install
// or Uninstall still recognizes them as owned. Uninstall never touches
// paths it doesn't own, and it only removes the manifest once every file
// it owns has either been removed or explicitly preserved - a failure
// midway leaves the manifest describing the pre-uninstall state rather
// than claiming an incomplete uninstall succeeded.
func Uninstall(manifestPath string, force ForcePolicy) ([]Target, error) {
	targets, m, err := classifyUninstall(manifestPath, force)
	if err != nil {
		return nil, err
	}

	preservedPaths := map[string]bool{}
	for _, t := range targets {
		if t.Status != StatusConflicted {
			continue
		}
		preservedPaths[t.Path] = true
	}

	remaining := make([]ManagedFile, 0, len(m.Files))
	for _, f := range m.Files {
		if preservedPaths[f.Path] {
			remaining = append(remaining, f)
			continue
		}
	}

	for _, t := range targets {
		if t.Status != StatusRemoved {
			continue
		}
		target := filepath.Join(t.Root, t.Path)
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return targets, fmt.Errorf("remove %s: %w", target, err)
		}
	}

	if len(remaining) == 0 {
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return targets, fmt.Errorf("remove manifest: %w", err)
		}
		return targets, nil
	}

	m.Files = remaining
	if err := Save(manifestPath, m); err != nil {
		return targets, fmt.Errorf("save manifest: %w", err)
	}
	return targets, nil
}
