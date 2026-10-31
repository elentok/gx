package codexdoctor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/sessionlookup"
)

const (
	sessionTimeout = 3 * time.Minute
	// settle lets the last frames and hook runs land after task_complete.
	settle = 300 * time.Millisecond
)

// Drivers runs the canned live sessions against the codex found on Env's PATH.
// Each session runs once, however many checks read it. Credentials are copied
// into a throwaway CODEX_HOME so the doctor never writes into the real one.
type Drivers struct {
	// Env is the caller's env: PATH finds codex; HOME and CODEX_HOME say where
	// the credentials live.
	Env []string
	// Timeout bounds each PTY session.
	Timeout time.Duration

	// afterPrompt, when set, runs once the prompt is typed. Tests use it to
	// drive a fake Codex that does not act on a prompt by itself.
	afterPrompt func(codexHome string)

	launch, turn, deny, commit func() (Recording, error)
}

func NewDrivers(env []string) *Drivers {
	d := &Drivers{Env: env, Timeout: sessionTimeout}
	d.launch = sync.OnceValues(d.runLaunch)
	d.turn = sync.OnceValues(d.runTurn)
	d.deny = sync.OnceValues(d.runDeny)
	d.commit = sync.OnceValues(d.runCommit)
	return d
}

// Session is a Source.Session for every live session. Names it does not know
// are skipped, not failed.
func (d *Drivers) Session(name string) (Recording, error) {
	switch name {
	case LaunchSession:
		return d.launch()
	case TurnSession:
		return d.turn()
	case DenySession:
		return d.deny()
	case CommitSession:
		return d.commit()
	case LiveRolloutsSession:
		return d.liveRollouts()
	}
	return Recording{}, ErrNoSession
}

// liveRollouts is the rollouts the PTY sessions produced, nothing older.
func (d *Drivers) liveRollouts() (Recording, error) {
	var rec Recording
	var errs []error
	for _, run := range []func() (Recording, error){d.turn, d.deny, d.commit} {
		r, err := run()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		rec.Rollouts = append(rec.Rollouts, r.Rollouts...)
	}
	if len(rec.Rollouts) == 0 && len(errs) > 0 {
		return Recording{}, errors.Join(errs...)
	}
	return rec, nil
}

func (d *Drivers) runLaunch() (Recording, error) {
	base, err := tempDir("gx-doctor-launch-")
	if err != nil {
		return Recording{}, err
	}
	defer os.RemoveAll(base)
	home, codexHome, work := filepath.Join(base, "home"), filepath.Join(base, "codex-home"), filepath.Join(base, "work")
	for _, p := range []string{home, codexHome, work} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return Recording{}, err
		}
	}
	bin, err := codexPath(d.Env)
	if err != nil {
		return Recording{}, err
	}
	args := LaunchArgs([]string{work})
	cmd := exec.Command(bin, append(slices.Clone(args), "debug", "prompt-input", "probe")...)
	cmd.Dir = work
	cmd.Env = withEnv(d.Env, "HOME="+home, "CODEX_HOME="+codexHome,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	rec := Recording{Launch: Launch{Args: args}}
	if err := runWithTimeout(cmd, d.Timeout); err != nil {
		rec.Launch.Err, rec.Launch.Stderr = err.Error(), stderr.String()
	}
	return rec, nil
}

func (d *Drivers) runTurn() (Recording, error) {
	cwd, err := tempDir("gx-doctor-turn-")
	if err != nil {
		return Recording{}, err
	}
	defer os.RemoveAll(cwd)
	return d.runPTY(ptySpec{cwd: cwd, prompt: "Run the shell command `echo gx-doctor`, then reply with the word done."})
}

// runDeny asks for a write outside the writable roots; the recording hook
// answers the approval with a deny. The target sits under HOME because the
// sandbox leaves temp dirs writable.
func (d *Drivers) runDeny() (Recording, error) {
	cwd, err := tempDir("gx-doctor-deny-")
	if err != nil {
		return Recording{}, err
	}
	defer os.RemoveAll(cwd)
	home := harness.LookupEnv(d.Env, "HOME")
	if home == "" {
		return Recording{}, errors.New("HOME is not set")
	}
	target := filepath.Join(home, fmt.Sprintf(".gx-doctor-denied-%d", time.Now().UnixNano()))
	defer os.Remove(target)
	rec, err := d.runPTY(ptySpec{
		cwd:    cwd,
		deny:   true,
		prompt: "Run the shell command `touch " + target + "`, then reply with the word done.",
	})
	if err != nil {
		return Recording{}, err
	}
	_, statErr := os.Stat(target)
	rec.DenyHonoured = errors.Is(statErr, fs.ErrNotExist) && slices.Contains(rec.Hooks, "PermissionRequest")
	return rec, nil
}

