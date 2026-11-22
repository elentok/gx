package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
)

// ErrNoProject means no project in the ticket store points at the repo.
var ErrNoProject = errors.New("no ticket-store project for this repo; run `gx project add .` to register it")

// ProjectDir finds the project directory in the ticket store whose
// project.json names repoRoot as its repo.
func ProjectDir(storePath, repoRoot string) (string, error) {
	want := canonicalPath(repoRoot)
	found := ""
	err := forEachProject(storePath, func(dir string, pf config.ProjectFile) bool {
		if pf.Repo != nil && canonicalPath(*pf.Repo) == want {
			found = dir
			return true
		}
		return false
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", ErrNoProject
	}
	return found, nil
}

// ProjectDirByName finds the registered project whose address name is name
// (see ProjectName). An unregistered name is refused with CodeUnknownProject.
func ProjectDirByName(storePath, name string) (string, error) {
	found := ""
	err := forEachProject(storePath, func(dir string, _ config.ProjectFile) bool {
		if ProjectName(dir) == name {
			found = dir
			return true
		}
		return false
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", &AddressError{Code: CodeUnknownProject, Msg: fmt.Sprintf("project %q is not a registered project", name)}
	}
	return found, nil
}

// ProjectDirs lists every project directory in the ticket store: each
// subdirectory holding a project.json. A missing store has no projects.
func ProjectDirs(storePath string) ([]string, error) {
	var dirs []string
	err := forEachProject(storePath, func(dir string, _ config.ProjectFile) bool {
		dirs = append(dirs, dir)
		return false
	})
	return dirs, err
}

// forEachProject calls fn for each subdirectory of storePath holding a
// project.json, stopping early when fn returns true. A missing store has no
// projects.
func forEachProject(storePath string, fn func(dir string, pf config.ProjectFile) (stop bool)) error {
	return forEachProjectRead(storePath, config.ReadProjectFile, fn)
}

func forEachProjectRead(storePath string, read func(dir string) (config.ProjectFile, error), fn func(dir string, pf config.ProjectFile) (stop bool)) error {
	if storePath == "" {
		return errors.New("ticket-store.path is not set")
	}
	entries, err := os.ReadDir(storePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read ticket store %s: %w", storePath, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(storePath, e.Name())
		pf, err := read(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if fn(dir, pf) {
			return nil
		}
	}
	return nil
}

// RootIn resolves dir's repo to its project directory in the ticket store at
// storePath.
func RootIn(storePath, dir string) (string, error) {
	repo, err := git.FindRepo(dir)
	if err != nil {
		return "", fmt.Errorf("not inside a git repo: %w", err)
	}
	return ProjectDir(storePath, repo.Root)
}

// RootForDirs is RootIn with the store path read from the config dirs holds.
func RootForDirs(dirs config.Dirs, dir string) (string, error) {
	cfg, err := dirs.Load()
	if err != nil {
		return "", err
	}
	return RootIn(cfg.TicketStore.Path, dir)
}

// ResolveEpicTarget splits arg ("project:epic" or a bare epic) into its
// project directory, project name and epic name. A bare epic name belongs to
// the cwd's project; an unknown project refuses with CodeUnknownProject.
func ResolveEpicTarget(cfgDirs config.Dirs, cwd, arg string) (projectDir, project, name string, err error) {
	project, name, hasProject := strings.Cut(arg, ":")
	if !hasProject {
		name = arg
		if projectDir, err = RootForDirs(cfgDirs, cwd); err != nil {
			return "", "", "", err
		}
		return projectDir, ProjectName(projectDir), name, nil
	}
	cfg, err := cfgDirs.Load()
	if err != nil {
		return "", "", "", err
	}
	dirs, err := ProjectDirs(cfg.TicketStore.Path)
	if err != nil {
		return "", "", "", err
	}
	for _, dir := range dirs {
		if ProjectName(dir) == project {
			return dir, project, name, nil
		}
	}
	return "", "", "", &AddressError{Code: CodeUnknownProject, Msg: fmt.Sprintf("no project %q", project)}
}

// ProjectName is the name addresses carry for projectDir: project.json's
// name, falling back to the directory name.
func ProjectName(projectDir string) string {
	if pf, err := config.ReadProjectFile(projectDir); err == nil && pf.Name != nil && *pf.Name != "" {
		return *pf.Name
	}
	return filepath.Base(projectDir)
}

// canonicalPath makes equal directories compare equal across symlinks (e.g.
// macOS /var vs /private/var) and trailing slashes.
func canonicalPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}
