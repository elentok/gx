// Command gxfixture starts one fresh, isolated gx server for a named scenario
// and prints, as one line of JSON, how to reach it. It stops on SIGTERM and
// removes everything it made. It never reads or writes the real HOME, config
// or state: the server and every CLI verb run in a temp HOME and temp XDG dirs.
//
//	gxfixture [--gx <gx binary>] [--fakeclaude <binary>] <scenario>
//
// A binary left out is built with `go build`, which needs the module as the
// working directory.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
)

// Output is the JSON gxfixture prints once the server answers.
type Output struct {
	WebURL   string `json:"web_url"`
	StateDir string `json:"state_dir"`
	Socket   string `json:"socket"`
	HoldFile string `json:"hold_file"`
	// Env is what a CLI verb needs to reach this server and no other.
	Env []string `json:"env"`
	// Gx is the gx binary the server runs.
	Gx string `json:"gx"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gxfixture:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	var gxBin, fakeBin, name string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--gx", "--fakeclaude":
			if i+1 == len(args) {
				return fmt.Errorf("%s needs a value", args[i])
			}
			if args[i] == "--gx" {
				gxBin = args[i+1]
			} else {
				fakeBin = args[i+1]
			}
			i++
		default:
			name = args[i]
		}
	}
	sc, ok := scenarios[name]
	if !ok {
		return fmt.Errorf("unknown scenario %q; known: %s", name, strings.Join(scenarioNames(), ", "))
	}

	// Unix socket paths are capped near 100 bytes, and a holder's socket sits
	// deep in the state dir, so the root stays short: macOS's TMPDIR is too long.
	root, err := os.MkdirTemp("/tmp", "gxf")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if gxBin, err = binary(gxBin, filepath.Join(root, "gx"), "github.com/elentok/gx"); err != nil {
		return err
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if fakeBin, err = binary(fakeBin, filepath.Join(binDir, "claude"), "github.com/elentok/gx/testutil/agentfake/cmd/fakeclaude"); err != nil {
		return err
	}
	if filepath.Base(fakeBin) != "claude" || filepath.Dir(fakeBin) != binDir {
		if err := os.Symlink(fakeBin, filepath.Join(binDir, "claude")); err != nil {
			return err
		}
	}

	port, err := freePort()
	if err != nil {
		return err
	}
	f := &fixture{
		root:   root,
		gx:     gxBin,
		hold:   filepath.Join(root, "hold"),
		store:  filepath.Join(root, "data", "gx", "tickets"),
		stateD: filepath.Join(root, "state", "gx"),
	}
	f.env, f.cliEnv = isolatedEnv(root, binDir)
	if err := f.writeConfig(port); err != nil {
		return err
	}
	if err := f.seed(sc); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := exec.Command(gxBin, "server")
	srv.Env = f.env
	logFile, err := os.Create(filepath.Join(root, "server.out"))
	if err != nil {
		return err
	}
	defer logFile.Close()
	srv.Stdout, srv.Stderr = logFile, logFile
	if err := srv.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- srv.Wait() }()
	defer f.shutdown(srv, exited)

	webURL, err := f.waitReady(ctx, exited)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, f.serverOutput())
	}
	for _, addr := range sc.queue {
		if out, err := f.cli("server", "queue", "add", addr); err != nil {
			return fmt.Errorf("queue %s: %w\n%s", addr, err, out)
		}
	}
	out, err := json.Marshal(Output{WebURL: webURL, StateDir: f.stateD, Socket: server.SocketPath(f.stateD), HoldFile: f.hold, Env: f.cliEnv, Gx: gxBin})
	if err != nil {
		return err
	}
	fmt.Println(string(out))

	select {
	case <-ctx.Done():
		return nil
	case err := <-exited:
		return fmt.Errorf("server exited: %v\n%s", err, f.serverOutput())
	}
}

type fixture struct {
	root, gx, hold, store, stateD string
	// env is what the server and its agents run with; cliEnv is the part a
	// CLI verb needs to reach the same server, which is all the output names.
	env, cliEnv []string
}

// isolatedEnv is the process environment with every variable that could reach
// the real HOME, config, state or Claude config replaced, and the ones that
// make a nested agent act differently dropped.
func isolatedEnv(root, binDir string) (env, cliEnv []string) {
	drop := map[string]bool{"HOME": true, "PATH": true, "GIT_CONFIG_GLOBAL": true, "AI_AGENT": true}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] && !strings.HasPrefix(k, "XDG_") && !strings.HasPrefix(k, "GX_") &&
			!strings.HasPrefix(k, "CLAUDE") && !strings.HasPrefix(k, "HERDR_") {
			env = append(env, kv)
		}
	}
	cliEnv = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
	}
	env = append(env, cliEnv...)
	env = append(env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=gxfixture", "GIT_AUTHOR_EMAIL=gxfixture@example.com",
		"GIT_COMMITTER_NAME=gxfixture", "GIT_COMMITTER_EMAIL=gxfixture@example.com",
	)
	return env, cliEnv
}

func (f *fixture) writeConfig(port int) error {
	dir := filepath.Join(f.root, "home", ".config", "gx")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := fmt.Sprintf(`{"agent-runner":"pty","runner-wait":"100ms","server":{"watch-config":false,"web":{"listen":"127.0.0.1:%d"}}}`, port)
	return os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0o644)
}

// seed writes the scenario's project repo (carrying the fake worker's config,
// which the worktrees inherit) and its tickets.
func (f *fixture) seed(sc scenario) error {
	repo := filepath.Join(f.root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	cfg, err := json.Marshal(agentfake.WorkerConfig{Gx: f.gx, Env: f.cliEnv, Hold: f.hold, HoldTimeout: "30s", Behaviour: sc.behaviour})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(repo, agentfake.WorkerConfigFile), cfg, 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = repo, f.env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}
	for _, p := range sc.projects {
		if err := servertest.WriteEpic(f.store, p, ""); err != nil {
			return err
		}
	}
	for _, e := range sc.epics {
		if err := servertest.WriteEpic(f.store, e.project, e.name); err != nil {
			return err
		}
		addr := e.project + ":" + e.name
		_, isMap := sc.designSteps[addr]
		if isMap {
			ticketMD := filepath.Join(f.store, e.project, e.name, "ticket.md")
			if err := os.WriteFile(ticketMD, []byte("---\nstatus: open\nkind: map\n---\n"), 0o644); err != nil {
				return err
			}
		}
		for _, tk := range e.tickets {
			if err := servertest.WriteTicketFile(f.store, e.project, e.name, tk.id, tk.slug, tk.opts); err != nil {
				return err
			}
			if tk.opts.PauseReason != "" {
				ticketPath := filepath.Join(f.store, e.project, e.name, "issues", tk.id+"-"+tk.slug+".md")
				if _, err := tickets.AppendLog(ticketPath, tickets.LogEvent{Type: tickets.EventPauseQuestion, By: "gx", Summary: "Seeded pause: " + tk.opts.PauseReason}); err != nil {
					return err
				}
			}
		}
		if err := servertest.WriteProjectRepo(f.store, e.project, repo, `,"auto-ff-merge":true`); err != nil {
			return err
		}
		if isMap {
			if out, err := f.cli("tickets", "set", addr, "--design-step", sc.designSteps[addr]); err != nil {
				return fmt.Errorf("set design step of %s: %w\n%s", addr, err, out)
			}
		}
	}
	return nil
}

func (f *fixture) cli(args ...string) ([]byte, error) {
	cmd := exec.Command(f.gx, args...)
	cmd.Env = f.env
	return cmd.CombinedOutput()
}

// waitReady polls until the server answers with its web URL.
func (f *fixture) waitReady(ctx context.Context, exited <-chan error) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			return "", fmt.Errorf("server exited before it answered: %v", err)
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		out, err := f.cli("server", "status", "--json")
		if err != nil {
			continue
		}
		var st struct {
			Running bool   `json:"running"`
			WebURL  string `json:"web_url"`
		}
		if json.Unmarshal(out, &st) == nil && st.Running && st.WebURL != "" {
			return st.WebURL, nil
		}
	}
	return "", errors.New("server did not answer within 30s")
}

func (f *fixture) serverOutput() string {
	out, _ := os.ReadFile(filepath.Join(f.root, "server.out"))
	return string(out)
}

// shutdown stops the server, then any agent holder it left behind: holders
// detach, and their command lines name the temp root.
func (f *fixture) shutdown(srv *exec.Cmd, exited <-chan error) {
	_ = srv.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(15 * time.Second):
		_ = srv.Process.Kill()
		<-exited
	}
	_ = exec.Command("pkill", "-f", f.root).Run()
}

// binary returns given when set, and otherwise builds pkg into out.
func binary(given, out, pkg string) (string, error) {
	if given != "" {
		return filepath.Abs(given)
	}
	if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
	}
	return out, nil
}
