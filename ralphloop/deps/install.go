package deps

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

	"github.com/elentok/gx/git"
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

// installDependenciesWithin installs dependencies in every directory of the
// worktree at path that holds a git-tracked lockfile. pathEnv, when set,
// replaces the process PATH for finding the package manager. No tracked
// lockfile is a no-op. go.mod is deliberately not a trigger: go build/go test
// populate the module cache lazily, so a separate download step buys nothing.
//
// All installs of one call share installTimeout. A failed install writes its
// full output to a log file in logDir (outside the worktree, so it survives the
// worktree's discard); "" skips the log.
//
// budget is the shared timeout, installTimeout outside tests.
func installDependenciesWithin(parent context.Context, path, pathEnv, logDir string, budget time.Duration) (commands []string, err error) {
	dirs, err := lockfileDirs(path)
	if err != nil {
		return nil, installError(path, err)
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	return installAll(ctx, path, dirs, pathEnv, logDir)
}

func installAll(ctx context.Context, path string, dirs []lockfileDir, pathEnv, logDir string) (commands []string, err error) {
	for _, dir := range dirs {
		command, err := installIn(ctx, path, dir.path, dir.lockfiles, pathEnv, logDir)
		if err != nil {
			return nil, installError(dir.path, err)
		}
		commands = append(commands, dir.path+": "+strings.Join(command, " "))
	}
	return commands, nil
}

// installAndSetupWithin is installDependenciesWithin followed by the setup
// shell commands, all inside one budget. Setup runs even when no lockfile is
// tracked, but never after a failed install. onSetupStart (may be nil) is
// called once between the two, and only when setup is non-empty. The commands
// list ends with one "setup: <command>" entry per setup command.
func installAndSetupWithin(parent context.Context, path, pathEnv, logDir string, budget time.Duration, setup []string, onSetupStart func()) ([]string, time.Duration, time.Duration, error) {
	dirs, err := lockfileDirs(path)
	if err != nil {
		return nil, 0, 0, installError(path, err)
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	start := time.Now()
	commands, err := installAll(ctx, path, dirs, pathEnv, logDir)
	installTime := time.Since(start)
	if err != nil || len(setup) == 0 {
		return commands, installTime, 0, err
	}
	if onSetupStart != nil {
		onSetupStart()
	}
	start = time.Now()
	for _, line := range setup {
		if out, err := runSetup(ctx, path, line, pathEnv); err != nil {
			return nil, installTime, 0, fmt.Errorf("running setup in %s: %w", path,
				describeInstallFailure(ctx, path, []string{line}, out, err, logDir))
		}
		commands = append(commands, "setup: "+line)
	}
	return commands, installTime, time.Since(start), nil
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
	out, stderr, err := git.RunRaw(path, git.NonInteractiveEnv(), "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr))
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
func installIn(ctx context.Context, worktree, dir string, lockfiles []string, pathEnv, logDir string) ([]string, error) {
	packageManager, err := readPackageManager(dir)
	if err != nil {
		return nil, err
	}
	command, err := chooseCommand(dir, lockfiles, packageManager)
	if err != nil {
		return nil, err
	}
	release, err := installFlights.acquire(ctx, installKey(ctx, worktree, dir, packageManager, pathEnv))
	if err != nil {
		return nil, describeInstallFailure(ctx, dir, command, nil, err, logDir)
	}
	defer release()
	out, err := runInstall(ctx, dir, command, pathEnv)
	if err != nil {
		return nil, describeInstallFailure(ctx, dir, command, out, err, logDir)
	}
	return command, nil
}

// readPackageManager is the packageManager field of dir's package.json, "" when unset.
func readPackageManager(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &pkg)
	}
	if err != nil {
		return "", fmt.Errorf("reading package.json: %w", err)
	}
	return pkg.PackageManager, nil
}

// runInstall runs command in dir and returns its combined output.
func runInstall(ctx context.Context, dir string, command []string, pathEnv string) ([]byte, error) {
	name := command[0]
	if pathEnv != "" {
		var err error
		if name, err = lookPathIn(name, pathEnv); err != nil {
			return nil, err
		}
	}
	return runNiced(ctx, dir, name, command[1:], nil)
}

// runSetup runs the shell line in dir like an install, with pathEnv (when set)
// as its PATH.
func runSetup(ctx context.Context, dir, line, pathEnv string) ([]byte, error) {
	var env []string
	if pathEnv != "" {
		env = []string{"PATH=" + pathEnv}
	}
	return runNiced(ctx, dir, "/bin/sh", []string{"-c", line}, env)
}

// runNiced runs name at low priority with extraEnv on top of the process
// environment, and returns the combined output.
func runNiced(ctx context.Context, dir, name string, args, extraEnv []string) ([]byte, error) {
	// Two installs at once still burst the CPU; run them at low priority.
	if nicePath, err := exec.LookPath("nice"); err == nil {
		args = append([]string{"-n", "10", name}, args...)
		name = nicePath
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "CI=true"), extraEnv...)
	// A killed manager's children can keep the output pipe open; don't wait on them.
	cmd.WaitDelay = time.Second
	return cmd.CombinedOutput()
}

// describeInstallFailure turns a failed runInstall into the error callers see:
// a timeout, the bare error when the manager printed nothing, or the output's
// tail plus the path of the full log.
func describeInstallFailure(ctx context.Context, dir string, command []string, out []byte, runErr error, logDir string) error {
	joined := strings.Join(command, " ")
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("%s: install cancelled", joined)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%s: timed out after %dm", joined, int(installTimeout.Minutes()))
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return fmt.Errorf("%s: %w", joined, runErr)
	}
	reason := fmt.Sprintf("%s: %s", joined, OutputTail(out))
	if logPath := writeInstallLog(logDir, dir, out); logPath != "" {
		reason += " (full output: " + logPath + ")"
	}
	return errors.New(reason)
}

