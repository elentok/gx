package codexdoctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/sessionlookup"
)

// LaunchArgs is the arg list the launch-flags check runs codex with. It comes
// from the production launch path, so the check cannot drift from it.
func LaunchArgs(writableRoots []string) []string {
	args := harness.CodexArgs(harness.CodexArgsInput{WritableRoots: writableRoots})
	return codexHarness().Launch(harness.LaunchOptions{Args: args}).Args
}

func codexHarness() harness.Harness {
	h, err := harness.Lookup(harness.Codex)
	if err != nil {
		panic(err) // Codex is always registered
	}
	return h
}

// checkLaunchFlags passes when codex accepted gx's launch args. A rejection
// names the flags or -c keys that appear in codex's stderr.
func checkLaunchFlags(in Input) (string, bool) {
	l := in.Launch
	if l.Err == "" {
		return fmt.Sprintf("codex accepted %d launch args", len(l.Args)), true
	}
	if named := rejectedArgs(l.Args, l.Stderr); len(named) > 0 {
		return "codex rejected " + strings.Join(named, ", "), false
	}
	return fmt.Sprintf("codex failed: %s", l.Err), false
}

// rejectedArgs finds each flag and each -c key of args that stderr mentions.
func rejectedArgs(args []string, stderr string) []string {
	var named []string
	for i, a := range args {
		name := a
		switch {
		case a == "-c" && i+1 < len(args):
			name, _, _ = strings.Cut(args[i+1], "=")
		case strings.HasPrefix(a, "-"):
		default:
			continue
		}
		if !slices.Contains(named, name) && strings.Contains(stderr, name) {
			named = append(named, name)
		}
	}
	return named
}

// checkOSC7501Title wants a state report from app=codex and a title the Codex
// harness can read. Real Codex titles are the cwd name or the prompt, never a
// session id.
func checkOSC7501Title(in Input) (string, bool) {
	var parser harness.ProgramStatusParser
	var raw []byte
	state := ""
	for _, f := range in.Frames {
		raw = append(raw, f.Data...)
		_, reports := parser.Feed(f.Data)
		for _, r := range reports {
			if r.App == "codex" && r.State != "" && state == "" {
				state = r.State
			}
		}
	}
	if state == "" {
		return "no OSC 7501 state= report from app=codex reached the PTY", false
	}
	name, ok := codexHarness().DisplayName(raw)
	if !ok {
		return "OSC 7501 arrived but no terminal title did", false
	}
	return fmt.Sprintf("state=%s, title %q", state, name), true
}

func checkHooksFire(in Input) (string, bool) {
	var missing []string
	for _, event := range []string{"SessionStart", "PreToolUse"} {
		if !slices.Contains(in.Hooks, event) {
			missing = append(missing, event)
		}
	}
	if len(missing) > 0 {
		return "hook never fired: " + strings.Join(missing, ", "), false
	}
	return "SessionStart and PreToolUse fired", true
}

func checkPermissionDeny(in Input) (string, bool) {
	if !slices.Contains(in.Hooks, "PermissionRequest") {
		return "PermissionRequest hook never fired", false
	}
	if !in.DenyHonoured {
		return "PermissionRequest fired but codex ignored the deny", false
	}
	return "PermissionRequest fired and the deny held", true
}

func checkProbeCommit(in Input) (string, bool) {
	if in.Probe.Err != "" {
		return "commit from a codex shell call failed: " + in.Probe.Err, false
	}
	return "commit landed from a linked worktree", true
}

// checkRolloutReader runs both readers over the rollouts the live sessions
// produced.
func checkRolloutReader(in Input) (string, bool) {
	if len(in.Rollouts) == 0 {
		return "the live sessions produced no rollout", false
	}
	for _, r := range in.Rollouts {
		if problem := CheckRollout(r.Data); problem != "" {
			return fmt.Sprintf("%s: %s", r.Name, problem), false
		}
		if _, err := codexsession.ReadChat(bytes.NewReader(r.Data)); err != nil {
			return fmt.Sprintf("%s: codexsession cannot read it: %v", r.Name, err), false
		}
	}
	return fmt.Sprintf("%d live rollouts read by both readers", len(in.Rollouts)), true
}

// The spawn model keys were found with `strings` on the 0.166.0 binary's
// ConfigToml: `hide_spawn_agent_metadata` and its sibling
// `expose_spawn_agent_model_overrides`, under [features.multi_agent_v2]. A line
// scan is enough; go.mod has no TOML library.
func checkSpawnModelFields(in Input) (string, bool) {
	data, err := os.ReadFile(filepath.Join(sessionlookup.RootsFrom(in.Env).CodexHome, "config.toml"))
	if err != nil {
		return "spawn model fields are visible", true
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if (key == "hide_spawn_agent_metadata" && value == "true") ||
			(key == "expose_spawn_agent_model_overrides" && value == "false") {
			return fmt.Sprintf("config.toml sets %s = %s, which hides the subagent model and effort fields: tier choice is ignored for subagents", key, value), false
		}
	}
	return "spawn model fields are visible", true
}

func checkManagedRules(in Input) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(sessionlookup.RootsFrom(in.Env).CodexHome, "rules"))
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				return "managed rules are installed", true
			}
		}
	}
	return "no managed rules under $CODEX_HOME/rules: run `gx codex setup`", false
}