// runCommit commits from a Codex shell call inside a throwaway linked
// worktree whose git dirs are the only writable roots besides the cwd.
func (d *Drivers) runCommit() (Recording, error) {
	base, err := tempDir("gx-doctor-commit-")
	if err != nil {
		return Recording{}, err
	}
	defer os.RemoveAll(base)
	repo, wt := filepath.Join(base, "repo"), filepath.Join(base, "wt")
	store, scratch := filepath.Join(base, "store"), filepath.Join(base, "scratch")
	for _, p := range []string{repo, store, scratch} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return Recording{}, err
		}
	}
	for _, args := range [][]string{
		{"-C", repo, "init", "-q", "-b", "main"},
		{"-C", repo, "config", "user.name", "gx-doctor"},
		{"-C", repo, "config", "user.email", "gx-doctor@example.invalid"},
		{"-C", repo, "commit", "-q", "--allow-empty", "-m", "initial"},
		{"-C", repo, "worktree", "add", "-q", "-b", "doctor-probe", wt},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return Recording{}, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	gitDirOut, err := exec.Command("git", "-C", wt, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return Recording{}, err
	}
	roots := []string{strings.TrimSpace(string(gitDirOut)), filepath.Join(repo, ".git"), store, scratch}
	rec, err := d.runPTY(ptySpec{
		cwd:    wt,
		roots:  roots,
		prompt: "Run the shell command `git commit --allow-empty -m gx-doctor-probe` in the current directory, then reply with the word done.",
	})
	if err != nil {
		return Recording{}, err
	}
	out, err := exec.Command("git", "-C", wt, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		return Recording{}, err
	}
	if n := strings.TrimSpace(string(out)); n != "2" {
		rec.Probe.Err = fmt.Sprintf("the worktree has %s commits after the shell call, want 2", n)
	}
	return rec, nil
}

// ptySpec describes one interactive session.
type ptySpec struct {
	cwd    string
	roots  []string
	prompt string
	// deny makes the recording hook deny every PermissionRequest.
	deny bool
}

// runPTY starts codex on a pty (not nativerunner.Pty, which hides the raw
// output), records the output with timestamps, answers OSC 7501 probes, types
// the prompt once codex reports idle, and stops at the rollout's task_complete.
func (d *Drivers) runPTY(spec ptySpec) (Recording, error) {
	bin, err := codexPath(d.Env)
	if err != nil {
		return Recording{}, err
	}
	tmp, err := tempDir("gx-doctor-pty-")
	if err != nil {
		return Recording{}, err
	}
	defer os.RemoveAll(tmp)
	codexHome := filepath.Join(tmp, "codex-home")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return Recording{}, err
	}
	realHome := sessionlookup.RootsFrom(d.Env).CodexHome
	for _, name := range []string{"auth.json", "config.toml"} {
		if data, err := os.ReadFile(filepath.Join(realHome, name)); err == nil {
			if err := os.WriteFile(filepath.Join(codexHome, name), data, 0o600); err != nil {
				return Recording{}, err
			}
		}
	}
	hookLog := filepath.Join(tmp, "hooks.log")
	script := filepath.Join(tmp, "hook.sh")
	if err := os.WriteFile(script, []byte(hookScript(hookLog, spec.deny)), 0o755); err != nil {
		return Recording{}, err
	}
	command := "sh " + shellQuote(script)
	args := harness.CodexArgs(harness.CodexArgsInput{
		WritableRoots: spec.roots,
		Hooks:         &harness.CodexHookCommands{Session: command, GuardBash: command, GuardQuestion: command, Permission: command},
	})
	// A fresh temp dir would otherwise stop at the trust dialog.
	args = append(args, "-c", "projects."+strconv.Quote(spec.cwd)+".trust_level=\"trusted\"")
	args = codexHarness().Launch(harness.LaunchOptions{Args: args}).Args

	cmd := exec.Command(bin, args...)
	cmd.Dir = spec.cwd
	cmd.Env = withEnv(d.Env, "CODEX_HOME="+codexHome)
	start := time.Now()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return Recording{}, err
	}
	rec := &ptyRecorder{ptmx: ptmx, start: start, idle: make(chan struct{}), done: make(chan struct{})}
	go rec.copy()
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = ptmx.Close()
		<-rec.done
	}()

	timeout := time.After(d.Timeout)
	select {
	case <-rec.idle:
	case <-rec.done:
		return Recording{}, errors.New("codex exited before it reported idle")
	case <-timeout:
		return Recording{}, errors.New("codex never reported idle")
	}
	if _, err := io.WriteString(ptmx, spec.prompt); err != nil {
		return Recording{}, err
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := io.WriteString(ptmx, "\r"); err != nil {
		return Recording{}, err
	}
	if d.afterPrompt != nil {
		go d.afterPrompt(codexHome)
	}

	var rollouts []Rollout
	for {
		rollouts, err = readRollouts(codexHome)
		if err != nil {
			return Recording{}, err
		}
		if slices.ContainsFunc(rollouts, func(r Rollout) bool { return bytes.Contains(r.Data, []byte(`"task_complete"`)) }) {
			break
		}
		select {
		case <-timeout:
			return Recording{}, errors.New("no task_complete in the rollout before the timeout")
		case <-rec.done:
			return Recording{}, errors.New("codex exited before the turn completed")
		case <-time.After(200 * time.Millisecond):
		}
	}
	time.Sleep(settle)
	if rollouts, err = readRollouts(codexHome); err != nil {
		return Recording{}, err
	}
	var hooks []string
	if data, err := os.ReadFile(hookLog); err == nil {
		hooks = strings.Fields(string(data))
	}
	return Recording{Frames: rec.snapshot(), Rollouts: rollouts, Hooks: hooks}, nil
}