// installTimeout is the budget all installs of one InstallDeps call share.
const installTimeout = 5 * time.Minute

// maxConcurrentInstalls caps installs server-wide: tickets launching together
// each ran a full install at once and saturated the CPU.
const maxConcurrentInstalls = 2

var installSlots = make(chan struct{}, maxConcurrentInstalls)

// InstallFunc installs the dependencies of the worktree at path, then runs the
// setup shell commands there, one after the other. onSetupStart (may be nil)
// is called once before the first setup command and never when setup is empty.
// commands lists the installs, then one "setup: <command>" entry per setup
// command; the durations are the time spent on each part.
type InstallFunc func(ctx context.Context, path, logDir string, setup []string, onSetupStart func()) (commands []string, installTime, setupTime time.Duration, err error)

// CapInstalls makes install wait for one of the shared install slots. The slot
// is held per call, outside any worktree lock the caller takes.
func CapInstalls(install InstallFunc) InstallFunc {
	return func(ctx context.Context, path, logDir string, setup []string, onSetupStart func()) ([]string, time.Duration, time.Duration, error) {
		select {
		case installSlots <- struct{}{}:
		case <-ctx.Done():
			return nil, 0, 0, installError(path, fmt.Errorf("install cancelled while waiting for a slot: %w", ctx.Err()))
		}
		defer func() { <-installSlots }()
		return install(ctx, path, logDir, setup, onSetupStart)
	}
}

// installTailBytes caps how much of a failed install's output reaches the error.
const installTailBytes = 2000

// OutputTail is the last installTailBytes of out, starting on a line boundary
// when it had to cut.
func OutputTail(out []byte) string {
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

// UsesPnpm reports whether the worktree at path uses pnpm in any directory that
// holds a tracked lockfile. Like the install, a package.json packageManager
// beats the lockfile. A listing error counts as no.
func UsesPnpm(path string) bool {
	dirs, err := lockfileDirs(path)
	if err != nil {
		return false
	}
	for _, dir := range dirs {
		pm, _ := readPackageManager(dir.path)
		if pm != "" {
			if strings.HasPrefix(pm, "pnpm@") {
				return true
			}
			continue
		}
		if slices.Contains(dir.lockfiles, "pnpm-lock.yaml") {
			return true
		}
	}
	return false
}

// PnpmHome is the directory pnpm keeps its store and global bins in: PNPM_HOME
// from env (KEY=VALUE entries) when set, else pnpm's per-OS default under
// home. "" when neither is known.
func PnpmHome(env []string, home, goos string) string {
	for _, kv := range slices.Backward(env) {
		if v, ok := strings.CutPrefix(kv, "PNPM_HOME="); ok && v != "" {
			return v
		}
	}
	switch {
	case home == "":
		return ""
	case goos == "darwin":
		return filepath.Join(home, "Library", "pnpm")
	default:
		return filepath.Join(home, ".local", "share", "pnpm")
	}
}
