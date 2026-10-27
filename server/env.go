package server

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// envHome is the home dir the server reads: HOME of Config.Env, or the process
// home when Config.Env is nil.
func (s *Server) envHome() (string, error) {
	if s.cfg.Env == nil {
		return os.UserHomeDir()
	}
	if home := harness.LookupEnv(s.cfg.Env, "HOME"); home != "" {
		return home, nil
	}
	return "", errors.New("HOME is not set in the server env")
}

// totalsHome is envHome, or "" for the process env, which ralphloop resolves
// itself.
func (s *Server) totalsHome() (string, error) {
	if s.cfg.Env == nil {
		return "", nil
	}
	return s.envHome()
}

// codexHome is the resolved codex dir of Config.Env, or "" for the process env,
// which codexsession resolves itself.
func (s *Server) codexHome() (string, error) {
	if s.cfg.Env == nil {
		return "", nil
	}
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
	if home == "" {
		return codexsession.LastAssistantText(id)
	}
	return codexsession.LastAssistantTextIn(home, id)
}

// transcriptPath is the claude transcript path of a session run in cwd.
func (s *Server) transcriptPath(cwd, sessionID string) (string, error) {
	if s.cfg.Env == nil {
		return transcript.Path(cwd, sessionID)
	}
	home, err := s.envHome()
	if err != nil {
		return "", err
	}
	return transcript.PathIn(home, cwd, sessionID), nil
}

// defaultDeps is ralphloop's default deps reading Config.Env's home dirs. A nil
// Config.Env keeps the process env.
func (s *Server) defaultDeps() ralphloop.Deps {
	if s.cfg.Env == nil {
		return ralphloop.DefaultDeps()
	}
	var o ralphloop.DepsOverrides
	o.Home, _ = s.envHome()
	o.CodexHome, _ = s.codexHome()
	o.StateDir, _ = s.lockStateDir()
	return ralphloop.DefaultDepsWithOverrides(o)
}

// lockStateDir is the gx state dir of Config.Env that merge locks live under,
// or "" for the process env, which ralphloop resolves itself.
func (s *Server) lockStateDir() (string, error) {
	if s.cfg.Env == nil {
		return "", nil
	}
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
