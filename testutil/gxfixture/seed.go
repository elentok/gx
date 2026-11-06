package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/conversations"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// writeConfig writes the fixture's config. overrides are `dotted.key=<json>`
// entries (a value that is not JSON is a string) set over the defaults.
func (f *fixture) writeConfig(port, recentlyFinished int, overrides []string) error {
	dir := filepath.Join(f.root, "home", ".config", "gx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	server := map[string]any{"watch-config": false, "web": map[string]any{"listen": fmt.Sprintf("127.0.0.1:%d", port)}}
	if recentlyFinished > 0 {
		server["recently-finished"] = recentlyFinished
	}
	// A wait shorter than a loaded fake's first turn reads as an undelivered
	// prompt: the runner kills the agent mid-finish and relaunches it.
	cfg := map[string]any{"agent-runner": "pty", "runner-wait": "2s", "server": server}
	for _, kv := range overrides {
		key, raw, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			return fmt.Errorf("--config %q: want key=value", kv)
		}
		var val any
		if json.Unmarshal([]byte(raw), &val) != nil {
			val = raw
		}
		node := cfg
		parts := strings.Split(key, ".")
		for _, p := range parts[:len(parts)-1] {
			next, ok := node[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[p] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = val
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

// seed writes the scenario's project repo (carrying the fake worker's config,
// which the worktrees inherit) and its tickets.
func (f *fixture) seed(sc scenario) error {
	repo := filepath.Join(f.root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	holdTimeout := "30s"
	if sc.Hold {
		holdTimeout = "1h"
	}
	wcfg := agentfake.WorkerConfig{Gx: f.gx, Env: f.cliEnv, Hold: f.hold, HoldTimeout: holdTimeout, Behaviour: sc.Behaviour, Design: sc.Design, DesignByMap: sc.DesignByMap, Chat: sc.Chat, Transcript: sc.Transcripts}
	if sc.Usage {
		resets := time.Now().Add(72 * time.Hour).Unix()
		wcfg.Usage = &agentfake.UsageConfig{
			FiveHour: agentfake.UsageWindow{UsedPercentage: 10, ResetsAt: resets},
			SevenDay: agentfake.UsageWindow{UsedPercentage: 99, ResetsAt: resets},
		}
	}
	if sc.RateLimitFromUsage {
		// Long enough for the flow to see the hold, short enough to wait it out; the fake sets
		// the window's reset when it hits the limit.
		wcfg.RateLimitFromUsage = true
		wcfg.RateLimitReset = "8s"
		wcfg.Usage = &agentfake.UsageConfig{FiveHour: agentfake.UsageWindow{UsedPercentage: 100}}
	}
	if sc.CodexUsage {
		wcfg.CodexUsage = &agentfake.UsageWindow{UsedPercentage: 99, ResetsAt: time.Now().Add(72 * time.Hour).Unix()}
	}
	cfg, err := json.Marshal(wcfg)
	if err != nil {
		return err
	}
	// ownRepo makes a project's own repo; with the chat knob it carries the
	// worker config too, since a session reads the config from its dir.
	ownRepo := func(p string) (string, error) {
		dir := filepath.Join(f.root, p+"-repo")
		if err := f.initRepo(dir); err != nil {
			return "", err
		}
		if sc.Chat {
			return dir, os.WriteFile(filepath.Join(dir, agentfake.WorkerConfigFile), cfg, 0o644)
		}
		return dir, nil
	}
	if err := os.WriteFile(filepath.Join(repo, agentfake.WorkerConfigFile), cfg, 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if err := f.git(repo, args...); err != nil {
			return err
		}
	}
	if sc.GitSetup != nil {
		if err := sc.GitSetup(f.git, repo); err != nil {
			return err
		}
	}
	if sc.Conversations != nil {
		dir := filepath.Join(f.root, "data", "gx", "conversations")
		started, recs := sc.Conversations(time.Now())
		if err := conversations.MarkStarted(dir, started); err != nil {
			return err
		}
		for _, r := range recs {
			if err := conversations.Save(dir, r); err != nil {
				return err
			}
		}
	}
	for _, p := range sc.Projects {
		if err := servertest.WriteEpic(f.store, p, ""); err != nil {
			return err
		}
		if !sc.NoCheckConfig {
			body := `{"name":"` + p + `","checks":{}}`
			if err := os.WriteFile(filepath.Join(f.store, p, "project.json"), []byte(body), 0o644); err != nil {
				return err
			}
		}
		if slices.Contains(sc.OwnRepos, p) {
			dir, err := ownRepo(p)
			if err != nil {
				return err
			}
			extra := ""
			if !sc.NoCheckConfig {
				extra = `,"checks":{}`
			}
			if err := servertest.WriteProjectRepo(f.store, p, dir, extra); err != nil {
				return err
			}
		}
	}
	for _, e := range sc.Epics {
		if err := servertest.WriteEpic(f.store, e.Project, e.Name); err != nil {
			return err
		}
		addr := e.Project + ":" + e.Name
		_, isMap := sc.DesignSteps[addr]
		if isMap {
			ticketMD := filepath.Join(f.store, e.Project, e.Name, "ticket.md")
			if err := os.WriteFile(ticketMD, []byte("---\nstatus: open\nkind: map\n---\n"), 0o644); err != nil {
				return err
			}
		}
		if meta, ok := sc.EpicMeta[addr]; ok {
			ticketMD := filepath.Join(f.store, e.Project, e.Name, "ticket.md")
			if err := os.WriteFile(ticketMD, []byte("---\n"+meta+"---\n"), 0o644); err != nil {
				return err
			}
		}
		if kind, ok := sc.Recovered[addr]; ok {
			ev := runlog.Event{Time: time.Now(), Type: string(events.RecoveryApplied), Kind: kind, Reason: "seeded", Outcome: "ok"}
			if err := runlog.AppendEvent(filepath.Join(f.store, e.Project), e.Name, ev); err != nil {
				return err
			}
		}
		for _, tk := range e.Tickets {
			if err := servertest.WriteTicketFile(f.store, e.Project, e.Name, tk.ID, tk.Slug, tk.Opts); err != nil {
				return err
			}
			if tk.Opts.PauseReason != "" {
				ticketPath := filepath.Join(f.store, e.Project, e.Name, "issues", tk.ID+"-"+tk.Slug+".md")
				if err := seedPauseRound(ticketPath, tk.Opts.PauseReason, sc.Recommend[addr+"/"+tk.ID]); err != nil {
					return err
				}
			}
		}
		extra := sc.ProjectExtra[e.Project]
		if sc.DiskLow {
			argv, err := json.Marshal([]string{f.cleanScript()})
			if err != nil {
				return err
			}
			extra += `,"disk-space":{"cleanup-command":` + string(argv) + `}`
		}
		if !slices.Contains(sc.ManualMerge, e.Project) {
			extra += `,"auto-ff-merge":true`
		}
		if !sc.NoCheckConfig {
			extra += `,"checks":{}`
		}
		projectRepo := repo
		if slices.Contains(sc.OwnRepos, e.Project) {
			if projectRepo, err = ownRepo(e.Project); err != nil {
				return err
			}
		}
		if err := servertest.WriteProjectRepo(f.store, e.Project, projectRepo, extra); err != nil {
			return err
		}
		if isMap {
			if out, err := f.cli("tickets", "set", addr, "--design-step", sc.DesignSteps[addr]); err != nil {
				return fmt.Errorf("set design step of %s: %w\n%s", addr, err, out)
			}
		}
	}
	return nil
}

// initRepo makes a git repo in dir with one commit.
func (f *fixture) initRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+filepath.Base(dir)+"\n"), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if err := f.git(dir, args...); err != nil {
			return err
		}
	}
	return nil
}

// seedPauseRound opens the round a real pause of reason opens: a question only
// where the reply goes to the agent, else a repair round. A recommendation is
// stored on it as an investigate agent would.
func seedPauseRound(ticketPath, reason string, rec recommendation) error {
	summary := "Seeded pause: " + reason
	ev := tickets.LogEvent{Type: tickets.EventPauseRepair, By: "gx", PauseKind: reason, Reason: summary}
	if schema.PauseReason(reason).AsksQuestion() {
		ev = tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: summary}
	}
	if _, err := tickets.AppendLog(ticketPath, ev); err != nil {
		return err
	}
	if rec.Verb == "" {
		return nil
	}
	_, err := tickets.RecommendOpenRound(ticketPath, rec.Verb, rec.Diagnosis)
	return err
}

// git runs a git command in dir with the fixture's identity.
func (f *fixture) git(dir string, args ...string) error {
	_, _, err := git.Run(dir, f.env, args...)
	return err
}

func (f *fixture) cli(args ...string) ([]byte, error) {
	cmd := exec.Command(f.gx, args...)
	cmd.Env = f.env
	return cmd.CombinedOutput()
}

// linkSession marks the session id a design session of the map at addr.
func (f *fixture) linkSession(id, addr string) error {
	cmd := exec.Command(f.gx, "agent", "link", addr)
	cmd.Env = append(slices.Clone(f.env), holder.AgentSessionEnv+"="+id)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("link %s to %s: %w\n%s", id, addr, err, out)
	}
	return nil
}
