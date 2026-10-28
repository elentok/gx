package check

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/git"
)

// The gx defaults: what a kind runs when the config has no command for it.
// Each kind is a list of steps; a step whose tool is missing is skipped.

// installNeed is a frozen install a package's tools depend on.
type installNeed struct{ dir, cmd string }

// step is one command of a gx default.
type step struct {
	name    string       // executable that must be installed; empty for brief steps
	dir     string       // working directory
	argv    []string     // arguments of name, or of the brief tool
	brief   brief.Tool   // set when the step runs in-process through brief
	install *installNeed // run first when set
}

type stepResult struct {
	skipped  string // why the step did not run
	failed   bool
	failures []string
}

// groups sorts the selected files into the languages the defaults know.
type groups struct {
	shell, python, golang []string
	packages              []*jsPackage
}

// jsPackage is the JS/TS files under one package.json.
type jsPackage struct {
	dir, lockDir string
	files        []string // relative to dir
	lock         *brief.Lockfile
}

var jsExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs"}

func (r *runner) groups() *groups {
	r.groupsOnce.Do(func() {
		g := &groups{}
		byDir := map[string]*jsPackage{}
		for _, f := range r.files {
			abs := filepath.Join(r.root, f)
			if st, err := os.Stat(abs); err != nil || st.IsDir() {
				continue
			}
			ext := filepath.Ext(f)
			switch {
			case ext == ".sh", ext == "" && hasShellShebang(abs):
				g.shell = append(g.shell, f)
			case ext == ".py":
				g.python = append(g.python, f)
			case ext == ".go":
				g.golang = append(g.golang, f)
			case slices.Contains(jsExts, ext):
				pkgDir := nearestUp(r.root, filepath.Dir(abs), func(d string) bool { return fileExists(filepath.Join(d, "package.json")) })
				if pkgDir == "" {
					continue
				}
				p := byDir[pkgDir]
				if p == nil {
					p = &jsPackage{dir: pkgDir}
					p.lockDir, p.lock = r.findLockfile(pkgDir)
					byDir[pkgDir] = p
					g.packages = append(g.packages, p)
				}
				rel, _ := filepath.Rel(pkgDir, abs)
				p.files = append(p.files, rel)
			}
		}
		r.grp = g
	})
	return r.grp
}

func (r *runner) findLockfile(pkgDir string) (string, *brief.Lockfile) {
	var found *brief.Lockfile
	dir := nearestUp(r.root, pkgDir, func(d string) bool {
		for i, l := range brief.Lockfiles {
			if fileExists(filepath.Join(d, l.File)) {
				found = &brief.Lockfiles[i]
				return true
			}
		}
		return false
	})
	return dir, found
}

// nearestUp returns the first directory from dir up to root (inclusive) for
// which ok holds, or "".
func nearestUp(root, dir string, ok func(string) bool) string {
	for d := dir; ; d = filepath.Dir(d) {
		if ok(d) {
			return d
		}
		if d == root || d == filepath.Dir(d) {
			return ""
		}
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// hasShellShebang reports whether the file starts with `#!` naming sh, bash or
// zsh, directly or through env.
func hasShellShebang(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 128)
	n, _ := f.Read(buf)
	line, _, _ := strings.Cut(string(buf[:n]), "\n")
	rest, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return false
	}
	prog := path.Base(fields[0])
	if prog == "env" {
		fields = slices.DeleteFunc(fields[1:], func(s string) bool { return strings.HasPrefix(s, "-") })
		if len(fields) == 0 {
			return false
		}
		prog = path.Base(fields[0])
	}
	return prog == "sh" || prog == "bash" || prog == "zsh"
}

// listed reports whether package.json in dir lists tool as a dependency.
func listed(dir, tool string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pj struct {
		Dependencies    map[string]json.RawMessage
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pj) != nil {
		return false
	}
	_, a := pj.Dependencies[tool]
	_, b := pj.DevDependencies[tool]
	return a || b
}

