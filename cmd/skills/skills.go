// Package skills is `gx skills install|uninstall`, plus Refresh for
// `gx server start`.
package skills

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/skills"

	"github.com/spf13/cobra"
)

// Deps is the runtime context of `gx skills`.
type Deps struct {
	cmdbase.Env
	AgentRoots   func() ([]string, error)
	ManifestPath func() (string, error)
	// ManifestDir is the folder holding one manifest per Skill source.
	ManifestDir func() (string, error)
	// StatusPath is where `gx server start` records a skill install problem
	// for the server to show.
	StatusPath func() (string, error)
	// BackupDir is optional; nil installs without a backup directory.
	BackupDir func() (string, error)
	// CodexHome is $CODEX_HOME; nil skips the duplicate-skill warning.
	CodexHome func() string
	// BundleSkills names the skills gx's bundle owns; nil reads the embedded
	// bundle. Tests inject it because the embed cannot change.
	BundleSkills func() ([]string, error)
}

// DefaultDeps wires the production skill locations for the given env and
// $CODEX_HOME. Env is left for the caller to fill in.
func DefaultDeps(dirs config.Dirs, codexHome string) Deps {
	return Deps{
		AgentRoots:   func() ([]string, error) { return defaultAgentRoots(dirs) },
		ManifestPath: func() (string, error) { return defaultManifestPath(dirs) },
		ManifestDir: func() (string, error) {
			base, err := dirs.UserConfigDir()
			if err != nil {
				return "", fmt.Errorf("resolve user config dir: %w", err)
			}
			return filepath.Dir(skills.ManifestPath(base, skills.BundleID)), nil
		},
		StatusPath: func() (string, error) { return defaultStatusPath(dirs) },
		BackupDir: func() (string, error) {
			base, err := dirs.DataDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(base, "skills-backups"), nil
		},
		CodexHome: func() string { return codexHome },
	}
}

// defaultAgentRoots resolves gx's canonical skill bundle's two install
// targets under the real user's home directory: Claude Code's user skill
// discovery root and Codex's user skill discovery root.
func defaultAgentRoots(dirs config.Dirs) ([]string, error) {
	home, err := dirs.Home()
	if err != nil {
		return nil, fmt.Errorf("resolve user home dir: %w", err)
	}
	return []string{skills.ClaudeSkillsRoot(home), skills.CodexSkillsRoot(home)}, nil
}

func defaultManifestPath(dirs config.Dirs) (string, error) {
	base, err := dirs.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return skills.ManifestPath(base, skills.BundleID), nil
}

func defaultStatusPath(dirs config.Dirs) (string, error) {
	stateDir, err := dirs.StateDir()
	if err != nil {
		return "", err
	}
	return skills.StatusPath(stateDir), nil
}

// New builds the `gx skills` command.
func New(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "install or remove gx's built-in agent skill bundle",
	}
	cmd.AddCommand(newSkillsInstallCmd(d), newSkillsUninstallCmd(d))
	return cmd
}

