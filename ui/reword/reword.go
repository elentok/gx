package reword

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

// EditorFinishedMsg is returned by tea.ExecProcess when the editor closes.
type EditorFinishedMsg struct {
	Err error
}

// Result describes what happened during a single Update call.
type Result struct {
	Done bool
	Err  error
}

type execStep struct {
	components.Step
	run func() (string, error)
}

type stepResultMsg struct {
	idx int
	err error
}

// Model owns the running phase of a reword operation (no confirm dialog).
type Model struct {
	IsOpen     bool
	Hash       string
	Subject    string
	NewSubject string

	tmpFile string
	origMsg string

	running bool
	steps   []execStep
	stepIdx int
	spinner spinner.Model
	root    string
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
}

// CmdOpenEditor writes a temp file pre-populated with the commit message and returns a
// tea.ExecProcess cmd that opens $EDITOR on it. Stores hash, subject, tmpFile, and
// original message on the model. Call ReadEditorResult after EditorFinishedMsg.
func (m *Model) CmdOpenEditor(root, hash, subject, body string, pushed bool) (tea.Cmd, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return nil, fmt.Errorf("$EDITOR is not set")
	}

	original := subject
	if body != "" {
		original = subject + "\n\n" + body
	}

	content := original + "\n"
	if pushed {
		content += "\n# warning: this commit has been pushed to origin\n"
	}

	f, err := os.CreateTemp("", "gx-reword-*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpFile := f.Name()
	if _, err = f.WriteString(content); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	f.Close()

	m.Hash = hash
	m.Subject = subject
	m.tmpFile = tmpFile
	m.origMsg = original
	m.NewSubject = ""

	parts := strings.Fields(editor)
	args := append(parts[1:], tmpFile)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(e error) tea.Msg {
		return EditorFinishedMsg{Err: e}
	}), nil
}

// ReadEditorResult reads the temp file written by CmdOpenEditor, strips comment lines,
// and compares to the original message. Returns changed=false if empty or unchanged.
func (m *Model) ReadEditorResult() (changed bool, newMsg string, err error) {
	data, err := os.ReadFile(m.tmpFile)
	if err != nil {
		return false, "", fmt.Errorf("read temp file: %w", err)
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}

	result := strings.TrimRight(strings.Join(lines, "\n"), "\n\r\t ")
	normalizedOriginal := strings.TrimRight(m.origMsg, "\n\r\t ")

	if result == "" || result == normalizedOriginal {
		return false, m.origMsg, nil
	}
	return true, result, nil
}

// StartRunning sets up the running steps and returns the first cmd.
// Hash and Subject must already be set (via CmdOpenEditor or directly).
func (m *Model) StartRunning(root, newMsg string) (tea.Cmd, error) {
	isHead, err := git.IsHEAD(root, m.Hash)
	if err != nil {
		return nil, err
	}

	hash := m.Hash
	var steps []execStep
	if isHead {
		steps = []execStep{{
			Step: components.Step{
				TitleBefore:  "reword HEAD",
				RunningTitle: "rewording HEAD...",
				TitleAfter:   "rewrote HEAD",
				TitleFailed:  "reword failed",
			},
			run: func() (string, error) { return git.RewordHead(root, newMsg) },
		}}
	} else {
		steps = []execStep{{
			Step: components.Step{
				TitleBefore:  "reword commit",
				RunningTitle: "rewording commit...",
				TitleAfter:   "rewrote commit",
				TitleFailed:  "reword failed",
			},
			run: func() (string, error) { return git.RewordCommit(root, hash, newMsg) },
		}}
	}

	m.NewSubject = strings.SplitN(newMsg, "\n", 2)[0]
	m.root = root
	m.steps = steps
	m.stepIdx = 0
	m.running = true
	m.steps[0].IsRunning = true
	m.IsOpen = true
	return tea.Batch(m.cmdRunStep(0), m.spinner.Tick), nil
}

// Update handles all messages while the running modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.hasFailed() {
			switch msg.String() {
			case "esc", "enter", "q":
				m.IsOpen = false
				return m, nil, Result{Done: true, Err: m.stepErr()}
			}
		}
		return m, nil, Result{}

	case stepResultMsg:
		if msg.idx != m.stepIdx {
			return m, nil, Result{}
		}
		if msg.err != nil {
			m.steps[msg.idx].IsRunning = false
			m.steps[msg.idx].HasFailed = true
			return m, nil, Result{}
		}
		m.steps[msg.idx].IsRunning = false
		m.steps[msg.idx].IsDone = true
		next := msg.idx + 1
		if next >= len(m.steps) {
			m.running = false
			m.IsOpen = false
			return m, nil, Result{Done: true}
		}
		m.stepIdx = next
		m.steps[next].IsRunning = true
		return m, m.cmdRunStep(next), Result{}

	case spinner.TickMsg:
		if !m.running {
			return m, nil, Result{}
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd, Result{}
	}
	return m, nil, Result{}
}

func (m Model) cmdRunStep(idx int) tea.Cmd {
	step := m.steps[idx]
	return func() tea.Msg {
		_, err := step.run()
		return stepResultMsg{idx: idx, err: err}
	}
}

func (m Model) hasFailed() bool {
	for _, s := range m.steps {
		if s.HasFailed {
			return true
		}
	}
	return false
}

func (m Model) stepErr() error {
	for _, s := range m.steps {
		if s.HasFailed {
			return &StepError{Title: s.TitleFailed}
		}
	}
	return nil
}

// StepError is returned when a step fails.
type StepError struct {
	Title string
}

func (e *StepError) Error() string { return e.Title }

// View renders the running modal.
func (m Model) View(width int) string {
	hash := m.Hash
	if len(hash) > 7 {
		hash = hash[:7]
	}

	header := "Rewording commit:\n\n  " + ui.StyleTitle.Render(hash) + " " + m.Subject

	modalW := width / 2
	if modalW < 56 {
		modalW = 56
	}

	displaySteps := make([]components.Step, len(m.steps))
	for i, s := range m.steps {
		displaySteps[i] = s.Step
	}
	body := header + "\n\n" + components.RenderSteps(displaySteps, m.spinner.View())
	if m.hasFailed() {
		body += "\n\n" + ui.StyleMuted.Render("press esc to dismiss")
	}
	return ui.RenderModalFrame(ui.ModalFrameOptions{
		Body:        body,
		Width:       modalW,
		BorderColor: ui.ColorYellow,
		HintColor:   ui.ColorSubtle,
	})
}
