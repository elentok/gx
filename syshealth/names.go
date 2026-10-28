package syshealth

import (
	"path/filepath"
	"strings"
)

const claudeName = "claude"

var subcommandTools = map[string]bool{
	"go": true, "npm": true, "pnpm": true, "yarn": true,
	"bun": true, "git": true, "cargo": true, "make": true,
}

var interpreters = map[string]bool{
	"node": true, "sh": true, "bash": true, "zsh": true,
}

func isInterpreter(base string) bool {
	return interpreters[base] || strings.HasPrefix(base, "python")
}

// IsClaude reports whether p is a claude process. The row has no executable
// path, so the native installer's path is taken from argv[0] when it is
// absolute; that is where it shows.
func IsClaude(p Process) bool {
	if len(p.Argv) == 0 {
		return false
	}
	argv0 := p.Argv[0]
	base := filepath.Base(argv0)
	if base == claudeName {
		return true
	}
	if filepath.IsAbs(argv0) && strings.Contains(argv0, "/claude/versions/") {
		return true
	}
	if base == "node" {
		for _, a := range p.Argv[1:] {
			if strings.HasSuffix(a, "@anthropic-ai/claude-code/cli.js") {
				return true
			}
		}
	}
	return false
}

// DisplayName is the short, readable name of p for a cause list.
func DisplayName(p Process) string {
	if len(p.Argv) == 0 {
		return p.Comm
	}
	if IsClaude(p) {
		return claudeName
	}
	base := filepath.Base(p.Argv[0])
	args := p.Argv[1:]
	switch {
	case subcommandTools[base]:
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				return base + " " + a
			}
		}
		return base
	case isInterpreter(base):
		for _, a := range args {
			if a == "-c" || a == "-e" {
				return base
			}
			if !strings.HasPrefix(a, "-") {
				return filepath.Base(a)
			}
		}
		return base
	}
	return base
}