// defaultSteps lists the steps of kind's gx default and the notes for what it
// leaves out on purpose.
func (r *runner) defaultSteps(kind string) (steps []step, notes []string) {
	g := r.groups()
	files := func(name string, args []string, fs []string) {
		for _, batch := range chunkFiles(fs, r.limit) {
			steps = append(steps, step{name: name, dir: r.root, argv: append(slices.Clone(args), batch...)})
		}
	}
	switch kind {
	case "format":
		files("shfmt", []string{"-w"}, g.shell)
		files("gofmt", []string{"-w"}, g.golang)
	case "lint":
		files("shellcheck", nil, g.shell)
	case "typecheck":
		files("basedpyright", nil, g.python)
		files("ty", []string{"check"}, g.python)
	case "test":
		if dirs := goDirs(g.golang); len(dirs) > 0 {
			for _, batch := range chunkFiles(dirs, r.limit) {
				steps = append(steps, step{name: "go", dir: r.root, brief: brief.GoTest{}, argv: batch})
			}
		}
	}
	for _, p := range g.packages {
		js, note := r.jsSteps(kind, p)
		steps = append(steps, js...)
		if note != "" {
			notes = append(notes, note)
		}
	}
	return steps, notes
}

func (r *runner) jsSteps(kind string, p *jsPackage) ([]step, string) {
	if p.lock == nil {
		return nil, fmt.Sprintf("%s: no lockfile", r.rel(p.dir))
	}
	var need *installNeed
	if !dirExists(filepath.Join(p.lockDir, "node_modules")) {
		need = &installNeed{dir: p.lockDir, cmd: p.lock.Install}
	}
	has := func(tool string) bool { return listed(p.dir, tool) || listed(p.lockDir, tool) }
	runner := func(tool string, args []string, fs []string) (out []step) {
		if !has(tool) {
			return nil
		}
		argv := append(slices.Clone(p.lock.Runner[1:]), tool)
		for _, batch := range chunkFiles(fs, r.limit) {
			out = append(out, step{name: p.lock.Runner[0], dir: p.dir, argv: append(append(argv, args...), batch...), install: need})
		}
		return out
	}
	switch kind {
	case "format":
		return runner("prettier", []string{"--write"}, p.files), ""
	case "lint":
		return runner("eslint", nil, p.files), ""
	case "typecheck":
		if fileExists(filepath.Join(p.dir, "tsconfig.json")) {
			return []step{{dir: p.dir, brief: brief.Tsc{}, argv: []string{"--noEmit"}, install: need}}, ""
		}
	case "test":
		if !has("vitest") {
			return nil, ""
		}
		var out []step
		for _, batch := range chunkFiles(p.files, r.limit) {
			out = append(out, step{dir: p.dir, brief: brief.Vitest{}, argv: append([]string{"related"}, batch...), install: need})
		}
		return out, ""
	}
	return nil, ""
}