func newSkillsInstallCmd(d Deps) *cobra.Command {
	var force []string
	var forceAll bool
	var dev, copyFiles bool
	cmd := &cobra.Command{
		Use:   "install [<dir>]",
		Short: "install gx's skill bundle, or the skills of a source folder <dir>, for Claude and Codex",
		Long: `Without <dir>, install gx's canonical skill bundle as managed copies.

With <dir>, make it a skill source: each direct subfolder holding a SKILL.md
is linked into both agent roots (or copied with --copy).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			policy := skills.NewForcePolicy(force...)
			if forceAll {
				policy = skills.ForceAll()
			}
			install := runSkillsInstall
			if dev {
				install = runSkillsInstallDev
			}
			if len(args) == 1 {
				if dev {
					return errors.New("--dev cannot be used with a source folder")
				}
				install = func(d Deps, p skills.ForcePolicy, out io.Writer) error {
					return runSkillsInstallSource(d, args[0], copyFiles, p, out)
				}
			} else if copyFiles {
				return errors.New("--copy needs a source folder <dir>")
			}
			if len(args) == 0 && !dev {
				return runSkillsInstallAll(d, policy, c.OutOrStdout())
			}
			return install(d, policy, c.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&force, "force", nil,
		"bundle-relative path(s) to force-overwrite despite a detected conflict")
	cmd.Flags().BoolVar(&forceAll, "force-all", false,
		"force-overwrite every conflicting path")
	cmd.Flags().BoolVar(&dev, "dev", false,
		"symlink the current checkout's skill directories; link shared files separately (not with <dir>)")
	cmd.Flags().BoolVar(&copyFiles, "copy", false,
		"with <dir>: install managed copies of each skill instead of links")
	return cmd
}

func newSkillsUninstallCmd(d Deps) *cobra.Command {
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

// runSkillsInstallAll is a bare install: the bundle, then every remembered
// source. It records the joined problem the way Refresh does (cleared when the
// run is clean) and fails when any part had a problem.
func runSkillsInstallAll(d Deps, force skills.ForcePolicy, out io.Writer) error {
	var problems []string
	if err := runSkillsInstall(d, force, out); err != nil {
		problems = append(problems, err.Error())
	}
	problems = append(problems, syncSources(d, force, out)...)
	problem := joinProblems(problems)
	if path, err := d.StatusPath(); err == nil {
		_ = skills.WriteProblem(path, problem)
	}
	if problem == "" {
		return nil
	}
	return errors.New(strings.Join(dedupe(problems), "\n"))
}

// runSkillsInstall extracts gx's embedded canonical skill bundle to a
// scratch directory, then installs it as managed copies into every agent
// root - reporting, per target, whether it was installed, updated, skipped
// (already up to date), or conflicted. A conflict aborts the entire install
// (see skills.Install), so every non-conflicted target is reported skipped
// rather than the status it would have received on a clean run.
func runSkillsInstall(d Deps, force skills.ForcePolicy, out io.Writer) error {
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
func runSkillsInstallDev(d Deps, force skills.ForcePolicy, out io.Writer) error {
	cwd, err := d.Getwd()
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

	if err := installSkills(d, force, out, skills.DevSourcePrefix+devRoot, sources, skills.ModeSymlink); err != nil {
		return err
	}
	// Sources holding a name the checkout owns drop it now, not at the next
	// bare install.
	if problems := dedupe(syncSources(d, force, out)); len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// bundleOwner returns the skill names gx's bundle owns and who installed them:
// the dev checkout in a dev install, else the built-in bundle. The dev
// checkout carries the same names as the embed.
func bundleOwner(d Deps) (map[string]bool, string, error) {
	namesOf := skills.BundleSkillNames
	if d.BundleSkills != nil {
		namesOf = d.BundleSkills
	}
	names, err := namesOf()
	if err != nil {
		return nil, "", err
	}
	owned := map[string]bool{}
	for _, name := range names {
		owned[name] = true
	}
	owner := "gx's skill bundle"
	if manifestPath, err := d.ManifestPath(); err == nil {
		if m, err := skills.Load(manifestPath); err == nil {
			if root, ok := skills.DevRoot(m); ok {
				owner = "the dev checkout " + root
			}
		}
	}
	return owned, owner, nil
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
func installSkills(d Deps, force skills.ForcePolicy, out io.Writer, source string, sources []skills.SourceFile, mode skills.InstallMode) error {
	targets, installErr := installSkillTargets(d, force, source, sources, mode)
	printTargetReport(out, targets, installErr == nil)
	if installErr == nil {
		warnCodexHomeDuplicates(d, out, sources)
	}

	var conflictErr *skills.ConflictError
	if errors.As(installErr, &conflictErr) {
		return fmt.Errorf("%w; rerun with --force <path> for each conflicted path, or --force-all to override every conflict", installErr)
	}
	return installErr
}

// installSkillTargets runs a single skill install and returns its per-target
// report without printing it.
func installSkillTargets(d Deps, force skills.ForcePolicy, source string, sources []skills.SourceFile, mode skills.InstallMode) ([]skills.Target, error) {
	manifestPath, err := d.ManifestPath()
	if err != nil {
		return nil, err
	}
	return installTargetsAt(d, manifestPath, force, source, sources, mode, false, false)
}

// installTargetsAt is installSkillTargets for an explicit manifest path.
func installTargetsAt(d Deps, manifestPath string, force skills.ForcePolicy, source string, sources []skills.SourceFile, mode skills.InstallMode, linkedMarker, switchMode bool) ([]skills.Target, error) {
	roots, err := d.AgentRoots()
	if err != nil {
		return nil, err
	}
	backupDir := ""
	if d.BackupDir != nil {
		backupDir, err = d.BackupDir()
		if err != nil {
			return nil, err
		}
	}
	return skills.Install(manifestPath, skills.InstallRequest{
		ProtectedSources: protectedSourceDirs(d, source),
		BackupDir:        backupDir,
		Source:           source,
		AgentRoots:       roots,
		Files:            sources,
		Force:            force,
		Mode:             mode,
		SwitchMode:       switchMode,

		AllowLinkedMarker: linkedMarker,
	})
}

// protectedSourceDirs lists every remembered Skill source folder plus the one
// being installed, which no agent root may resolve into.
func protectedSourceDirs(d Deps, source string) []string {
	var dirs []string
	if real, ok := skills.ThirdPartyRoot(skills.Manifest{Source: source}); ok {
		dirs = append(dirs, real)
	}
	if d.ManifestDir == nil {
		return dirs
	}
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return dirs
	}
	paths, _ := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	for _, path := range paths {
		if m, err := skills.Load(path); err == nil {
			if real, ok := skills.ThirdPartyRoot(m); ok {
				dirs = append(dirs, real)
			}
		}
	}
	return dirs
}

// runSkillsInstallSource installs the skills of the Skill source dir: one
// directory link per skill into every root, or managed copies with copyFiles.
func runSkillsInstallSource(d Deps, dir string, copyFiles bool, force skills.ForcePolicy, out io.Writer) error {
	if d.ManifestDir == nil {
		return errors.New("skill sources are not configured")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("skill source %s: %w", dir, err)
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return fmt.Errorf("skill source %s is not a folder", dir)
	}
	// Link targets stay inside the path the user gave, not the resolved one.
	names, err := skills.ScanSource(abs)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintf(out, "no skills found in %s\n", dir)
		return nil
	}
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(manifestDir, skills.SourceID(real)+".json")
	syncErr := syncSourceDir(d, manifestPath, abs, real, names, copyFiles, true, force, out)
	var clash *clashError
	if syncErr != nil && !errors.As(syncErr, &clash) {
		return syncErr
	}
	// A source remembered earlier still holds the clashing names; resync it so
	// it drops them too, whatever order the sources were installed in.
	problems := dedupe(clashProblems(syncErr))
	for _, other := range clashingSources(d, real, names) {
		if p := syncSource(d, other.manifestPath, other.manifest, other.real, force, out); p != "" {
			problems = dedupe(append(problems, p))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// clashError carries the name clashes of a source install that otherwise
// succeeded: the clashing names were skipped, everything else installed.
type clashError struct{ problems []string }

func (e *clashError) Error() string { return strings.Join(e.problems, "; ") }

func clashProblems(err error) []string {
	var clash *clashError
	if errors.As(err, &clash) {
		return clash.problems
	}
	return nil
}

// dedupe drops repeated problems, keeping first-seen order. Both sides of a
// clash report the same line.
func dedupe(problems []string) []string {
	seen := map[string]bool{}
	var kept []string
	for _, p := range problems {
		if p != "" && !seen[p] {
			seen[p] = true
			kept = append(kept, p)
		}
	}
	return kept
}

// rememberedSource is another remembered Skill source and the skill names it
// currently has.
type rememberedSource struct {
	manifestPath string
	manifest     skills.Manifest
	real, abs    string
	names        []string
}

// otherSources lists every remembered Skill source except the one at real that
// still exists and can be scanned.
func otherSources(d Deps, real string) []rememberedSource {
	if d.ManifestDir == nil {
		return nil
	}
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	var others []rememberedSource
	for _, path := range paths {
		m, err := skills.Load(path)
		if err != nil {
			continue
		}
		other, ok := skills.ThirdPartyRoot(m)
		if !ok || other == real {
			continue
		}
		if info, err := os.Stat(other); err != nil || !info.IsDir() {
			continue
		}
		abs := recordedAbs(m, other)
		names, err := skills.ScanSource(abs)
		if err != nil {
			continue
		}
		others = append(others, rememberedSource{path, m, other, abs, names})
	}
	return others
}

// recordedAbs recovers the folder path the person gave for a source, which may
// differ from its resolved path, from the links gx recorded.
func recordedAbs(m skills.Manifest, real string) string {
	for _, f := range m.Files {
		if f.Kind == skills.KindDirectoryLink && f.LinkTarget != "" {
			return filepath.Dir(f.LinkTarget)
		}
	}
	return real
}

// sourceClashes returns the name clashes between the source at real, with the
// given names, and every other remembered source: name -> sorted real paths.
func sourceClashes(d Deps, real string, names []string) (map[string][]string, []rememberedSource) {
	others := otherSources(d, real)
	all := map[string][]string{real: names}
	for _, o := range others {
		all[o.real] = o.names
	}
	return skills.SourceClashes(all), others
}

// clashingSources lists the other remembered sources that share a skill name
// with the source at real.
func clashingSources(d Deps, real string, names []string) []rememberedSource {
	clashes, others := sourceClashes(d, real, names)
	var out []rememberedSource
	for _, o := range others {
		for _, name := range o.names {
			if slices.Contains(clashes[name], real) {
				out = append(out, o)
				break
			}
		}
	}
	return out
}

// syncSourceDir installs the named skills of the source abs (real is its
// resolved path) into every root, minus the names another source also has: a
// clash is skipped in both sources and returned as a clashError after the rest
// installed. A source that lost all its skills passes no names, which removes
// its links. Only an explicit install passes switchMode: a re-sync keeps the
// recorded mode.
func syncSourceDir(d Deps, manifestPath, abs, real string, names []string, copyFiles, switchMode bool, force skills.ForcePolicy, out io.Writer) error {
	dir := abs
	clashes, _ := sourceClashes(d, real, names)
	owned, owner, err := bundleOwner(d)
	if err != nil {
		return err
	}
	var problems []string
	kept := names[:0:0]
	for _, name := range names {
		if owned[name] {
			problems = append(problems, fmt.Sprintf("skill name %q is owned by %s and by source %s; the source's copy was skipped, delete the old copy from %s", name, owner, real, real))
			continue
		}
		holders := clashes[name]
		if len(holders) == 0 {
			kept = append(kept, name)
			continue
		}
		problems = append(problems, fmt.Sprintf("skill name %q is shared by %s, skipped in every one", name, strings.Join(holders, " and ")))
	}
	names = kept
	files, err := skills.SourceSkillFiles(abs, names, copyFiles)
	if err != nil {
		return err
	}
	mode := skills.ModeSymlink
	if copyFiles {
		mode = skills.ModeManagedCopy
	}
	targets, installErr := installTargetsAt(d, manifestPath, force, skills.ThirdPartySourcePrefix+real, files, mode, true, switchMode)
	printTargetReport(out, targets, installErr == nil)
	var conflictErr *skills.ConflictError
	if errors.As(installErr, &conflictErr) {
		return fmt.Errorf("%w; rerun with --force <path> for each conflicted path, or --force-all to override every conflict", installErr)
	}
	if installErr != nil {
		return installErr
	}
	fmt.Fprintf(out, "%d skills from %s: %s\n", len(names), dir, summarizeTargets(targets))
	if len(problems) > 0 {
		return &clashError{problems}
	}
	return nil
}

// Refresh reinstalls gx's skill bundle the way it was last installed:
// dev symlinks from the same checkout, else managed copies (also when it was
// never installed). It never forces and never fails its caller: a problem is
// printed and recorded for the server to show, and a clean install clears
// the record.
func Refresh(d Deps, out io.Writer) {
	problems := append([]string{refreshProblem(d, out)}, syncSources(d, skills.NewForcePolicy(), out)...)
	problem := joinProblems(problems)
	if problem != "" {
		fmt.Fprintf(out, "warning: skills: %s\n", problem)
	}
	path, err := d.StatusPath()
	if err == nil {
		err = skills.WriteProblem(path, problem)
	}
	if err != nil {
		fmt.Fprintf(out, "warning: record skill install result: %v\n", err)
	}
}

func joinProblems(problems []string) string {
	return strings.Join(dedupe(problems), "; ")
}

// syncSources re-syncs every remembered Skill source in its recorded mode and
// returns one problem per source that failed or is gone. Each source stands
// alone: a problem never stops the next one.
func syncSources(d Deps, force skills.ForcePolicy, out io.Writer) []string {
	if d.ManifestDir == nil {
		return nil
	}
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return []string{err.Error()}
	}
	paths, _ := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	var problems []string
	for _, path := range paths {
		m, err := skills.Load(path)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		real, ok := skills.ThirdPartyRoot(m)
		if !ok {
			continue
		}
		if p := syncSource(d, path, m, real, force, out); p != "" {
			problems = append(problems, p)
		}
	}
	return problems
}

func syncSource(d Deps, manifestPath string, m skills.Manifest, real string, force skills.ForcePolicy, out io.Writer) string {
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return fmt.Sprintf("skill source %s is gone, skills left as they are; run `gx skills uninstall %s`", real, real)
	}
	// Links point inside the folder path the person gave, which may differ from
	// its resolved path; recover it from the recorded links.
	abs := recordedAbs(m, real)
	names, err := skills.ScanSource(abs)
	if err != nil {
		return fmt.Sprintf("skill source %s: %v", real, err)
	}
	copyFiles := m.Mode == skills.ModeManagedCopy
	if err := syncSourceDir(d, manifestPath, abs, real, names, copyFiles, false, force, out); err != nil {
		var clash *clashError
		if errors.As(err, &clash) {
			return strings.Join(clash.problems, "; ")
		}
		return fmt.Sprintf("skill source %s: %v", real, err)
	}
	return ""
}

func refreshProblem(d Deps, out io.Writer) string {
	manifestPath, err := d.ManifestPath()
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
	warnCodexHomeDuplicates(d, out, sources)
	return ""
}

// warnCodexHomeDuplicates prints a warning for each gx skill that also sits
// under $CODEX_HOME/skills. It is informational only: it never fails the
// install, never deletes, and is not recorded as a server problem.
func warnCodexHomeDuplicates(d Deps, out io.Writer, sources []skills.SourceFile) {
	if d.CodexHome == nil {
		return
	}
	for _, path := range skills.CodexHomeDuplicates(d.CodexHome(), sources) {
		fmt.Fprintf(out, "warning: skills: %s duplicates a gx skill installed under ~/.agents/skills; Codex would find the skill twice\n", path)
	}
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
func runSkillsUninstall(d Deps, force []string, out io.Writer) error {
	manifestPath, err := d.ManifestPath()
	if err != nil {
		return err
	}
	forcePolicy := skills.NewForcePolicy(force...)

	backupDir := ""
	if d.BackupDir != nil {
		backupDir, err = d.BackupDir()
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
