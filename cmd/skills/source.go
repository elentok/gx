package skills

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/skills"
)

// sourceManifest is one manifest in the manifest folder. err is set when it
// could not be loaded; manifests of gx's own bundle or dev install are left out.
type sourceManifest struct {
	path     string
	manifest skills.Manifest
	real     string
	err      error
}

// loadSources reads every Skill source manifest. Callers decide whether a
// manifest that failed to load is skipped or reported.
func loadSources(d Deps) ([]sourceManifest, error) {
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return nil, err
	}
	paths, _ := filepath.Glob(filepath.Join(manifestDir, "*.json"))
	var sources []sourceManifest
	for _, path := range paths {
		m, err := skills.Load(path)
		if err != nil {
			sources = append(sources, sourceManifest{path: path, err: err})
			continue
		}
		if real, ok := skills.ThirdPartyRoot(m); ok {
			sources = append(sources, sourceManifest{path: path, manifest: m, real: real})
		}
	}
	return sources, nil
}

// protectedSourceDirs lists every remembered Skill source folder plus the one
// being installed, which no agent root may resolve into.
func protectedSourceDirs(d Deps, source string) []string {
	var dirs []string
	if real, ok := skills.ThirdPartyRoot(skills.Manifest{Source: source}); ok {
		dirs = append(dirs, real)
	}
	sources, _ := loadSources(d)
	for _, s := range sources {
		if s.err == nil {
			dirs = append(dirs, s.real)
		}
	}
	return dirs
}

// rememberedSource is a remembered Skill source and the skill names it
// currently has.
type rememberedSource struct {
	manifestPath string
	manifest     skills.Manifest
	real         string
	names        []string
}

