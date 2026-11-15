package commandlist

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/elentok/gx/brief"
)

// claudeProbeTimeout bounds one probe. The user's SessionStart hooks run
// before the reply and take a few seconds.
const claudeProbeTimeout = 15 * time.Second

const claudeInitializeRequest = `{"type":"control_request","request_id":"r1","request":{"subtype":"initialize"}}`

// claudeReply is the part of the initialize control_response we read.
type claudeReply struct {
	Type     string `json:"type"`
	Response struct {
		Response struct {
			Commands []struct {
				Name         string   `json:"name"`
				Description  string   `json:"description"`
				ArgumentHint string   `json:"argumentHint"`
				Aliases      []string `json:"aliases"`
				Builtin      bool     `json:"builtin"`
			} `json:"commands"`
		} `json:"response"`
	} `json:"response"`
}

// ProbeClaude lists the slash commands a Claude session started in dir would
// offer, merged with the built-ins. env is the environment of a Claude launch;
// the gx session and agent-dir variables are dropped from it. The probe leaves
// no transcript behind (--no-session-persistence).
func ProbeClaude(ctx context.Context, dir string, env []string) ([]Entry, error) {
	return probeClaude(ctx, dir, env, claudeProbeTimeout)
}

func probeClaude(ctx context.Context, dir string, env []string, timeout time.Duration) ([]Entry, error) {
	reply, err := runClaudeInitialize(ctx, dir, env, timeout, ParseClaudeReply)
	if err != nil {
		return nil, err
	}
	return Merge(reply, claudeBuiltins), nil
}

// runClaudeInitialize starts the short-lived claude process, sends the one
// initialize request and returns what parse reads from the reply. It is the
// process start, timeout and kill-on-reply code the command and model reads
// share.
func runClaudeInitialize[T any](ctx context.Context, dir string, env []string, timeout time.Duration, parse func(io.Reader) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env = withoutGXAgentVars(env)
	cmd := commandIn(ctx, env, "claude", "-p",
		"--input-format", "stream-json", "--output-format", "stream-json",
		"--verbose", "--no-session-persistence")
	cmd.Dir = dir
	// A stub or hook may leave a grandchild holding the pipes after the kill.
	cmd.WaitDelay = time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return zero, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return zero, err
	}
	if err := cmd.Start(); err != nil {
		return zero, fmt.Errorf("start claude: %w", err)
	}
	if _, err := io.WriteString(stdin, claudeInitializeRequest+"\n"); err != nil {
		cancel()
		_ = cmd.Wait()
		return zero, fmt.Errorf("write initialize request: %w", err)
	}

	type result struct {
		reply T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		reply, err := parse(stdout)
		done <- result{reply, err}
	}()
	var res result
	timedOut := false
	select {
	case res = <-done:
	case <-ctx.Done():
		timedOut = true
	}
	// The reply is all we need: end the child instead of waiting for it. Wait
	// kills it and, after WaitDelay, closes the pipe a blocked reader holds.
	_ = stdin.Close()
	cancel()
	waitErr := cmd.Wait()
	if timedOut {
		<-done
		return zero, fmt.Errorf("claude probe timed out after %s", timeout)
	}
	if res.err != nil {
		if errors.Is(res.err, errNoReply) && waitErr != nil {
			return zero, fmt.Errorf("claude probe failed: %w", waitErr)
		}
		return zero, res.err
	}
	return res.reply, nil
}

// commandIn builds the command for program with env as its environment. The
// program is found over env's PATH, not the process's, as a harness launch is.
func commandIn(ctx context.Context, env []string, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = env
	if path, err := brief.LookPath(env, program); err == nil {
		cmd.Path, cmd.Err = path, nil
	}
	return cmd
}

// withoutGXAgentVars drops GX_AGENT_SESSION and every GX_*AGENT_DIR variable:
// the probe is not an agent session of gx.
func withoutGXAgentVars(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "GX_AGENT_SESSION" || (strings.HasPrefix(name, "GX_") && strings.HasSuffix(name, "AGENT_DIR")) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

var errNoReply = errors.New("claude probe: no initialize reply")

// ParseClaudeReply reads Claude's stream-json output up to the initialize
// control_response and returns its commands. Lines before it (hook events) are
// skipped.
func ParseClaudeReply(r io.Reader) ([]Entry, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg claudeReply
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			return nil, fmt.Errorf("claude probe: bad JSON line: %w", err)
		}
		if msg.Type != "control_response" {
			continue
		}
		cmds := msg.Response.Response.Commands
		out := make([]Entry, 0, len(cmds))
		for _, c := range cmds {
			e := Entry{Name: c.Name, ArgHint: c.ArgumentHint, Description: c.Description, Aliases: c.Aliases, Source: SourceSkill}
			switch {
			case c.Builtin:
				e.Source = SourceBuiltIn
			case strings.HasSuffix(c.Name, " (MCP)"):
				e.Source = SourceMCP
				e.Name = mcpTypedName(strings.TrimSuffix(c.Name, " (MCP)"))
			}
			out = append(out, e)
		}
		return out, nil
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("claude probe: read reply: %w", err)
	}
	return nil, errNoReply
}

// mcpTypedName turns the reply's "server:prompt" into the form a user types,
// "mcp__server__prompt", with characters outside A-Za-z0-9_- in the server
// name replaced by "_".
func mcpTypedName(listed string) string {
	i := strings.LastIndex(listed, ":")
	if i < 0 {
		return listed
	}
	server := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		}
		return '_'
	}, listed[:i])
	return "mcp__" + server + "__" + listed[i+1:]
}
