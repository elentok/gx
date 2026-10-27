package check

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Candidate is a command detection found, with the file and name it came from.
type Candidate struct {
	Command string `json:"command"`
	Source  string `json:"source"`
}

// DefaultLine says what the gx default runs for a kind.
var DefaultLine = map[string]string{
	"format":    "gofmt / prettier on changed files",
	"lint":      "go vet / eslint on changed files",
	"typecheck": "go build / tsc on changed packages",
	"test":      "go test / vitest on changed packages",
}

// scriptNames lists the exact names that match a kind. Nothing else matches,
// so a build script is never offered.
var scriptNames = map[string][]string{
	"format":    {"format", "fmt"},
	"lint":      {"lint"},
	"typecheck": {"typecheck", "type-check", "tsc"},
	"test":      {"test"},
}

// Detect reads root's Makefile, mise.toml, justfile and package.json files and
// returns each kind's candidates in rank order. It never runs anything.
func Detect(root string) map[string][]Candidate {
	found := map[string][]Candidate{}
	add := func(names []string, rel string, run func(name string) string) {
		for _, kind := range Kinds {
			for _, name := range names {
				if slices.Contains(scriptNames[kind], name) {
					found[kind] = append(found[kind], Candidate{Command: run(name), Source: rel + " · " + name})
				}
			}
		}
	}
	makeRun := func(n string) string { return "make " + n }
	miseRun := func(n string) string { return "mise run " + n }
	justRun := func(n string) string { return "just " + n }
	add(makefileTargets(filepath.Join(root, "Makefile")), "Makefile", makeRun)
	add(miseTasks(filepath.Join(root, "mise.toml")), "mise.toml", miseRun)
	add(justRecipes(filepath.Join(root, "justfile")), "justfile", justRun)

	pm := packageManager(root)
	for _, dir := range packageDirs(root) {
		rel := filepath.ToSlash(filepath.Join(dir, "package.json"))
		if dir == "." {
			rel = "package.json"
		}
		add(packageScripts(filepath.Join(root, rel)), rel, func(n string) string { return pm.run(dir, n) })
	}
	return found
}

func scanLines(path string, fn func(line string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fn(sc.Text())
	}
	_ = sc.Err() // a partly read file still yields the names found so far
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9_][A-Za-z0-9_.-]*)\s*:($|[^=])`)

func makefileTargets(path string) []string {
	var names []string
	scanLines(path, func(line string) {
		if m := makeTarget.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	})
	return names
}

var justRecipe = regexp.MustCompile(`^@?([A-Za-z_][A-Za-z0-9_-]*)[^:=]*:($|[^=])`)

func justRecipes(path string) []string {
	var names []string
	scanLines(path, func(line string) {
		if m := justRecipe.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	})
	return names
}

var miseKey = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=`)

// miseTasks scans for `[tasks.<name>]` headers and keys directly under
// `[tasks]`. A line scan is enough for the names; there is no TOML library.
func miseTasks(path string) []string {
	var names []string
	inTasks := false
	scanLines(path, func(line string) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			header := strings.Trim(line, "[] ")
			inTasks = header == "tasks"
			if name, ok := strings.CutPrefix(header, "tasks."); ok {
				names = append(names, strings.Trim(name, `"'`))
			}
			return
		}
		if inTasks {
			if m := miseKey.FindStringSubmatch(line); m != nil {
				names = append(names, m[1])
			}
		}
	})
	return names
}

type packageJSON struct {
	Scripts    map[string]json.RawMessage `json:"scripts"`
	Workspaces json.RawMessage            `json:"workspaces"`
}

func readPackageJSON(path string) packageJSON {
	var p packageJSON
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &p)
	}
	return p
}

func packageScripts(path string) []string {
	return slices.Sorted(func(yield func(string) bool) {
		for name := range readPackageJSON(path).Scripts {
			if !yield(name) {
				return
			}
		}
	})
}

// packageDirs lists the root first, then the workspace packages the root
// package.json names, as root-relative slash paths.
func packageDirs(root string) []string {
	dirs := []string{"."}
	p := readPackageJSON(filepath.Join(root, "package.json"))
	var globs []string
	if json.Unmarshal(p.Workspaces, &globs) != nil {
		var obj struct {
			Packages []string `json:"packages"`
		}
		if json.Unmarshal(p.Workspaces, &obj) == nil {
			globs = obj.Packages
		}
	}
	for _, g := range globs {
		matches, _ := filepath.Glob(filepath.Join(root, g, "package.json"))
		slices.Sort(matches)
		for _, m := range matches {
			if rel, err := filepath.Rel(root, filepath.Dir(m)); err == nil {
				dirs = append(dirs, filepath.ToSlash(rel))
			}
		}
	}
	return dirs
}

type pkgManager string

func packageManager(root string) pkgManager {
	for _, l := range []struct{ file, pm string }{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}} {
		if _, err := os.Stat(filepath.Join(root, l.file)); err == nil {
			return pkgManager(l.pm)
		}
	}
	return "npm"
}

// run is the command that runs script name of the package in dir (root-relative).
func (pm pkgManager) run(dir, name string) string {
	if dir == "." {
		return string(pm) + " run " + name
	}
	flag := map[pkgManager]string{"pnpm": "--dir", "yarn": "--cwd", "npm": "--prefix"}[pm]
	return string(pm) + " " + flag + " " + dir + " run " + name
}
