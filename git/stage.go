package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// StageFileStatus represents one file entry from git status porcelain output.
type StageFileStatus struct {
	Path         string
	RenameFrom   string
	IndexStatus  byte
	WorktreeCode byte
}

func (s StageFileStatus) IsUntracked() bool {
	return s.IndexStatus == '?' && s.WorktreeCode == '?'
}

func (s StageFileStatus) HasStagedChanges() bool {
	return s.IndexStatus != ' ' && s.IndexStatus != '?'
}

func (s StageFileStatus) HasUnstagedChanges() bool {
	if s.IsUntracked() {
		return true
	}
	return s.WorktreeCode != ' ' && s.WorktreeCode != '?'
}

func (s StageFileStatus) IsRenamed() bool {
	return s.IndexStatus == 'R' || s.WorktreeCode == 'R'
}

// XY returns the two-character porcelain status code.
func (s StageFileStatus) XY() string {
	return string([]byte{s.IndexStatus, s.WorktreeCode})
}

// WorktreeRoot resolves dir to the root of the current non-bare worktree.
func WorktreeRoot(dir string) (string, error) {
	out := runAllowFail(dir, []string{"rev-parse", "--show-toplevel"})
	if out == "" {
		return "", fmt.Errorf("not inside a worktree: %s", dir)
	}
	return out, nil
}

