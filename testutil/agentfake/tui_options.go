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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
)

// outageTurns parses TUIClassifierOutageEnv: a count of failing turns, 0 when unset or bad.
func outageTurns(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

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
	// clearIgnored makes every Ctrl+U a no-op (see TUIClearIgnoredEnv).
	clearIgnored bool
	// redrawDelay holds back the redraw after a typed key or clear (see TUIRedrawDelayEnv).
	redrawDelay time.Duration
	// hung makes Enter a no-op as well (see TUIHangLaunchesEnv).
	hung bool
	// registryDelay, unknownSkill, noTurn and launchRecord are the skill-prompt
	// knobs; see the TUI...Env consts.
	registryDelay time.Duration
	unknownSkill  bool
	noTurn        bool
	launchRecord  bool
	// plainScreen draws the "> " screen instead of the input box (see TUIPlainScreenEnv).
	plainScreen bool
	// classifierOutage is how many more turns fail (see TUIClassifierOutageEnv).
	classifierOutage int
	// model is --model, reported on every reply.
	model string
	// settings is --settings: inline JSON or a path to a JSON file.
	settings string
}

// HangThisLaunch counts this launch in the counter file of a
// TUIHangLaunchesEnv spec and reports whether it is within the first n. The
// Claude and Codex fakes share the spec.
func HangThisLaunch(spec string) bool {
	n, file, ok := strings.Cut(spec, ":")
	limit, err := strconv.Atoi(n)
	if !ok || err != nil {
		return false
	}
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return false
	}
	defer f.Close()
	prior, _ := f.Seek(0, io.SeekEnd)
	_, _ = f.WriteString("x")
	return int(prior) < limit
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
	redrawDelay, _ := time.ParseDuration(os.Getenv(TUIRedrawDelayEnv))
	hung := HangThisLaunch(os.Getenv(TUIHangLaunchesEnv))
	return tuiOptions{
		hung:             hung,
		plainScreen:      os.Getenv(TUIPlainScreenEnv) == "1",
		classifierOutage: outageTurns(os.Getenv(TUIClassifierOutageEnv)),
		sessionID:        *sessionID,
		trust:            os.Getenv(TUITrustEnv) == "1",
		dropKey:          os.Getenv(TUITrustDropKeyEnv) == "1",
		noPidFile:        os.Getenv(TUINoPidFileEnv) == "1",
		mouse:            os.Getenv(TUIMouseEnv) == "1",
		draft:            os.Getenv(TUIDraftEnv),
		clearLeavesI:     os.Getenv(TUIClearLeavesIEnv) == "1",
		clearIgnored:     hung || os.Getenv(TUIClearIgnoredEnv) == "1",
		redrawDelay:      redrawDelay,
		registryDelay:    registryDelay,
		unknownSkill:     os.Getenv(TUIUnknownSkillEnv) == "1",
		noTurn:           os.Getenv(TUINoTurnEnv) == "1",
		launchRecord:     os.Getenv(TUILaunchRecordEnv) != "0",
		model:            cmp.Or(*model, ReportedModel),
		settings:         settings,
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
		transcript: tuiTranscriptPath(home, cwd, opts.sessionID),
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
