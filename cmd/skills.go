package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/skills"

	"github.com/spf13/cobra"
)

func newSkillsCmd(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "install or remove gx's built-in agent skill bundle",
	}
	cmd.AddCommand(newSkillsInstallCmd(d), newSkillsUninstallCmd(d))
	return cmd
}

func newSkillsInstallCmd(d deps) *cobra.Command {
	var force []string
	var forceAll bool
	var dev bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install gx's canonical skill bundle for Claude and Codex",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			policy := skills.NewForcePolicy(force...)
			if forceAll {
				policy = skills.ForceAll()
			}
			install := runSkillsInstall
			if dev {
				install = runSkillsInstallDev
			}
			if err := install(d, policy, c.OutOrStdout()); err != nil {
				return err
			}
			clearSkillsProblem(d)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil,
		"bundle-relative path(s) to force-overwrite despite a detected conflict")
	cmd.Flags().BoolVar(&forceAll, "force-all", false,
		"force-overwrite every conflicting path")
	cmd.Flags().BoolVar(&dev, "dev", false,
		"symlink the current checkout's skill directories; link shared files separately")
	return cmd
}

func newSkillsUninstallCmd(d deps) *cobra.Command {
	var force []string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "remove gx's canonical skill bundle for Claude and Codex",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSkillsUninstall(d, force, c.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil,
		"bundle-relative path(s) to force-remove despite local modification")
	return cmd
}

// clearSkillsProblem drops the problem the last `gx server start` recorded,
// so the web UI stops showing it once a manual install succeeds.
func clearSkillsProblem(d deps) {
	if path, err := d.skillsStatusPath(); err == nil {
		_ = skills.WriteProblem(path, "")
	}
}

// runSkillsInstall extracts gx's embedded canonical skill bundle to a
// scratch directory, then installs it as managed copies into every agent
// root - reporting, per target, whether it was installed, updated, skipped
// (already up to date), or conflicted. A conflict aborts the entire install
// (see skills.Install), so every non-conflicted target is reported skipped
// rather than the status it would have received on a clean run.
func runSkillsInstall(d deps, force skills.ForcePolicy, out io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "gx-skills-bundle-*")
	if err != nil {
		return fmt.Errorf("create scratch dir for bundle extraction: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	sources, err := skills.ExtractBundle(tmpDir)
	if err != nil {
		return err
	}

	return installSkills(d, force, out, skills.BundleSource, sources, skills.ModeManagedCopy)
}

// runSkillsInstallDev symlinks the skill bundle out of a gx source checkout
// instead of installing the embedded copy, so a contributor's edits under
// skills/** show up to both agents immediately. The checkout is resolved
// from the git repository containing the invocation working directory, not
// from the running binary's location, so it works the same from a nested
// subdirectory or a linked worktree; runSkillsInstallDev refuses to change
// anything if that repository doesn't contain gx's skill bundle.
func runSkillsInstallDev(d deps, force skills.ForcePolicy, out io.Writer) error {
	cwd, err := d.getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	devRoot, err := resolveDevRoot(cwd)
	if err != nil {
		return err
	}
	sources, err := skills.DevSourceFiles(devRoot)
	if err != nil {
		return err
	}

	return installSkills(d, force, out, skills.DevSourcePrefix+devRoot, sources, skills.ModeSymlink)
}

// resolveDevRoot returns the root of the git repository (or, for a linked
// worktree, the specific checkout) containing dir - the directory whose
// skills/ subdirectory gx dev mode should link from.
func resolveDevRoot(dir string) (string, error) {
	info, err := git.IdentifyDir(dir)
	if err != nil {
		return "", fmt.Errorf("resolve git repo containing %s: %w", dir, err)
	}
	if info.WorktreeRoot != "" {
		return info.WorktreeRoot, nil
	}
	return info.Repo.Root, nil
}

// installSkills runs a single skill install, reporting per-target results
// and turning a conflict into an actionable error message.
func installSkills(d deps, force skills.ForcePolicy, out io.Writer, source string, sources []skills.SourceFile, mode skills.InstallMode) error {
	targets, installErr := installSkillTargets(d, force, source, sources, mode)
	printTargetReport(out, targets, installErr == nil)

	var conflictErr *skills.ConflictError
	if errors.As(installErr, &conflictErr) {
		return fmt.Errorf("%w; rerun with --force <path> for each conflicted path, or --force-all to override every conflict", installErr)
	}
	return installErr
}

// installSkillTargets runs a single skill install and returns its per-target
// report without printing it.
func installSkillTargets(d deps, force skills.ForcePolicy, source string, sources []skills.SourceFile, mode skills.InstallMode) ([]skills.Target, error) {
	roots, err := d.skillsAgentRoots()
	if err != nil {
		return nil, err
	}
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return nil, err
	}
	backupDir := ""
	if d.skillsBackupDir != nil {
		backupDir, err = d.skillsBackupDir()
		if err != nil {
			return nil, err
		}
	}
	return skills.Install(manifestPath, skills.InstallRequest{
		BackupDir:  backupDir,
		Source:     source,
		AgentRoots: roots,
		Files:      sources,
		Force:      force,
		Mode:       mode,
	})
}

