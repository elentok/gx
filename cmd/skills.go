package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

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
	var dev bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install gx's canonical skill bundle for Claude and Codex",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if dev {
				return runSkillsInstallDev(d, force, c.OutOrStdout())
			}
			return runSkillsInstall(d, force, c.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil,
		"bundle-relative path(s) to force-overwrite despite a detected conflict")
	cmd.Flags().BoolVar(&dev, "dev", false,
		"symlink the current checkout's skill files instead of installing embedded copies")
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

// runSkillsInstall extracts gx's embedded canonical skill bundle to a
// scratch directory, then installs it as managed copies into every agent
// root - reporting, per target, whether it was installed, updated, skipped
// (already up to date), or conflicted. A conflict aborts the entire install
// (see skills.Install), so every non-conflicted target is reported skipped
// rather than the status it would have received on a clean run.
func runSkillsInstall(d deps, force []string, out io.Writer) error {
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
func runSkillsInstallDev(d deps, force []string, out io.Writer) error {
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

	return installSkills(d, force, out, "gx dev checkout: "+devRoot, sources, skills.ModeSymlink)
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
func installSkills(d deps, force []string, out io.Writer, source string, sources []skills.SourceFile, mode skills.InstallMode) error {
	roots, err := d.skillsAgentRoots()
	if err != nil {
		return err
	}
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return err
	}

	req := skills.InstallRequest{
		Source:     source,
		AgentRoots: roots,
		Files:      sources,
		Force:      skills.NewForcePolicy(force...),
		Mode:       mode,
	}

	targets, installErr := skills.Install(manifestPath, req)
	printTargetReport(out, targets, installErr == nil)

	var conflictErr *skills.ConflictError
	if errors.As(installErr, &conflictErr) {
		return fmt.Errorf("%w; rerun with --force <path> for each conflicted path to override", installErr)
	}
	return installErr
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

	targets, err := skills.Uninstall(manifestPath, forcePolicy)
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
	for _, t := range targets {
		status := t.Status
		if !committed && status != skills.StatusConflicted {
			status = skills.StatusSkipped
		}
		fmt.Fprintf(out, "%-10s %s\n", status, t.RootRelPath())
	}
}
