package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func DeltaAvailable() bool {
	_, err := exec.LookPath("delta")
	return err == nil
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