// ptyRecorder copies the pty's output into timestamped frames and answers the
// terminal-side half of OSC 7501.
type ptyRecorder struct {
	ptmx  io.ReadWriter
	start time.Time

	mu     sync.Mutex
	frames []Frame
	parser harness.ProgramStatusParser

	idle     chan struct{}
	idleOnce sync.Once
	done     chan struct{}
}

func (r *ptyRecorder) copy() {
	defer close(r.done)
	buf := make([]byte, 32*1024)
	for {
		n, err := r.ptmx.Read(buf)
		if n > 0 {
			chunk := bytes.Clone(buf[:n])
			r.mu.Lock()
			r.frames = append(r.frames, Frame{T: time.Since(r.start).Seconds(), Data: chunk})
			probes, reports := r.parser.Feed(chunk)
			r.mu.Unlock()
			for range probes {
				_, _ = io.WriteString(r.ptmx, harness.ProgramStatusProbe)
			}
			for _, rep := range reports {
				if rep.App == "codex" && rep.State == "idle" {
					r.idleOnce.Do(func() { close(r.idle) })
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (r *ptyRecorder) snapshot() []Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.frames)
}

// hookScript logs each hook event name to log. With deny it answers a
// PermissionRequest with the deny decision gx's own hook uses.
func hookScript(log string, deny bool) string {
	script := "payload=$(cat)\n" +
		"event=$(printf '%s' \"$payload\" | sed -n 's/.*\"hook_event_name\" *: *\"\\([A-Za-z]*\\)\".*/\\1/p')\n" +
		"printf '%s\\n' \"$event\" >> " + shellQuote(log) + "\n"
	if deny {
		script += "if [ \"$event\" = PermissionRequest ]; then\n" +
			"  printf '%s' '{\"hookSpecificOutput\":{\"decision\":{\"behavior\":\"deny\",\"message\":\"denied by gx codex doctor\"},\"hookEventName\":\"PermissionRequest\"}}'\n" +
			"fi\n"
	}
	return script
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// readRollouts is every rollout-*.jsonl under home/sessions, by base name.
func readRollouts(home string) ([]Rollout, error) {
	var out []Rollout
	err := filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() || !strings.HasPrefix(e.Name(), "rollout-") || !strings.HasSuffix(e.Name(), ".jsonl") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, Rollout{Name: e.Name(), Data: data})
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// tempDir is a fresh temp dir with symlinks resolved: the sandbox wants real
// paths, and macOS temp dirs sit behind one.
func tempDir(prefix string) (string, error) {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return real, nil
}

// codexPath finds codex on env's PATH; exec.Command would read this process's.
func codexPath(env []string) (string, error) {
	for _, dir := range filepath.SplitList(harness.LookupEnv(env, "PATH")) {
		p := filepath.Join(dir, "codex")
		if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", errors.New("codex not found on PATH")
}

// withEnv returns env with each KEY=VALUE in set replacing any earlier KEY.
func withEnv(env []string, set ...string) []string {
	keys := map[string]bool{}
	for _, kv := range set {
		key, _, _ := strings.Cut(kv, "=")
		keys[key] = true
	}
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		key, _, _ := strings.Cut(kv, "=")
		return keys[key]
	})
	return append(out, set...)
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("timed out after %s", timeout)
	}
}