// ListStageFiles returns status entries suitable for an interactive staging UI.
func ListStageFiles(worktreeRoot string) ([]StageFileStatus, error) {
	out, _, err := runNoOptionalLocks(worktreeRoot, []string{"status", "--porcelain=v1", "--untracked-files=all", "-z"})
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	parts := strings.Split(out, "\x00")
	items := make([]StageFileStatus, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		tok := parts[i]
		if tok == "" || len(tok) < 3 {
			continue
		}
		if tok[2] != ' ' {
			continue
		}

		entry := StageFileStatus{
			IndexStatus:  tok[0],
			WorktreeCode: tok[1],
			Path:         tok[3:],
		}

		// In porcelain v1 -z format, renames/copies include an extra NUL path.
		if entry.IndexStatus == 'R' || entry.IndexStatus == 'C' || entry.WorktreeCode == 'R' || entry.WorktreeCode == 'C' {
			if i+1 < len(parts) && parts[i+1] != "" {
				entry.RenameFrom = parts[i+1]
				i++
			}
		}

		items = append(items, entry)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items, nil
}

// UnstagePath removes path changes from the index while preserving worktree edits.
func UnstagePath(worktreeRoot, path string) error {
	_, _, err := run(worktreeRoot, []string{"reset", "-q", "HEAD", "--", path})
	return err
}

// DiffPath returns a unified diff for path. When cached is true, it reads index
// vs HEAD. Output is plain (no ANSI colors).
func DiffPath(worktreeRoot, path string, cached bool, contextLines int) (string, error) {
	args := []string{"diff", "--no-color", diffContextArg(contextLines)}
	if cached {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	out, _, err := run(worktreeRoot, args)
	if err != nil {
		return "", err
	}
	return out, nil
}

// DiffPathWithDelta returns a unified diff rendered by git diff with delta as
// pager. If delta is unavailable, it falls back to git's own color output.
func DiffPathWithDelta(worktreeRoot, path string, cached bool, sideBySide bool, renderWidth int, contextLines int) (string, error) {
	rawArgs := []string{"diff", "--no-color", diffContextArg(contextLines)}
	if cached {
		rawArgs = append(rawArgs, "--cached")
	}
	rawArgs = append(rawArgs, "--", path)
	raw, _, err := run(worktreeRoot, rawArgs)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if out, deltaErr := colorizeWithDelta(worktreeRoot, raw, sideBySide, renderWidth); deltaErr == nil {
		return out, nil
	}

	fallbackArgs := []string{"diff", "--color=always", diffContextArg(contextLines)}
	if cached {
		fallbackArgs = append(fallbackArgs, "--cached")
	}
	fallbackArgs = append(fallbackArgs, "--", path)
	fallbackOut, _, fallbackErr := run(worktreeRoot, fallbackArgs)
	if fallbackErr != nil {
		return "", fallbackErr
	}
	return fallbackOut, nil
}

// DiffUntrackedPath returns a /dev/null -> file patch for an untracked path.
// Plain output is returned when color is false; otherwise output is rendered by
// git diff with delta as pager where possible.
func DiffUntrackedPath(worktreeRoot, path string, color bool, sideBySide bool, renderWidth int, contextLines int) (string, error) {
	diffPath := path

	if !color {
		return runGitAllowExitCodes(worktreeRoot, nil, map[int]bool{0: true, 1: true}, "diff", "--no-index", "--no-color", diffContextArg(contextLines), "--", "/dev/null", diffPath)
	}

	raw, err := runGitAllowExitCodes(worktreeRoot, nil, map[int]bool{0: true, 1: true},
		"diff",
		"--no-index",
		"--no-color",
		diffContextArg(contextLines),
		"--",
		"/dev/null",
		diffPath,
	)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if out, deltaErr := colorizeWithDelta(worktreeRoot, raw, sideBySide, renderWidth); deltaErr == nil {
		return out, nil
	}

	return runGitAllowExitCodes(worktreeRoot, nil, map[int]bool{0: true, 1: true},
		"diff",
		"--no-index",
		"--color=always",
		diffContextArg(contextLines),
		"--",
		"/dev/null",
		diffPath,
	)
}

// BinaryFileSizes returns previous and new file sizes for binary diff summaries.
// prevSize is read from HEAD (or rename source when available).
// newSize is read from the worktree path, falling back to the index blob.
func BinaryFileSizes(worktreeRoot string, file StageFileStatus) (prevSize, newSize int64, prevOK, newOK bool) {
	prevPath := file.Path
	if file.RenameFrom != "" {
		prevPath = file.RenameFrom
	}
	prevSize, prevOK = gitObjectSize(worktreeRoot, "HEAD:"+prevPath)

	if info, err := os.Stat(filepath.Join(worktreeRoot, filepath.FromSlash(file.Path))); err == nil && info.Mode().IsRegular() {
		newSize = info.Size()
		newOK = true
	}
	if !newOK {
		newSize, newOK = gitObjectSize(worktreeRoot, ":"+file.Path)
	}

	return prevSize, newSize, prevOK, newOK
}

func gitObjectSize(worktreeRoot, object string) (int64, bool) {
	out := strings.TrimSpace(runAllowFail(worktreeRoot, []string{"cat-file", "-s", object}))
	if out == "" {
		return 0, false
	}
	size, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, false
	}
	return size, true
}

func diffContextArg(contextLines int) string {
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 20 {
		contextLines = 20
	}
	return fmt.Sprintf("-U%d", contextLines)
}

func colorizeWithDelta(worktreeRoot, raw string, sideBySide bool, renderWidth int) (string, error) {
	args := []string{"--paging=never"}
	if !sideBySide {
		args = append(args, "--color-only")
	}
	if sideBySide {
		args = append(args, "--side-by-side")
		if renderWidth > 0 {
			args = append(args, "--width", strconv.Itoa(renderWidth))
		}
	}
	configPath, cleanup, err := tempDeltaConfig(worktreeRoot)
	if err == nil && configPath != "" {
		defer cleanup()
		args = append(args, "--config", configPath)
	} else {
		args = append(args, "--no-gitconfig")
	}

	cmd := exec.Command("delta", args...)
	cmd.Stdin = strings.NewReader(raw)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(errBuf.String())
		if stderr == "" {
			stderr = err.Error()
		}
		return "", fmt.Errorf("delta: %s", stderr)
	}
	return strings.TrimRight(outBuf.String(), "\r\n"), nil
}

type deltaConfigCacheEntry struct {
	once sync.Once
	path string
	err  error
}

var deltaConfigCache sync.Map

func tempDeltaConfig(worktreeRoot string) (path string, cleanup func(), err error) {
	cacheKey := worktreeRoot
	if cacheKey == "" {
		cacheKey = "."
	}
	entryAny, _ := deltaConfigCache.LoadOrStore(cacheKey, &deltaConfigCacheEntry{})
	entry := entryAny.(*deltaConfigCacheEntry)
	entry.once.Do(func() {
		entry.path, entry.err = createTempDeltaConfig(worktreeRoot)
	})
	return entry.path, func() {}, entry.err
}