func (r *runner) rel(dir string) string {
	rel, err := filepath.Rel(r.root, dir)
	if err != nil {
		return dir
	}
	return rel
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// goDirs lists the directories to test as go package arguments. Directories go
// ignores (testdata, names starting with _ or .) are left out.
func goDirs(goFiles []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range goFiles {
		d := path.Dir(filepath.ToSlash(f))
		ignored := false
		for seg := range strings.SplitSeq(d, "/") {
			if seg == "testdata" || (seg != "." && strings.ContainsAny(seg[:1], "_.")) {
				ignored = true
			}
		}
		if ignored || seen[d] {
			continue
		}
		seen[d] = true
		if d == "." {
			dirs = append(dirs, ".")
		} else {
			dirs = append(dirs, "./"+d)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// chunkFiles splits files into groups whose total length stays under limit,
// like xargs. No files yields no groups.
func chunkFiles(files []string, limit int) [][]string {
	var out [][]string
	var cur []string
	size := 0
	for _, f := range files {
		if len(cur) > 0 && size+len(f)+1 > limit {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, f)
		size += len(f) + 1
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// runDefault runs kind's gx default into log and sets res.
func (r *runner) runDefault(kind string, log io.Writer, res *KindResult) {
	steps, notes := r.defaultSteps(kind)
	ran := 0
	for _, s := range steps {
		sr := r.runStep(kind, s, log)
		switch {
		case sr.skipped != "":
			notes = append(notes, sr.skipped)
			continue
		case sr.failed:
			res.Status = StatusFail
			res.Failures = append(res.Failures, sr.failures...)
		}
		ran++
	}
	if ran == 0 {
		res.Status = StatusSkip
		if len(notes) == 0 {
			notes = []string{"no matching files"}
		}
		res.Reason = strings.Join(slices.Compact(notes), "; ")
		return
	}
	res.Notes = append(res.Notes, slices.Compact(notes)...)
}

func (r *runner) runStep(kind string, s step, log io.Writer) stepResult {
	if s.name != "" {
		if _, err := exec.LookPath(s.name); err != nil {
			return stepResult{skipped: s.name + " not installed"}
		}
	}
	if s.install != nil {
		if out, err := r.ensureInstalled(*s.install); err != nil {
			fmt.Fprintf(log, "$ %s (in %s)\n%s%v\n", s.install.cmd, s.install.dir, out, err)
			return stepResult{failed: true}
		}
	}
	if s.brief == nil {
		fmt.Fprintf(log, "$ %s\n", strings.Join(append([]string{s.name}, s.argv...), " "))
		cmd := exec.Command(s.name, s.argv...)
		cmd.Dir = s.dir
		cmd.Env = r.childEnv(kind)
		cmd.Stdout, cmd.Stderr = log, log
		return stepResult{failed: cmd.Run() != nil}
	}
	fmt.Fprintf(log, "$ gx brief %s %s\n", s.brief.Name(), strings.Join(s.argv, " "))
	exit, outcome := brief.RunOutcome(s.brief, brief.Options{Dir: s.dir, Args: s.argv, DataDir: r.dataDir, Out: log, CheckRun: r.runID, CheckKind: kind})
	sr := stepResult{failed: exit != brief.ExitPass}
	for _, u := range outcome.Failures {
		sr.failures = append(sr.failures, u.String())
	}
	return sr
}

// ensureInstalled runs a frozen install once per directory and run; a later
// caller gets the first result.
func (r *runner) ensureInstalled(n installNeed) (string, error) {
	r.installMu.Lock()
	defer r.installMu.Unlock()
	if done, ok := r.installs[n.dir]; ok {
		return done.out, done.err
	}
	var buf bytes.Buffer
	cmd := exec.Command("sh", "-c", n.cmd)
	cmd.Dir = n.dir
	cmd.Env = r.childEnv("")
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if r.installs == nil {
		r.installs = map[string]installDone{}
	}
	r.installs[n.dir] = installDone{buf.String(), err}
	return buf.String(), err
}

type installDone struct {
	out string
	err error
}

// hashFiles maps each file to the hash of its content; a missing file maps to "".
func hashFiles(root string, files []string) map[string]string {
	h := make(map[string]string, len(files))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		h[f] = string(sum[:])
	}
	return h
}

// afterFormat stages the files format rewrote in a staged run, so the commit
// holds the formatted files, and notes the rewrite otherwise.
func (r *runner) afterFormat(before map[string]string, res *KindResult) {
	var changed []string
	for f, was := range before {
		if now := hashFiles(r.root, []string{f})[f]; now != was {
			changed = append(changed, f)
		}
	}
	if len(changed) == 0 {
		return
	}
	sort.Strings(changed)
	if r.mode != ModeStaged {
		res.Notes = append(res.Notes, fmt.Sprintf("format changed %d files in the working tree, nothing staged", len(changed)))
		return
	}
	for _, batch := range chunkFiles(changed, r.limit) {
		if stdout, stderr, err := git.Run(r.root, git.NonInteractiveEnv(), append([]string{"add", "--"}, batch...)...); err != nil {
			res.Status = StatusFail
			res.Tail = []string{"git add: " + firstLine(strings.TrimSpace(stdout+"\n"+stderr))}
			return
		}
	}
	res.Notes = append(res.Notes, fmt.Sprintf("re-staged %d files that format rewrote", len(changed)))
}