// rememberedSources scans every loaded source that still exists as a folder.
func rememberedSources(sources []sourceManifest) []rememberedSource {
	var remembered []rememberedSource
	for _, s := range sources {
		if s.err != nil {
			continue
		}
		if info, err := os.Stat(s.real); err != nil || !info.IsDir() {
			continue
		}
		names, err := skills.ScanSource(recordedAbs(s.manifest, s.real))
		if err != nil {
			continue
		}
		remembered = append(remembered, rememberedSource{s.path, s.manifest, s.real, names})
	}
	return remembered
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
func sourceClashes(remembered []rememberedSource, real string, names []string) map[string][]string {
	all := map[string][]string{}
	for _, o := range remembered {
		all[o.real] = o.names
	}
	all[real] = names
	return skills.SourceClashes(all)
}

// clashingSources lists the other remembered sources that share a skill name
// with the source at real.
func clashingSources(remembered []rememberedSource, real string, names []string) []rememberedSource {
	clashes := sourceClashes(remembered, real, names)
	var out []rememberedSource
	for _, o := range remembered {
		if o.real == real {
			continue
		}
		for _, name := range o.names {
			if slices.Contains(clashes[name], real) {
				out = append(out, o)
				break
			}
		}
	}
	return out
}

// runSkillsInstallSource installs the skills of the Skill source dir: one
// directory link per skill into every root, or managed copies with copyFiles.
func runSkillsInstallSource(d Deps, dir string, copyFiles bool, force skills.ForcePolicy, out io.Writer) error {
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
	sources, err := loadSources(d)
	if err != nil {
		return err
	}
	remembered := rememberedSources(sources)
	problems, err := syncSourceDir(d, syncOptions{
		manifestPath: filepath.Join(manifestDir, skills.SourceID(real)+".json"),
		abs:          abs,
		real:         real,
		names:        names,
		copyFiles:    copyFiles,
		switchMode:   true,
		force:        force,
		remembered:   remembered,
	}, out)
	if err != nil {
		return err
	}
	// A source remembered earlier still holds the clashing names; resync it so
	// it drops them too, whatever order the sources were installed in.
	problems = dedupe(problems)
	// The resync of the others must see this source too, which may be new.
	remembered = append(remembered, rememberedSource{real: real, names: names})
	for _, other := range clashingSources(remembered, real, names) {
		if p := syncSource(d, remembered, other.manifestPath, other.manifest, other.real, force, out); p != "" {
			problems = dedupe(append(problems, p))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// syncOptions describes one source folder to install into the agent roots.
type syncOptions struct {
	manifestPath string
	abs          string // the folder path the person gave
	real         string // its resolved path
	names        []string
	copyFiles    bool
	// switchMode is set only by an explicit install: a re-sync keeps the
	// recorded mode.
	switchMode bool
	force      skills.ForcePolicy
	remembered []rememberedSource
}

// syncSourceDir installs the named skills of a source into every root, minus
// the names another source or the bundle also has: a clash is skipped in both
// sources and returned as a problem after the rest installed. A source that
// lost all its skills passes no names, which removes its links.
func syncSourceDir(d Deps, o syncOptions, out io.Writer) ([]string, error) {
	clashes := sourceClashes(o.remembered, o.real, o.names)
	owned, owner, err := bundleOwner(d)
	if err != nil {
		return nil, err
	}
	var problems []string
	var kept []string
	for _, name := range o.names {
		if owned[name] {
			problems = append(problems, fmt.Sprintf("skill name %q is owned by %s and by source %s; the source's copy was skipped, delete the old copy from %s", name, owner, o.real, o.real))
			continue
		}
		holders := clashes[name]
		if len(holders) == 0 {
			kept = append(kept, name)
			continue
		}
		problems = append(problems, fmt.Sprintf("skill name %q is shared by %s, skipped in every one", name, strings.Join(holders, " and ")))
	}
	files, err := skills.SourceSkillFiles(o.abs, kept, o.copyFiles)
	if err != nil {
		return nil, err
	}
	mode := skills.ModeSymlink
	if o.copyFiles {
		mode = skills.ModeManagedCopy
	}
	_, statErr := os.Stat(o.manifestPath)
	firstInstall := os.IsNotExist(statErr)
	targets, installErr := installTargetsAt(d, installOptions{
		manifestPath: o.manifestPath,
		source:       skills.ThirdPartySourcePrefix + o.real,
		files:        files,
		mode:         mode,
		force:        o.force,
		linkedMarker: true,
		switchMode:   o.switchMode,
	})
	printTargetReport(out, targets, installErr == nil)
	if firstInstall && installErr == nil {
		if err := removeDanglingLinks(d, o.abs); err != nil {
			return nil, err
		}
	}
	if installErr != nil {
		return nil, withForceHint(installErr)
	}
	fmt.Fprintf(out, "%d skills from %s: %s\n", len(kept), o.abs, summarizeTargets(targets))
	warnCodexHomeDuplicates(d, out, files)
	return problems, nil
}

// removeDanglingLinks clears leftover links into a source for skills deleted
// from it, in every agent root.
func removeDanglingLinks(d Deps, source string) error {
	roots, err := d.AgentRoots()
	if err != nil {
		return err
	}
	for _, root := range roots {
		if _, err := skills.RemoveDanglingLinksInto(root, source); err != nil {
			return err
		}
	}
	return nil
}

// syncSources re-syncs every remembered Skill source in its recorded mode and
// returns one problem per source that failed or is gone. Each source stands
// alone: a problem never stops the next one.
func syncSources(d Deps, force skills.ForcePolicy, out io.Writer) []string {
	sources, err := loadSources(d)
	if err != nil {
		return []string{err.Error()}
	}
	remembered := rememberedSources(sources)
	var problems []string
	for _, s := range sources {
		if s.err != nil {
			problems = append(problems, s.err.Error())
			continue
		}
		if p := syncSource(d, remembered, s.path, s.manifest, s.real, force, out); p != "" {
			problems = append(problems, p)
		}
	}
	return problems
}

func syncSource(d Deps, remembered []rememberedSource, manifestPath string, m skills.Manifest, real string, force skills.ForcePolicy, out io.Writer) string {
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
	problems, err := syncSourceDir(d, syncOptions{
		manifestPath: manifestPath,
		abs:          abs,
		real:         real,
		names:        names,
		copyFiles:    m.Mode == skills.ModeManagedCopy,
		force:        force,
		remembered:   remembered,
	}, out)
	if err != nil {
		return fmt.Sprintf("skill source %s: %v", real, err)
	}
	return strings.Join(problems, "; ")
}

// runSkillsUninstallSource removes one Skill source's entries and record. The
// record is keyed by the folder's real path; a folder that is gone cannot be
// resolved, so its parent is resolved instead, then the path as typed.
func runSkillsUninstallSource(d Deps, dir string, force []string, out io.Writer) error {
	manifestDir, err := d.ManifestDir()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	candidates := []string{abs}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		candidates = []string{real, abs}
	} else if parent, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		candidates = []string{filepath.Join(parent, filepath.Base(abs)), abs}
	}
	manifestPath := filepath.Join(manifestDir, skills.SourceID(candidates[0])+".json")
	for _, c := range candidates {
		p := filepath.Join(manifestDir, skills.SourceID(c)+".json")
		if _, err := os.Stat(p); err == nil {
			manifestPath = p
			break
		}
	}
	return uninstallManifest(d, manifestPath, "skill source "+dir, force, out)
}