// refreshSkills reinstalls gx's skill bundle the way it was last installed:
// dev symlinks from the same checkout, else managed copies (also when it was
// never installed). It never forces and never fails its caller: a problem is
// printed and recorded for the server to show, and a clean install clears
// the record.
func refreshSkills(d deps, out io.Writer) {
	problem := refreshSkillsProblem(d, out)
	if problem != "" {
		fmt.Fprintf(out, "warning: skills: %s\n", problem)
	}
	path, err := d.skillsStatusPath()
	if err == nil {
		err = skills.WriteProblem(path, problem)
	}
	if err != nil {
		fmt.Fprintf(out, "warning: record skill install result: %v\n", err)
	}
}

func refreshSkillsProblem(d deps, out io.Writer) string {
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return err.Error()
	}
	m, err := skills.Load(manifestPath)
	if err != nil && !errors.Is(err, skills.ErrNotExist) {
		return err.Error()
	}

	fix := "run `gx skills install --force <path>` or `--force-all`"
	source, mode := skills.BundleSource, skills.ModeManagedCopy
	var sources []skills.SourceFile
	if root, ok := skills.DevRoot(m); ok {
		if _, err := os.Stat(root); err != nil {
			return fmt.Sprintf("dev checkout %s is gone, skills left as they are; run `gx skills install`, or `gx skills install --dev` from a checkout", root)
		}
		if sources, err = skills.DevSourceFiles(root); err != nil {
			return err.Error()
		}
		fix = "run `gx skills install --dev --force <path>` or `--force-all` from " + root
		source, mode = skills.DevSourcePrefix+root, skills.ModeSymlink
	} else {
		tmpDir, err := os.MkdirTemp("", "gx-skills-bundle-*")
		if err != nil {
			return fmt.Sprintf("create scratch dir for bundle extraction: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		if sources, err = skills.ExtractBundle(tmpDir); err != nil {
			return err.Error()
		}
	}

	targets, err := installSkillTargets(d, skills.NewForcePolicy(), source, sources, mode)
	if err != nil {
		return installProblem(targets, err, fix)
	}
	printSkillsBackup(out, targets)
	fmt.Fprintf(out, "skills: %s\n", summarizeTargets(targets))
	return ""
}

// installProblem describes a failed install, naming its conflicted paths and
// fix, the hint that resolves them.
func installProblem(targets []skills.Target, err error, fix string) string {
	var conflictErr *skills.ConflictError
	if !errors.As(err, &conflictErr) {
		return fmt.Sprintf("%v; %s", err, fix)
	}
	var paths []string
	for _, t := range targets {
		if t.Status == skills.StatusConflicted {
			paths = append(paths, t.RootRelPath())
		}
	}
	return fmt.Sprintf("install refused, %d path(s) have local changes or content gx did not install: %s; %s",
		len(paths), strings.Join(paths, ", "), fix)
}

// summarizeTargets counts a clean install's targets by status, e.g.
// "2 installed, 1 updated", or "up to date" when nothing changed.
func summarizeTargets(targets []skills.Target) string {
	counts := map[skills.TargetStatus]int{}
	for _, t := range targets {
		counts[t.Status]++
	}
	var parts []string
	for _, s := range []skills.TargetStatus{skills.StatusInstalled, skills.StatusUpdated} {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return "up to date"
	}
	return strings.Join(parts, ", ")
}

// runSkillsUninstall removes gx's canonical skill bundle's manifest-owned
// files, reporting each target as removed or conflicted (preserved because
// it's locally modified and force didn't authorize its removal). Uninstall
// never aborts on conflict, so the targets it returns always match what
// actually happened.
func runSkillsUninstall(d deps, force []string, out io.Writer) error {
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return err
	}
	forcePolicy := skills.NewForcePolicy(force...)

	backupDir := ""
	if d.skillsBackupDir != nil {
		backupDir, err = d.skillsBackupDir()
		if err != nil {
			return err
		}
	}
	targets, err := skills.Uninstall(manifestPath, forcePolicy, skills.UninstallOptions{BackupDir: backupDir})
	if errors.Is(err, skills.ErrNotExist) {
		fmt.Fprintln(out, "gx's skill bundle is not installed")
		return nil
	}
	if err != nil {
		return err
	}
	printTargetReport(out, targets, true)
	return nil
}

// printTargetReport prints one line per target. When committed is false, an
// install was aborted by a conflict (skills.Install writes nothing at all in
// that case), so every non-conflicted target is downgraded to "skipped" to
// match what the filesystem actually shows.
func printTargetReport(out io.Writer, targets []skills.Target, committed bool) {
	printSkillsBackup(out, targets)
	for _, t := range targets {
		status := t.Status
		if !committed && status != skills.StatusConflicted {
			status = skills.StatusSkipped
		}
		fmt.Fprintf(out, "%-10s %s\n", status, t.RootRelPath())
	}
}

func printSkillsBackup(out io.Writer, targets []skills.Target) {
	for _, t := range targets {
		if t.BackupDir != "" {
			fmt.Fprintf(out, "skills backups: %s\n", t.BackupDir)
			return
		}
	}
}
