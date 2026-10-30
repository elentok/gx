package skills

import "os"

// UninstallOptions selects the durable recovery location for legacy installs.
type UninstallOptions struct{ BackupDir string }

// Uninstall removes owned entries without traversing directory links. Conflicted
// entries remain recorded, so a later operation can still recognize ownership.
func Uninstall(manifestPath string, force ForcePolicy, options ...UninstallOptions) ([]Target, error) {
	unlock, err := lockInstallation(manifestPath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := buildUninstallPlan(manifestPath, force)
	if err != nil {
		return nil, err
	}
	if len(options) > 0 && options[0].BackupDir != "" {
		p.backupDir = options[0].BackupDir
		p.manifest.BackupDir = p.backupDir
	}
	backup, err := executePlan(p, manifestPath, func() error {
		if len(p.manifest.Files) > 0 {
			return Save(manifestPath, p.manifest)
		}
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
	for i := range p.targets {
		p.targets[i].BackupDir = backup
	}
	return p.targets, err
}
