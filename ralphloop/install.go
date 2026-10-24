package ralphloop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
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
func installDependenciesWith(path, pathEnv string) (commands []string, err error) {
	dirs, err := lockfileDirs(path)
	if err != nil {
		return nil, installError(path, err)
	}
	for _, dir := range dirs {
		command, err := installIn(dir.path, dir.lockfiles, pathEnv)
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

// installIn chooses dir's manager and runs its install there.
func installIn(dir string, lockfiles []string, pathEnv string) ([]string, error) {
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
	cmd := exec.Command(name, command[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=true")
	if out, err := cmd.CombinedOutput(); err != nil {
		if len(bytes.TrimSpace(out)) == 0 {
			return nil, fmt.Errorf("%s: %w", strings.Join(command, " "), err)
		}
		return nil, fmt.Errorf("%s: %s", strings.Join(command, " "), out)
	}
	return command, nil
}

// chooseCommand picks the install command: packageManager when set, else the
// directory's lockfiles.
func chooseCommand(dir string, lockfiles []string, packageManager string) ([]string, error) {
	if packageManager != "" {
		name, version, found := strings.Cut(packageManager, "@")
		major, _ := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
		command, ok := installCommand(name, major >= 2)
		if !found || version == "" || !ok {
			return nil, fmt.Errorf("unsupported packageManager %q", packageManager)
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
