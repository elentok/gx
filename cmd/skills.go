package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

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
	cmd := &cobra.Command{
		Use:   "install",
		Short: "install gx's canonical skill bundle for Claude and Codex",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSkillsInstall(d, force, c.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil,
		"bundle-relative path(s) to force-overwrite despite a detected conflict")
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
	roots, err := d.skillsAgentRoots()
	if err != nil {
		return err
	}
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "gx-skills-bundle-*")
	if err != nil {
		return fmt.Errorf("create scratch dir for bundle extraction: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	sources, err := skills.ExtractBundle(tmpDir)
	if err != nil {
		return err
	}

	req := skills.InstallRequest{
		Source:     skills.BundleSource,
		AgentRoots: roots,
		Files:      sources,
		Force:      skills.NewForcePolicy(force...),
	}

	plan, err := skills.Plan(manifestPath, req)
	if err != nil {
		return err
	}

	installErr := skills.Install(manifestPath, req)
	printTargetReport(out, plan, installErr == nil)

	var conflictErr *skills.ConflictError
	if errors.As(installErr, &conflictErr) {
		return fmt.Errorf("%w; rerun with --force <path> for each conflicted path to override", installErr)
	}
	return installErr
}

// runSkillsUninstall removes gx's canonical skill bundle's manifest-owned
// files, reporting each target as removed or conflicted (preserved because
// it's locally modified and force didn't authorize its removal). Uninstall
// never aborts on conflict - see skills.Uninstall - so the plan computed here
// always matches what actually happened.
func runSkillsUninstall(d deps, force []string, out io.Writer) error {
	manifestPath, err := d.skillsManifestPath()
	if err != nil {
		return err
	}
	forcePolicy := skills.NewForcePolicy(force...)

	plan, err := skills.PlanUninstall(manifestPath, forcePolicy)
	if errors.Is(err, skills.ErrNotExist) {
		fmt.Fprintln(out, "gx's skill bundle is not installed")
		return nil
	}
	if err != nil {
		return err
	}

	if err := skills.Uninstall(manifestPath, forcePolicy); err != nil {
		return err
	}
	printTargetReport(out, plan, true)
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
