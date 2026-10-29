package servertest

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/testutil/agentfake"
)

// SPIKE (server-tests-pty): the harness runs the pty runner by default. Its
// holder is this test binary (holderArg) and its claude is the TUI fake.

// sysctlProcs is a nativerunner.ProcessTable that reads the kernel's process
// table in process (sysctl), so a lookup spawns no `ps`. Darwin only.
type sysctlProcs struct{}

func (sysctlProcs) Lookup(pid int) (nativerunner.Process, bool, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if errors.Is(err, unix.EIO) || (err == nil && int(kp.Proc.P_pid) != pid) {
		return nativerunner.Process{}, false, nil
	}
	if err != nil {
		return nativerunner.Process{}, false, err
	}
	st := kp.Proc.P_starttime
	start := strconv.FormatInt(st.Sec, 10) + "." + strconv.FormatInt(int64(st.Usec), 10)
	return nativerunner.Process{Start: start, Cmdline: procArgs(pid)}, true, nil
}

// procArgs is pid's argv joined by spaces, or "" when it can't be read (a
// zombie, another user's process).
func procArgs(pid int) string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return ""
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	rest := raw[4:]
	// The exec path, then NUL padding, then argc NUL-ended strings.
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return ""
	}
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	args := make([]string, 0, argc)
	for range argc {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			break
		}
		args = append(args, string(rest[:j]))
		rest = rest[j+1:]
	}
	return strings.Join(args, " ")
}

// ptyRunner is the harness's pty runner: a nativerunner.Pty plus StopAll,
// which the harness cleanup calls so a run's Wait ends before WaitSettled.
type ptyRunner struct {
	*nativerunner.Pty

	mu     sync.Mutex
	launch func(p Prompt)
	cwds   map[string]string // label -> cwd
}

func (r *ptyRunner) Start(opts agentrunner.StartOptions) (agentrunner.Session, error) {
	s, err := r.Pty.Start(opts)
	if err == nil {
		r.mu.Lock()
		r.cwds[s.Label] = opts.Cwd
		r.mu.Unlock()
	}
	return s, err
}

// Prompt runs the RegisterLaunch agent, as the herdr fake's `agent prompt`
// does, then makes the TUI fake's turn start and end at once.
func (r *ptyRunner) Prompt(s agentrunner.Session, text string) error {
	return r.PromptNudged(s, text, nil)
}

func (r *ptyRunner) PromptNudged(s agentrunner.Session, text string, onNudge func(string)) error {
	r.mu.Lock()
	agent, cwd := r.launch, r.cwds[s.Label]
	r.mu.Unlock()
	if agent != nil {
		_, rest, _ := strings.Cut(text, " ")
		addr, _, _ := strings.Cut(rest, "\n")
		addr = strings.TrimPrefix(addr, "--ticket ")
		agent(Prompt{Address: addr, Cwd: cwd, Text: text})
		home := harnessLookup(r.Env, "HOME")
		if err := agentfake.SendTUIControl(home, s.SessionID, agentfake.Control{Action: agentfake.ActionFast}); err != nil {
			return err
		}
	}
	return r.Pty.PromptNudged(s, text, onNudge)
}

// StopAll stops every holder under Root.
func (r *ptyRunner) StopAll() {
	stopSessionHolders(r.Root)
}

func harnessLookup(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}

// newPtyRunner builds the default runner over stateDir with env as the
// holders' env.
func newPtyRunner(stateDir string, env []string, wait time.Duration) *ptyRunner {
	exe, _ := os.Executable()
	return &ptyRunner{
		Pty: &nativerunner.Pty{
			Root:         filepath.Join(stateDir, "agents"),
			Holder:       []string{exe, holderArg},
			Claude:       exe,
			Procs:        sysctlProcs{},
			PollInterval: 20 * time.Millisecond,
			StartTimeout: 10 * time.Second,
			PromptGrace:  500 * time.Millisecond,
			StopGrace:    2 * time.Second,
			N:            wait,
			Env:          append(env[:len(env):len(env)], agentfake.TUIEnv+"=1"),
		},
		cwds: map[string]string{},
	}
}

// herdrListed reports whether the top-level test of name is in the file
// $GX_SPIKE_HERDR_TESTS (one name per line): such a test keeps herdr.
func herdrListed(name string) bool {
	path := os.Getenv("GX_SPIKE_HERDR_TESTS")
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	top, _, _ := strings.Cut(name, "/")
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == top {
			return true
		}
	}
	return false
}
