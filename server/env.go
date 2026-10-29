package server

import (
	"errors"
	"path/filepath"

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

// lastCodexText is the last assistant text of a codex conversation.
func (s *Server) lastCodexText(id string) (string, bool, error) {
	home, err := s.codexHome()
	if err != nil {
		return "", false, err
	}
	return codexsession.LastAssistantTextIn(home, id)
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
