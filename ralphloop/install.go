package ralphloop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// lockfileManagers maps each tracked lockfile name to the package manager that
// owns it. bun.lock and bun.lockb are one manager.
var lockfileManagers = map[string]string{
	"package-lock.json": "npm",
	"pnpm-lock.yaml":    "pnpm",
	"yarn.lock":         "yarn",
	"bun.lock":          "bun",
	"bun.lockb":         "bun",
}

// installCommand is the non-interactive, lockfile-respecting install for
// manager. It never imports, generates or pins anything. yarnBerry picks
// between Yarn's two incompatible CLIs. ok is false for a manager gx has no
// command for.
func installCommand(manager string, yarnBerry bool) (command []string, ok bool) {
	switch manager {
	case "npm":
		return []string{"npm", "ci"}, true
	case "pnpm":
		return []string{"pnpm", "install", "--frozen-lockfile"}, true
	case "yarn":
		if yarnBerry {
			return []string{"yarn", "install", "--immutable"}, true
		}
		return []string{"yarn", "install", "--frozen-lockfile"}, true
	case "bun":
		return []string{"bun", "install", "--frozen-lockfile"}, true
	}
	return nil, false
}

// installDependenciesWith installs dependencies in every directory of the
// worktree at path that holds a git-tracked lockfile. pathEnv, when set,
// replaces the process PATH for finding the package manager. No tracked
// lockfile is a no-op. go.mod is deliberately not a trigger: go build/go test
// populate the module cache lazily, so a separate download step buys nothing.
//
// All installs of one call share installTimeout. A failed install writes its
// full output to a log file in logDir (outside the worktree, so it survives the
// worktree's discard); "" skips the log.
func installDependenciesWith(path, pathEnv, logDir string) (commands []string, err error) {
	return installDependenciesWithin(path, pathEnv, logDir, installTimeout)
}

func installDependenciesWithin(path, pathEnv, logDir string, budget time.Duration) (commands []string, err error) {
	dirs, err := lockfileDirs(path)
	if err != nil {
		return nil, installError(path, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	for _, dir := range dirs {
		command, err := installIn(ctx, dir.path, dir.lockfiles, pathEnv, logDir)
		if err != nil {
			return nil, installError(dir.path, err)
		}
		commands = append(commands, dir.path+": "+strings.Join(command, " "))
	}
	return commands, nil
}

func installError(dir string, err error) error {
	return fmt.Errorf("installing dependencies in %s: %w", dir, err)
}

type lockfileDir struct {
	path      string
	lockfiles []string // base names, sorted
}

// lockfileDirs lists the directories of the worktree at path that hold a
// tracked lockfile, sorted by path.
func lockfileDirs(path string) ([]lockfileDir, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = path
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	byDir := map[string][]string{}
	for _, tracked := range strings.Split(string(out), "\x00") {
		name := filepath.Base(tracked)
		if _, ok := lockfileManagers[name]; !ok {
			continue
		}
		dir := filepath.Join(path, filepath.Dir(tracked))
		byDir[dir] = append(byDir[dir], name)
	}
	dirs := make([]lockfileDir, 0, len(byDir))
	for dir, names := range byDir {
		sort.Strings(names)
		dirs = append(dirs, lockfileDir{path: dir, lockfiles: names})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].path < dirs[j].path })
	return dirs, nil
}

// installIn chooses dir's manager and runs its install there until ctx ends.
func installIn(ctx context.Context, dir string, lockfiles []string, pathEnv, logDir string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &pkg)
	}
	if err != nil {
		return nil, fmt.Errorf("reading package.json: %w", err)
	}

	command, err := chooseCommand(dir, lockfiles, pkg.PackageManager)
	if err != nil {
		return nil, err
	}
	name := command[0]
	if pathEnv != "" {
		if name, err = lookPathIn(name, pathEnv); err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(command, " "), err)
		}
	}
	cmd := exec.CommandContext(ctx, name, command[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=true")
	// A killed manager's children can keep the output pipe open; don't wait on them.
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err == nil {
		return command, nil
	}
	joined := strings.Join(command, " ")
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s: timed out after %dm", joined, int(installTimeout.Minutes()))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, fmt.Errorf("%s: %w", joined, err)
	}
	reason := fmt.Sprintf("%s: %s", joined, outputTail(out))
	if logPath := writeInstallLog(logDir, dir, out); logPath != "" {
		reason += " (full output: " + logPath + ")"
	}
	return nil, errors.New(reason)
}

// installTimeout is the budget all installs of one InstallDeps call share.
const installTimeout = 5 * time.Minute

// installTailBytes caps how much of a failed install's output reaches the error.
const installTailBytes = 2000

// outputTail is the last installTailBytes of out, starting on a line boundary
// when it had to cut.
func outputTail(out []byte) string {
	out = bytes.TrimSpace(out)
	if len(out) <= installTailBytes {
		return string(out)
	}
	out = out[len(out)-installTailBytes:]
	if i := bytes.IndexByte(out, '\n'); i >= 0 {
		out = out[i+1:]
	}
	return "..." + "\n" + string(out)
}

// writeInstallLog saves out under logDir and returns the file's path, or ""
// when there is no logDir or the write failed (the error text stands alone then).
func writeInstallLog(logDir, installDir string, out []byte) string {
	if logDir == "" || os.MkdirAll(logDir, 0o755) != nil {
		return ""
	}
	name := fmt.Sprintf("install-%s-%d.log", filepath.Base(installDir), time.Now().UnixNano())
	logPath := filepath.Join(logDir, name)
	if os.WriteFile(logPath, out, 0o644) != nil {
		return ""
	}
	return logPath
}

// chooseCommand picks the install command: packageManager when set, else the
// directory's lockfiles.
func chooseCommand(dir string, lockfiles []string, packageManager string) ([]string, error) {
	if packageManager != "" {
		name, version, found := strings.Cut(packageManager, "@")
		unsupported := fmt.Errorf("unsupported packageManager %q", packageManager)
		if !found || version == "" {
			return nil, unsupported
		}
		major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
		if err != nil {
			return nil, unsupported
		}
		command, ok := installCommand(name, major >= 2)
		if !ok {
			return nil, unsupported
		}
		return command, nil
	}

	var managers []string
	for _, lockfile := range lockfiles {
		if m := lockfileManagers[lockfile]; !slices.Contains(managers, m) {
			managers = append(managers, m)
		}
	}
	if len(managers) > 1 {
		return nil, fmt.Errorf("lockfiles of different package managers (%s); set packageManager in package.json",
			strings.Join(lockfiles, ", "))
	}
	yarnBerry := false
	if managers[0] == "yarn" {
		lock, err := os.ReadFile(filepath.Join(dir, "yarn.lock"))
		if err != nil {
			return nil, fmt.Errorf("reading yarn.lock: %w", err)
		}
		yarnBerry = bytes.Contains(lock, []byte("__metadata:"))
	}
	command, _ := installCommand(managers[0], yarnBerry)
	return command, nil
}
