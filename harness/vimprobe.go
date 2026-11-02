package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	vimSettle     = 500 * time.Millisecond
	vimSettlePoll = 50 * time.Millisecond
	backspaceKey  = "\x7f"
)

// vimSessions is the probe's per-session memory, keyed by TypingIO.Session. The
// harness value is rebuilt on every lookup, so the memory lives here.
var vimSessions = struct {
	sync.Mutex
	m map[string]*vimSession
}{m: map[string]*vimSession{}}

type vimSession struct {
	// overrideFiles is the editor-mode files' editorMode values at launch; set when the
	// launch forced normal mode.
	overrideFiles string
	overridden    bool
	// toggled latches once the files changed to vim after launch.
	toggled bool
	iPress  int
}

func sessionState(id string) *vimSession {
	s := vimSessions.m[id]
	if s == nil {
		s = &vimSession{}
		vimSessions.m[id] = s
	}
	return s
}

// markEditorOverride records that session was launched with editorMode normal,
// and what the editor-mode files held then, so a later change reads as a
// /config toggle.
func markEditorOverride(session string, env []string) {
	if session == "" {
		return
	}
	vimSessions.Lock()
	defer vimSessions.Unlock()
	s := sessionState(session)
	s.overridden, s.overrideFiles = true, editorFilesContent(env)
}

// NoteEditorOverride records that session, launched by an earlier server, was
// launched with editorMode normal. Its snapshot is of the files now, so a
// /config toggle made before the server restarted is not seen. env is the
// claude config env the probe reads, nil for the process's own.
func NoteEditorOverride(session string, env []string) { markEditorOverride(session, env) }

// EditorOverrideApplied reports whether session was launched with the normal
// editor mode override.
func EditorOverrideApplied(session string) bool {
	vimSessions.Lock()
	defer vimSessions.Unlock()
	return vimSessions.m[session] != nil && vimSessions.m[session].overridden
}

// InputPresses is how many `i` keys the probe pressed in session.
func InputPresses(session string) int {
	vimSessions.Lock()
	defer vimSessions.Unlock()
	if s := vimSessions.m[session]; s != nil {
		return s.iPress
	}
	return 0
}

// InputText is the plain text of the claude input box in styled or plain rows;
// false means no input box was found.
func InputText(rows []string) (string, bool) { return inputText(rows) }

// RecordInputPress counts an `i` gx pressed in session. The probe calls it; a
// test calls it to stand for a stray `i` the probe left.
func RecordInputPress(session string) { recordIPress(session) }

func recordIPress(session string) {
	vimSessions.Lock()
	defer vimSessions.Unlock()
	sessionState(session).iPress++
}

// probeWanted says whether the effective editor mode could be vim. Under the
// launch override only a toggle seen after launch counts; the person's own
// files saying vim from before launch do not.
func probeWanted(session string, env []string, io TypingIO) bool {
	vim := claudeVimMode(env)
	vimSessions.Lock()
	defer vimSessions.Unlock()
	s := vimSessions.m[session]
	if s == nil || !s.overridden {
		return vim
	}
	if !vim {
		s.toggled = false
		return false
	}
	if !s.toggled && editorFilesContent(env) != s.overrideFiles {
		s.toggled = true
		io.log("editor mode toggled to vim after launch, probing")
	}
	return s.toggled
}

// editorFilesContent is the editorMode each config file holds, nothing else:
// claude rewrites ~/.claude.json all the time, so its whole content changing is
// no sign of a /config toggle.
func editorFilesContent(env []string) string {
	configDir, globalDir, err := claudeConfigDirs(env)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for _, path := range []string{filepath.Join(globalDir, ".claude.json"), filepath.Join(configDir, "settings.json")} {
		var cfg struct {
			EditorMode string `json:"editorMode"`
		}
		if data, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(data, &cfg)
		}
		sb.WriteString(cfg.EditorMode)
		sb.WriteByte(0)
	}
	return sb.String()
}

// inputText is the plain text of the input box: the last input row after its
// marker plus the indented rows that continue it, trailing blanks trimmed.
func inputText(rows []string) (string, bool) {
	start := lastInputRow(rows, claudeInput)
	if start < 0 {
		return "", false
	}
	// The placeholder is not text: typing replaces it, so it must not count as
	// a before-state the typed key "changed".
	if inputState(rows, claudeInput) == InputEmpty {
		return "", true
	}
	cells := parseCells(rows[start])
	var sb strings.Builder
	for _, c := range cells[1:] {
		sb.WriteRune(c.r)
	}
	for _, row := range rows[start+1:] {
		p := plain(row)
		if !strings.HasPrefix(p, "  ") {
			break
		}
		sb.WriteByte('\n')
		sb.WriteString(p)
	}
	return strings.Trim(strings.ReplaceAll(sb.String(), " ", " "), " \n"), true
}

func footerHasInsert(rows []string) bool {
	input := lastInputRow(rows, claudeInput)
	if input < 0 {
		return false
	}
	for _, row := range rows[input+1:] {
		if strings.Contains(plain(row), vimInsertMarker) {
			return true
		}
	}
	return false
}

// settledRows reads until two reads in a row agree, so a stale or transient
// frame never decides. ok is false when the screen never settled or has no
// input box.
func settledRows(io TypingIO) (rows []string, text string, ok bool) {
	var prev string
	havePrev := false
	for deadline := time.Now().Add(vimSettle); ; {
		r, err := io.Rows()
		if err == nil {
			if t, found := inputText(r); found {
				key := strings.Join(r, "\n")
				if havePrev && key == prev {
					return r, t, true
				}
				prev, havePrev = key, true
			} else {
				havePrev = false
			}
		}
		if !time.Now().Before(deadline) {
			return nil, "", false
		}
		time.Sleep(vimSettlePoll)
	}
}

// grewByI reports whether after is before with one `i` inserted anywhere.
func grewByI(before, after string) bool {
	if len(after) != len(before)+1 {
		return false
	}
	for k := 0; k < len(after); k++ {
		if after[k] == 'i' && after[:k]+after[k+1:] == before {
			return true
		}
	}
	return false
}

// probeVim finds out by effect whether claude's input is in INSERT, leaving
// the input as it found it. false means unknown: the caller must not type.
func probeVim(session string, io TypingIO) bool {
	rows, before, ok := settledRows(io)
	if !ok {
		io.logUnknown("input never settled before the probe")
		return false
	}
	if footerHasInsert(rows) {
		io.log("INSERT marker shown, no key needed")
		return true
	}
	if err := io.SendKeys("i"); err != nil {
		io.log("could not press i: " + err.Error())
		return false
	}
	recordIPress(session)
	rows, after, ok := settledRows(io)
	switch {
	case !ok:
		io.logUnknown("input never settled after pressing i")
		return false
	case footerHasInsert(rows) && !grewByI(before, after):
		// The marker appearing is the proof: the box was not in INSERT before, so
		// the `i` was a command, not text.
		io.log("was NORMAL, now INSERT")
		return true
	case grewByI(before, after):
		if err := io.SendKeys(backspaceKey); err != nil {
			io.log("could not remove the stray i: " + err.Error())
			return false
		}
		io.log("was INSERT without a marker, removed the i")
		return true
	case after != before:
		io.logUnknown("input changed by more than the i, person input assumed")
		return false
	}
	io.logUnknown("neither marker nor input change after pressing i")
	return false
}
