package agentfake

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorkerConfigFile is read from the agent's working directory (the iteration
// worktree, so it must be committed in the repo). The server launches the
// agent with fixed args and no env, so this file is the worker's only knob.
const WorkerConfigFile = ".agentfake-worker.json"

// StartedSuffix names the marker file (Hold + StartedSuffix) the worker creates
// once it has received a prompt: herdr's agent status never reports "working",
// so tests wait on this instead.
const StartedSuffix = ".started"

// WorkerConfig tells RunWorker how to finish a ticket.
type WorkerConfig struct {
	Gx   string   `json:"gx"`             // gx binary used to report the iteration finished
	Env  []string `json:"env,omitempty"`  // KEY=VALUE entries gx runs with (XDG dirs)
	Hold string   `json:"hold,omitempty"` // while this file exists the agent stays "working"
	// HoldTimeout is a Go duration after which the hold is ignored, so a flow
	// that forgets to release it cannot hang; empty means no limit.
	HoldTimeout string `json:"hold_timeout,omitempty"`
	// Behaviour maps a ticket address to what the worker does with it:
	// BehaviourFinish (the default), BehaviourAsk, BehaviourFail or BehaviourPermission.
	Behaviour map[string]string `json:"behaviour,omitempty"`
}

// What a worker does with a ticket.
const (
	// BehaviourFinish commits and reports the iteration finished.
	BehaviourFinish = "finish"
	// BehaviourAsk pauses the ticket with a question and commits nothing.
	BehaviourAsk = "ask"
	// BehaviourFail goes idle without a commit or a report, which the server
	// reads as a stalled agent.
	BehaviourFail = "fail"
	// BehaviourPermission blocks the TUI fake on a Bash permission prompt, which
	// only the pty runner can approve. WorkTicket itself does nothing for it.
	BehaviourPermission = "permission"
)

const askBody = "## Question\nWhich option should the fake worker take?\n\n## Handoff\nNo files touched; the fake worker waits for an answer.\n"

// LoadWorkerConfig reads WorkerConfigFile from dir.
func LoadWorkerConfig(dir string) (WorkerConfig, error) {
	var cfg WorkerConfig
	raw, err := os.ReadFile(filepath.Join(dir, WorkerConfigFile))
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(raw, &cfg)
}

// WorkTicket does one ticket's work in the current directory as cfg says: it
// marks the prompt received, holds the finish while the Hold file exists, then
// finishes, asks or fails.
func WorkTicket(cfg WorkerConfig, address string) error {
	if cfg.Hold != "" {
		if err := os.WriteFile(cfg.Hold+StartedSuffix, nil, 0o644); err != nil {
			return err
		}
	}
	switch cfg.Behaviour[address] {
	case BehaviourFail:
		return nil
	case BehaviourAsk:
		return askQuestion(cfg, address)
	}
	waitHold(cfg)
	return finishTicket(cfg, address)
}

func waitHold(cfg WorkerConfig) {
	limit, _ := time.ParseDuration(cfg.HoldTimeout)
	start := time.Now()
	for fileExists(cfg.Hold) && (limit <= 0 || time.Since(start) < limit) {
		time.Sleep(100 * time.Millisecond)
	}
}

func askQuestion(cfg WorkerConfig, address string) error {
	cmd := exec.Command(cfg.Gx, "tickets", "ask", address, "--body", "-")
	cmd.Env = append(os.Environ(), cfg.Env...)
	cmd.Stdin = strings.NewReader(askBody)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gx tickets ask: %w\n%s", err, out)
	}
	return nil
}

// RunWorker is a fake agent that does a ticket's work: for each submitted
// prompt (`/skill <address>`) it stays working while the Hold file exists,
// commits a file in the current directory, reports the iteration finished
// through gx, and goes idle again.
func RunWorker(in io.Reader, out io.Writer) error {
	raw, err := os.ReadFile(WorkerConfigFile)
	if err != nil {
		return err
	}
	var cfg WorkerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	fmt.Fprint(out, idleTitle)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		fmt.Fprint(out, workingTitle)
		if err := WorkTicket(cfg, fields[len(fields)-1]); err != nil {
			return err
		}
		fmt.Fprint(out, idleTitle)
	}
	return nil
}

func finishTicket(cfg WorkerConfig, address string) error {
	name := "done-" + strings.NewReplacer(":", "-", "/", "-").Replace(address) + ".txt"
	if err := os.WriteFile(name, []byte("done\n"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"git", "add", name},
		{"git", "-c", "user.name=agentfake", "-c", "user.email=agentfake@example.com", "commit", "-m", "work on " + address},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}
	cmd := exec.Command(cfg.Gx, "tickets", "set", address, "--iteration-status", "finished")
	cmd.Env = append(os.Environ(), cfg.Env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gx tickets set: %w\n%s", err, out)
	}
	return nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