func createTempDeltaConfig(worktreeRoot string) (path string, err error) {
	includes := make([]string, 0, 2)

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		userConfig := filepath.Join(home, ".gitconfig")
		if _, statErr := os.Stat(userConfig); statErr == nil {
			includes = append(includes, userConfig)
		}
	}

	gitDir := runAllowFail(worktreeRoot, []string{"rev-parse", "--git-dir"})
	if gitDir != "" {
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(worktreeRoot, gitDir)
		}
		repoConfig := filepath.Join(gitDir, "config")
		if _, statErr := os.Stat(repoConfig); statErr == nil {
			includes = append(includes, repoConfig)
		}
	}

	if len(includes) == 0 {
		return "", fmt.Errorf("no git config found for delta")
	}

	f, err := os.CreateTemp("", "gx-delta-*.gitconfig")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, cfg := range includes {
		b.WriteString("[include]\n\tpath = ")
		b.WriteString(cfg)
		b.WriteString("\n")
	}
	b.WriteString("[delta]\n\tside-by-side = false\n\thunk-header-decoration-style = ol\n")
	content := b.String()
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// ApplyPatchToIndex applies patch to the index in worktreeRoot.
func ApplyPatchToIndex(worktreeRoot, patch string, reverse bool, unidiffZero bool) error {
	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if reverse {
		args = append(args, "-R")
	}
	if unidiffZero {
		args = append(args, "--unidiff-zero")
	}
	_, err := runGitAllowExitCodes(worktreeRoot, []byte(patch), map[int]bool{0: true}, args...)
	if err == nil {
		return nil
	}
	if runErr, ok := err.(*RunError); ok {
		if strings.TrimSpace(runErr.Stdout) == "" && strings.TrimSpace(runErr.Stderr) == "" {
			return fmt.Errorf("%w\npatch:\n%s", err, patch)
		}
	}
	return err
}

// ApplyPatchToWorktree applies patch to the worktree in worktreeRoot.
func ApplyPatchToWorktree(worktreeRoot, patch string, reverse bool, unidiffZero bool) error {
	args := []string{"apply", "--whitespace=nowarn"}
	if reverse {
		args = append(args, "-R")
	}
	if unidiffZero {
		args = append(args, "--unidiff-zero")
	}
	_, err := runGitAllowExitCodes(worktreeRoot, []byte(patch), map[int]bool{0: true}, args...)
	if err == nil {
		return nil
	}
	if runErr, ok := err.(*RunError); ok {
		if strings.TrimSpace(runErr.Stdout) == "" && strings.TrimSpace(runErr.Stderr) == "" {
			return fmt.Errorf("%w\npatch:\n%s", err, patch)
		}
	}
	return err
}

// RestorePaths restores the given paths in both index and worktree from HEAD.
func RestorePaths(worktreeRoot string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := []string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}
	args = append(args, paths...)
	_, _, err := run(worktreeRoot, args)
	return err
}

// DiscardUntrackedPath removes an untracked path from the working tree.
func DiscardUntrackedPath(worktreeRoot, relPath string) error {
	root := filepath.Clean(worktreeRoot)
	target := filepath.Clean(filepath.Join(root, relPath))
	if target == root {
		return fmt.Errorf("refusing to remove worktree root")
	}
	prefix := root + string(filepath.Separator)
	if !strings.HasPrefix(target, prefix) {
		return fmt.Errorf("path escapes worktree root: %s", relPath)
	}
	return os.RemoveAll(target)
}

// StagePath stages a full path (used for untracked files).
func StagePath(worktreeRoot, path string) error {
	_, _, err := run(worktreeRoot, []string{"add", "--", path})
	return err
}

// StageIntentPath records an intent-to-add entry without adding content.
func StageIntentPath(worktreeRoot, path string) error {
	_, _, err := run(worktreeRoot, []string{"add", "-N", "--", path})
	return err
}

func runGitAllowExitCodes(dir string, stdin []byte, allowed map[int]bool, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	out := strings.TrimRight(outBuf.String(), "\r\n")
	if err == nil {
		return out, nil
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if allowed[exitErr.ExitCode()] {
		return out, nil
	}
	stderr := strings.TrimSpace(strings.TrimRight(errBuf.String(), "\r\n"))
	if stderr == "" {
		stderr = strings.TrimSpace(out)
	}
	return out, &RunError{
		Args:   args,
		Dir:    dir,
		Stdout: strings.TrimSpace(out),
		Stderr: stderr,
		Code:   exitErr.ExitCode(),
	}
}
