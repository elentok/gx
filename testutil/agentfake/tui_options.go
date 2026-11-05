package agentfake

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/elentok/gx/transcript"
)

// stallTurnLength is how long a BehaviourStall turn stays busy.
const stallTurnLength = 1500 * time.Millisecond

// TUIClaude runs the fake interactive claude on the process's terminal and
// exits with the code an exit control asked for.
func TUIClaude() {
	code, err := runTUIClaude()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

type tuiOptions struct {
	sessionID string
	trust     bool
	dropKey   bool
	noPidFile bool
	mouse     bool
	// draft seeds the input; a non-empty one also styles the input row.
	draft string
	// clearLeavesI makes the first Ctrl+U leave "i" (see TUIClearLeavesIEnv).
	clearLeavesI bool
	// registryDelay, unknownSkill, noTurn and launchRecord are the skill-prompt
	// knobs; see the TUI...Env consts.
	registryDelay time.Duration
	unknownSkill  bool
	noTurn        bool
	launchRecord  bool
	// readySignal is false only for TUIReadyOff (see TUIReadyDelayEnv).
	readySignal bool
	readyDelay  time.Duration
	// model is --model, reported on every reply.
	model string
	// settings is --settings: inline JSON or a path to a JSON file.
	settings string
}

// parseTUIArgs accepts exactly the PTY runner's launch flags, so a new flag
// fails the conformance suite instead of passing unseen.
func parseTUIArgs(args []string) (tuiOptions, error) {
	fs := flag.NewFlagSet("claude", flag.ContinueOnError)
	sessionID := fs.String("session-id", "", "")
	resume := fs.String("resume", "", "")
	fs.String("permission-mode", "", "")
	// Claude merges repeated --settings; the fake only needs the statusLine one.
	var settings string
	fs.Func("settings", "", func(v string) error {
		if settings == "" || statusLineCommand(v) != "" {
			settings = v
		}
		return nil
	})
	fs.String("disallowedTools", "", "")
	fs.String("append-system-prompt", "", "")
	fs.String("append-system-prompt-file", "", "")
	model := fs.String("model", "", "")
	fs.String("effort", "", "")
	fs.String("remote-control", "", "")
	if err := fs.Parse(args); err != nil {
		return tuiOptions{}, err
	}
	// Real claude fails with both unless --fork-session; --resume keeps the id.
	if *sessionID != "" && *resume != "" {
		return tuiOptions{}, errors.New("agentfake: --session-id and --resume are exclusive")
	}
	if *sessionID == "" {
		*sessionID = *resume
	}
	if *sessionID == "" {
		return tuiOptions{}, errors.New("agentfake: --session-id or --resume is required")
	}
	registryDelay, _ := time.ParseDuration(os.Getenv(TUIRegistryDelayEnv))
	readyDelay, _ := time.ParseDuration(os.Getenv(TUIReadyDelayEnv))
	return tuiOptions{
		readySignal:   os.Getenv(TUIReadyDelayEnv) != TUIReadyOff,
		readyDelay:    readyDelay,
		sessionID:     *sessionID,
		trust:         os.Getenv(TUITrustEnv) == "1",
		dropKey:       os.Getenv(TUITrustDropKeyEnv) == "1",
		noPidFile:     os.Getenv(TUINoPidFileEnv) == "1",
		mouse:         os.Getenv(TUIMouseEnv) == "1",
		draft:         os.Getenv(TUIDraftEnv),
		clearLeavesI:  os.Getenv(TUIClearLeavesIEnv) == "1",
		registryDelay: registryDelay,
		unknownSkill:  os.Getenv(TUIUnknownSkillEnv) == "1",
		noTurn:        os.Getenv(TUINoTurnEnv) == "1",
		launchRecord:  os.Getenv(TUILaunchRecordEnv) != "0",
		model:         cmp.Or(*model, ReportedModel),
		settings:      settings,
	}, nil
}

func runTUIClaude() (int, error) {
	if isClaudeProbe(os.Args[1:]) {
		return runClaudeProbe()
	}
	opts, err := parseTUIArgs(os.Args[1:])
	if err != nil {
		return 0, err
	}
	// Claude reads its terminal raw; a cooked one would hold a lone Esc
	// until the next Enter.
	if term.IsTerminal(os.Stdin.Fd()) {
		if _, err := term.MakeRaw(os.Stdin.Fd()); err != nil {
			return 0, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	ctlPath := TUIControlPath(home, opts.sessionID)
	if err := os.MkdirAll(filepath.Dir(ctlPath), 0o755); err != nil {
		return 0, err
	}
	if err := syscall.Mkfifo(ctlPath, 0o600); err != nil {
		return 0, err
	}
	defer os.Remove(ctlPath)
	// Read-write so the FIFO never reads EOF between two senders.
	ctl, err := os.OpenFile(ctlPath, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer ctl.Close()
	in, err := os.Create(TUIInputPath(home, opts.sessionID))
	if err != nil {
		return 0, err
	}
	defer in.Close()
	ack, err := os.Create(TUIAckPath(home, opts.sessionID))
	if err != nil {
		return 0, err
	}
	defer ack.Close()
	argv, err := json.Marshal(os.Args[1:])
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(TUIArgsPath(home, opts.sessionID), argv, 0o644); err != nil {
		return 0, err
	}
	if opts.model == StaleModel {
		return 0, fmt.Errorf("agentfake: model %q is not available", opts.model)
	}

	now := time.Now()
	t := &tui{
		out:        os.Stdout,
		ack:        ack,
		opts:       opts,
		started:    now,
		pidPath:    pidPathFor(home, os.Getpid()),
		ctlPath:    ctlPath,
		transcript: transcript.PathIn(home, cwd, opts.sessionID),
		pid: PidFile{
			Pid:        os.Getpid(),
			SessionID:  opts.sessionID,
			Cwd:        cwd,
			StartedAt:  now.UnixMilli(),
			ProcStart:  now.Format(time.ANSIC),
			Version:    "2.1.296",
			Kind:       "interactive",
			Entrypoint: "cli",
		},
	}
	t.typed.WriteString(opts.draft)
	return t.run(io.TeeReader(os.Stdin, in), ctl)
}
