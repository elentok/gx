// Package claudehistory enumerates Claude Code's on-disk session history
// (~/.claude/projects) for the history browser: project directories and,
// within each, the conversations recorded there.
package claudehistory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Project represents a Claude Code project directory.
type Project struct {
	// Dir is the on-disk directory name under the projects root.
	Dir string
	// Cwd is the real working directory path identified from transcripts.
	Cwd string
	// Label is the display name (basename of Cwd, or worktree label).
	Label string
	// Subtitle is the dimmed secondary line (Cwd with $HOME collapsed to ~).
	Subtitle string
	// NewestMtime is the mtime of the most recently modified .jsonl in Dir.
	NewestMtime time.Time
}

// ListProjects enumerates projects under root, sorted by newest .jsonl mtime
// (most recent first). Root defaults to ~/.claude/projects when empty.
func ListProjects(root string) ([]Project, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".claude", "projects")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	home, _ := os.UserHomeDir()

	var projects []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		p, ok := buildProject(dir, e.Name(), home)
		if !ok {
			continue
		}
		projects = append(projects, p)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].NewestMtime.After(projects[j].NewestMtime)
	})

	return projects, nil
}

func buildProject(dir, dirName, home string) (Project, bool) {
	mtime, jsonls := newestJSONL(dir)
	if len(jsonls) == 0 {
		return Project{}, false
	}

	cwd := cwdFromTranscripts(jsonls)
	if cwd == "" {
		cwd = dedashDir(dirName)
	}

	label := buildLabel(cwd)
	subtitle := collapseTilde(cwd, home)

	return Project{
		Dir:         dir,
		Cwd:         cwd,
		Label:       label,
		Subtitle:    subtitle,
		NewestMtime: mtime,
	}, true
}

// newestJSONL returns the newest .jsonl mtime and all .jsonl paths in dir.
func newestJSONL(dir string) (time.Time, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, nil
	}
	var newest time.Time
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest, paths
}

// cwdFromTranscripts reads the first transcript line carrying a "cwd" field.
func cwdFromTranscripts(paths []string) string {
	for _, p := range paths {
		if cwd := cwdFromFile(p); cwd != "" {
			return cwd
		}
	}
	return ""
}

func cwdFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		var rec struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Cwd != "" {
			return rec.Cwd
		}
	}
	return ""
}

// buildLabel returns the display label for a project cwd.
// Worktree paths (containing /.claude/worktrees/ or /worktrees/) get a
// "parent » worktree" label; others get basename(cwd).
func buildLabel(cwd string) string {
	for _, marker := range []string{"/.claude/worktrees/", "/worktrees/"} {
		if before, after, ok := strings.Cut(cwd, marker); ok {
			parent := filepath.Base(before)
			worktree := strings.SplitN(after, "/", 2)[0]
			return parent + " » " + worktree
		}
	}
	return filepath.Base(cwd)
}

// dedashDir converts a dash-encoded directory name to a best-effort path.
// Claude encodes both / and . as - so this is lossy — used only as fallback.
func dedashDir(name string) string {
	if name == "" {
		return name
	}
	// Replace leading dash with /
	if name[0] == '-' {
		name = "/" + name[1:]
	}
	return strings.ReplaceAll(name, "-", "/")
}

// collapseTilde replaces the user's home directory prefix with ~.
func collapseTilde(path, home string) string {
	if home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
