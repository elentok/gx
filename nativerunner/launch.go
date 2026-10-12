package nativerunner

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Launch helpers shared by the headless runner, the pty runner and the holder.

// MetaFile is where an agent directory records its launch.
const MetaFile = "meta.json"

// Meta is what meta.json records about a launched agent.
type Meta struct {
	Runner string `json:"runner"`
	// PID is the agent's own pid, claude on every runner.
	PID      int    `json:"pid"`
	PIDStart string `json:"pid_start"`
	// HolderPID is the pty holder that owns claude's terminal (pty only).
	HolderPID      int    `json:"holder_pid,omitempty"`
	HolderPIDStart string `json:"holder_pid_start,omitempty"`
	SessionID      string `json:"session_id"`
	Label          string `json:"label"`
	Epic           string `json:"epic"`
	Cwd            string `json:"cwd"`
	// Offset is where this session's events begin in out.jsonl, always at a
	// line boundary. Earlier lines belong to a previous run in the same dir.
	Offset int64 `json:"offset"`
}

// LaunchEnv drops only the vars that make claude think it is nested inside
// another claude session, which would stop it saving its transcript and its
// pid status file.
func LaunchEnv(base, extra []string) []string {
	env := slices.DeleteFunc(slices.Clone(base), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return name == "CLAUDECODE" || name == "CLAUDE_CODE_ENTRYPOINT" || name == "CLAUDE_CODE_CHILD_SESSION"
	})
	env = append(env, "DISABLE_AUTOUPDATER=1")
	return append(env, extra...)
}

// StartDetached starts cmd in its own session so it outlives the server and
// its pid leads a process group that can be signalled as a whole.
func StartDetached(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// NewUUID returns a random version 4 uuid.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// RecordLaunch confirms the launched process is still alive, stamps its start
// time into meta and writes meta.json.
func RecordLaunch(procs ProcessTable, dir string, meta *Meta) error {
	proc, ok, err := procs.Lookup(meta.PID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pid %d exited during launch", meta.PID)
	}
	meta.PIDStart = proc.Start
	return WriteMeta(dir, *meta)
}

func WriteMeta(dir string, m Meta) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, MetaFile), append(data, '\n'), 0o644)
}
