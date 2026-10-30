package skills

import (
	"path/filepath"
	"sort"
)

func buildUninstallPlan(manifestPath string, force ForcePolicy) (*installationPlan, error) {
	m, err := Load(manifestPath)
	if err != nil {
		return nil, err
	}
	old, err := ownedPaths(m)
	if err != nil {
		return nil, err
	}
	p := &installationPlan{manifest: m, backupDir: m.BackupDir}
	if p.backupDir == "" {
		p.backupDir = filepath.Join(filepath.Dir(manifestPath), "backups")
	}
	preserved := map[string]bool{}
	keys := make([]string, 0, len(old))
	for path := range old {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		f := old[path]
		root := filepath.Clean(path[:len(path)-len(f.Path)])
		// Old manifests with files underneath a directory link do not own that
		// directory. Refuse traversal even with force rather than infer ownership.
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
		if !AllowWrite(owner, f.Path, force) {
			status = StatusConflicted
			preserved[f.Path] = true
		}
		p.targets = append(p.targets, Target{Root: root, Path: f.Path, Ownership: owner, Status: status})
		if status == StatusRemoved {
			if err := protectRecordedSources(path, m); err != nil {
				return nil, err
			}
			p.operations = append(p.operations, operation{path: path, remove: true})
		}
	}
	p.manifest.Files = nil
	for _, f := range m.Files {
		if preserved[f.Path] {
			p.manifest.Files = append(p.manifest.Files, f)
		}
	}
	displayTargets(p, m.AgentRoots)
	return p, nil
}
