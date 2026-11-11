package agentfake

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The pieces below are shared by the program fake and the claude TUI fake
// (package tuiclaude): the TUI fake imports this package, never the reverse.

const (
	BracketedPasteOn = "\x1b[?2004h"
	PasteStart       = "\x1b[200~"
	PasteEnd         = "\x1b[201~"
)

const EscKey = 0x1b

const (
	CtrlC = 0x03
	CtrlU = 0x15
)

// TuiEvent is one key from the terminal, one control, or the end of input.
type TuiEvent struct {
	Key     byte
	Control *Control
	Resized bool
	Err     error
}

func PtyClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO)
}

// TuiEvents merges the keys read from in with the controls read from ctl.
func TuiEvents(in io.Reader, ctl io.Reader) <-chan TuiEvent {
	events := make(chan TuiEvent)
	go func() {
		r := bufio.NewReader(in)
		for {
			b, err := r.ReadByte()
			if err != nil {
				events <- TuiEvent{Err: err}
				return
			}
			events <- TuiEvent{Key: b}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(ctl)
		for scanner.Scan() {
			var c Control
			if err := json.Unmarshal(scanner.Bytes(), &c); err != nil {
				events <- TuiEvent{Err: fmt.Errorf("bad control line %q: %w", scanner.Text(), err)}
				return
			}
			events <- TuiEvent{Control: &c}
		}
	}()
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			events <- TuiEvent{Resized: true}
		}
	}()
	return events
}

// SendControlWhenReady writes c to the side-channel FIFO that ready reports,
// waiting until ready says the fake has it open. A FIFO that merely exists has
// no reader yet, and a write to it fails with ENXIO.
func SendControlWhenReady(ready func() (path string, ok bool), c Control) error {
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		path, ok := ready()
		if ok {
			return SendControl(path, c)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("agentfake: %s never opened", path)
		}
	}
}

// AskExitSuffix names the file beside an AskCall's OutFile that holds the hook's exit code.
const AskExitSuffix = ".exit"

// AskCall is the AskUserQuestion call an ActionAskQuestion runs the hook for.
type AskCall struct {
	ToolUseID string        `json:"tool_use_id"`
	Questions []AskQuestion `json:"questions"`
	// OutFile, when set, receives the hook's stdout, so a test reads its result.
	// The hook's exit code lands in OutFile+AskExitSuffix once it ends.
	OutFile string `json:"out_file,omitempty"`
	// TimeoutMs, when set, stands in for claude's hook timeout: the hook is
	// killed after it. Dialog, when set, is the screen the fake shows on
	// "input needed" once the hook ends without an answer (timeout or release).
	TimeoutMs int      `json:"timeout_ms,omitempty"`
	Dialog    []string `json:"dialog,omitempty"`
}

// AskQuestion is one question of an AskCall, in claude's own field names.
type AskQuestion struct {
	Header      string      `json:"header"`
	Question    string      `json:"question"`
	MultiSelect bool        `json:"multiSelect"`
	Options     []AskOption `json:"options"`
}

// AskOption is one choice of an AskQuestion.
type AskOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// AskScreen is an AskUserQuestion dialog with no hook: the TUI fake draws the
// real layouts, takes the keys claude takes (arrows, digits, Enter, Esc and
// typed text) and writes what it did to files, so a test can read the keys.
type AskScreen struct {
	Questions []AskQuestion `json:"questions"`
	// KeysFile receives one line per key read: "down", "up", "enter", "esc", or
	// the character typed.
	KeysFile string `json:"keys_file,omitempty"`
	// AnswersFile receives the answers as JSON once the dialog ends: an object
	// keyed by question, or the string "cancelled".
	AnswersFile string `json:"answers_file,omitempty"`
	// MutateAfterKeys, when above zero, rewrites the question once that many
	// keys have been read, as a dialog closed and reopened under a person.
	MutateAfterKeys int `json:"mutate_after_keys,omitempty"`
}

type PendingWork struct {
	Cfg     WorkerConfig
	Address string
}

