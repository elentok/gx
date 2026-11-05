package projects

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// AgentEnv names the agent ("claude" or "codex") a ralph-loop run launched
// under, so `gx agent model` answers for that agent.
const AgentEnv = "GX_AGENT"

// envHome is the home dir the server reads: HOME of Config.Env, which New
// fills from the process env when the caller leaves it nil.
func (p *Registry) EnvHome() (string, error) {
	if home := harness.LookupEnv(p.cfg().Env, "HOME"); home != "" {
		return home, nil
	}
	return "", errors.New("HOME is not set in the server env")
}

// codexHome is the resolved codex dir of Config.Env.
func (p *Registry) CodexHome() (string, error) {
	if dir := harness.LookupEnv(p.cfg().Env, "CODEX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := p.EnvHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// lastCodexText is the text of the agent's last turn in a codex conversation:
// every agent message after the person's last one, oldest first. A Codex turn
// can hold several messages, so a round in an earlier one still counts. Without
// a turn it is the last assistant text of the rollout.
func (p *Registry) LastCodexText(id string) (string, bool, error) {
	home, err := p.CodexHome()
	if err != nil {
		return "", false, err
	}
	path, ok, err := codexsession.RolloutPathIn(home, id)
	if err != nil || !ok {
		return "", false, err
	}
	res, _, err := chatreader.ReadFile(harness.Codex, path)
	if err != nil {
		return "", false, err
	}
	texts := transcript.TrailingAgentTexts(res.Messages)
	if len(texts) == 0 {
		return codexsession.LastAssistantTextIn(home, id)
	}
	slices.Reverse(texts)
	return strings.Join(texts, "\n\n"), true, nil
}

// transcriptPath is the claude transcript path of a session run in cwd.
func (p *Registry) TranscriptPath(cwd, sessionID string) (string, error) {
	home, err := p.EnvHome()
	if err != nil {
		return "", err
	}
	return transcript.PathIn(home, cwd, sessionID), nil
}

// defaultDeps is ralphloop's default deps reading Config.Env's home dirs.
func (p *Registry) DefaultDeps() ralphloop.Deps {
	o := ralphloop.DepsOverrides{RunnerWait: p.cfg().RunnerWait, Finished: p.back.SessionFinished, RateLimitResetBuffer: p.cfg().RateLimitResetBuffer, GuardPoll: p.cfg().GuardPoll}
	o.Home, _ = p.EnvHome()
	o.CodexHome, _ = p.CodexHome()
	o.StateDir, _ = p.LockStateDir()
	return ralphloop.DefaultDepsWithOverrides(o)
}

// lockStateDir is the gx state dir of Config.Env that merge locks live under.
func (p *Registry) LockStateDir() (string, error) {
	if dir := harness.LookupEnv(p.cfg().Env, "XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "gx"), nil
	}
	home, err := p.EnvHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "gx"), nil
}

// mergeLockDir is the merge lock dir of the epic at epicPath.
func (p *Registry) MergeLockDir(epicPath string) (string, error) {
	stateDir, err := p.LockStateDir()
	if err != nil {
		return "", err
	}
	return ralphloop.MergeLockDirIn(stateDir, epicPath)
}

// typing is the typing rules for the claude sessions this server runs, reading
// claude's config from Config.Env.
func (p *Registry) Typing() harness.Typing { return harness.ClaudeTypingIn(p.cfg().Env) }

// typingIO is the ready-to-type step's view of the session in dir. It carries
// the session id the probe keys its memory by, the server logger and the
// caller's label, so the probe's decisions reach server.log.
func (p *Registry) TypingIO(dir string, c *holder.HolderClient, caller string) harness.TypingIO {
	io := nativerunner.HolderTypingIO(c)
	if meta, err := readTypingMeta(dir); err == nil {
		io.Session = meta.ConversationID
	}
	io.Log = p.log.With("gx_session", filepath.Base(dir))
	io.Caller = caller
	return io
}

// typingMeta is the part of a session's meta.json the typing rules read.
type typingMeta struct {
	Harness        string `json:"harness"`
	ConversationID string `json:"conversation_id"`
}

func readTypingMeta(dir string) (typingMeta, error) {
	var meta typingMeta
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(data, &meta)
}

// sessionTyping is the typing rules of the session in dir, claude's when its
// meta can't be read.
func (p *Registry) SessionTyping(dir string) harness.Typing {
	meta, err := readTypingMeta(dir)
	if err != nil {
		return p.Typing()
	}
	return p.TypingFor(meta.Harness)
}

// typingFor is the typing rules of a session's harness kind; an empty or
// unknown kind types as claude.
func (p *Registry) TypingFor(kind string) harness.Typing {
	if kind != "" && kind != string(harness.Claude) {
		if t, err := harness.LookupTyping(agentrunner.Kind(kind)); err == nil {
			return t
		}
	}
	return p.Typing()
}
