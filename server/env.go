package server

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/chatreader"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// envHome is the home dir the server reads: HOME of Config.Env, which New
// fills from the process env when the caller leaves it nil.
func (s *Server) envHome() (string, error) {
	if home := harness.LookupEnv(s.cfg.Env, "HOME"); home != "" {
		return home, nil
	}
	return "", errors.New("HOME is not set in the server env")
}

// codexHome is the resolved codex dir of Config.Env.
func (s *Server) codexHome() (string, error) {
	if dir := harness.LookupEnv(s.cfg.Env, "CODEX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := s.envHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// lastCodexText is the text of the agent's last turn in a codex conversation:
// every agent message after the person's last one, oldest first. A Codex turn
// can hold several messages, so a round in an earlier one still counts. Without
// a turn it is the last assistant text of the rollout.
func (s *Server) lastCodexText(id string) (string, bool, error) {
	home, err := s.codexHome()
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
	texts := trailingAgentTexts(res.Messages)
	if len(texts) == 0 {
		return codexsession.LastAssistantTextIn(home, id)
	}
	slices.Reverse(texts)
	return strings.Join(texts, "\n\n"), true, nil
}

// transcriptPath is the claude transcript path of a session run in cwd.
func (s *Server) transcriptPath(cwd, sessionID string) (string, error) {
	home, err := s.envHome()
	if err != nil {
		return "", err
	}
	return transcript.PathIn(home, cwd, sessionID), nil
}

// defaultDeps is ralphloop's default deps reading Config.Env's home dirs.
func (s *Server) defaultDeps() ralphloop.Deps {
	o := ralphloop.DepsOverrides{RunnerWait: s.cfg.RunnerWait, Finished: s.sessionFinished}
	o.Home, _ = s.envHome()
	o.CodexHome, _ = s.codexHome()
	o.StateDir, _ = s.lockStateDir()
	return ralphloop.DefaultDepsWithOverrides(o)
}

// lockStateDir is the gx state dir of Config.Env that merge locks live under.
func (s *Server) lockStateDir() (string, error) {
	if dir := harness.LookupEnv(s.cfg.Env, "XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "gx"), nil
	}
	home, err := s.envHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "gx"), nil
}

// mergeLockDir is the merge lock dir of the epic at epicPath.
func (s *Server) mergeLockDir(epicPath string) (string, error) {
	stateDir, err := s.lockStateDir()
	if err != nil {
		return "", err
	}
	return ralphloop.MergeLockDirIn(stateDir, epicPath)
}

// typing is the typing rules for the claude sessions this server runs, reading
// claude's config from Config.Env.
func (s *Server) typing() harness.Typing { return harness.ClaudeTypingIn(s.cfg.Env) }

// sessionTyping is the typing rules of the session in dir, claude's when its
// meta can't be read.
func (s *Server) sessionTyping(dir string) harness.Typing {
	meta, err := readSessionMeta(dir)
	if err != nil {
		return s.typing()
	}
	return s.typingFor(meta.Harness)
}

// typingFor is the typing rules of a session's harness kind; an empty or
// unknown kind types as claude.
func (s *Server) typingFor(kind string) harness.Typing {
	if kind != "" && kind != string(harness.Claude) {
		if t, err := harness.LookupTyping(agentrunner.Kind(kind)); err == nil {
			return t
		}
	}
	return s.typing()
}