// DesignState is set once a /gx-design prompt arrives; rounds counts the ones
// played.
type DesignState struct {
	Active bool
	rounds int
	Turns  int
	// script is the turn script of the map this session designs, from
	// WorkerConfig.DesignByMap; nil falls back to WorkerConfig.Design.
	script []string
}

// IsAsk reports whether the behaviour's turn is a single gx ask call.
func IsAsk(behaviour string) bool {
	switch behaviour {
	case BehaviourAsk, BehaviourAskOptions, BehaviourWriteAsk, BehaviourAskEmptyFinish:
		return true
	}
	return false
}

// StatusLinePayload is the JSON claude sends the statusLine command. Without
// usage there is no rate_limits key.
func StatusLinePayload(model string, usage *UsageConfig) ([]byte, error) {
	payload := map[string]any{
		"model": map[string]any{"display_name": model},
	}
	if usage != nil {
		payload["rate_limits"] = usage
	}
	return json.Marshal(payload)
}

// ToolCall is a tool call Claude has started: its name and input.
type ToolCall struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input,omitempty"`
}

// ActionExit makes the TUI fake exit with Control.Code.
const ActionExit = "exit"

// ActionDescendants makes the TUI fake spawn a tree of long-lived processes
// under itself, to stage a fork storm without a real runaway: Control.Count
// sleepers at each of Control.Depth shell levels (Depth 0 or 1 is one level).
// The tree is one process group, killed by ActionStopDescendants and when the
// fake exits; a watcher inside it also ends the tree if the fake is killed
// outright.
const ActionDescendants = "descendants"

// ActionStopDescendants kills the tree an ActionDescendants started.
const ActionStopDescendants = "stopdescendants"

// SleeperCommand is the command line of each process of a descendants tree, as
// ps shows it, for a test that counts them.
const SleeperCommand = "sleep 100000"

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// descendantLevel is the script of one shell level: count sleepers and, below
// the top level, the next level's shell.
func descendantLevel(count, depth int) string {
	var b strings.Builder
	for range count {
		b.WriteString(SleeperCommand + " & ")
	}
	if depth > 1 {
		b.WriteString("sh -c ")
		b.WriteString(shellQuote(descendantLevel(count, depth-1)))
		b.WriteString(" & ")
	}
	b.WriteString("wait")
	return b.String()
}

// SpawnDescendants starts the tree under the calling process; it ends when
// the caller does. A fixture uses it for a tree that gx did not launch.
func SpawnDescendants(count, depth int) (pid int, err error) {
	var cmd *exec.Cmd
	if err := SpawnDescendantsInto(&cmd, count, depth); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// SpawnDescendantsInto starts the tree into *slot, which the fake keeps so it
// can stop the tree later.
func SpawnDescendantsInto(slot **exec.Cmd, count, depth int) error {
	if count <= 0 {
		return fmt.Errorf("descendants control needs a positive count, got %d", count)
	}
	if *slot != nil {
		return fmt.Errorf("descendants already running")
	}
	script := "sh -c " + shellQuote(descendantLevel(count, depth)) + " & " +
		"while kill -0 " + strconv.Itoa(os.Getpid()) + " 2>/dev/null; do sleep 1; done; kill 0"
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	*slot = cmd
	return nil
}

func StopDescendantsIn(slot **exec.Cmd) {
	if *slot == nil {
		return
	}
	_ = syscall.Kill(-(*slot).Process.Pid, syscall.SIGKILL)
	_ = (*slot).Wait()
	*slot = nil
}

var imageMediaTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// ImageMediaType is the media type of an image path's extension, empty for a
// non-image.
func ImageMediaType(path string) string {
	return imageMediaTypes[strings.ToLower(filepath.Ext(path))]
}

// IsImageFile reports whether path is an absolute path to an existing PNG,
// JPEG, GIF or WebP file.
func IsImageFile(path string) bool {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n") {
		return false
	}
	if ImageMediaType(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
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
