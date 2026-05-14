package amend

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

// Result describes what changed during a single Update call.
type Result struct {
	Decided  bool  // user made a yes/no choice (confirm phase)
	Accepted bool  // user chose yes
	Done     bool  // amend operation finished (running or confirm-rejected)
	Err      error // non-nil if a step failed
}

// execStep pairs display state with its git operation.
type execStep struct {
	components.Step
	run func() (string, error)
}

type stepResultMsg struct {
	idx int
	err error
}

// Model owns the entire amend lifecycle: confirm dialog → running steps.
type Model struct {
	IsOpen bool

	// commit info shown in confirm view
	Hash    string
	Subject string
	files   []string
	pushed  bool

	// confirm phase
	yes bool

	// running phase
	running  bool
	steps    []execStep
	stepIdx  int
	spinner  spinner.Model
	root     string
}

// New returns a zero-value Model.
func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{spinner: sp}
}

// Open fetches staged files, checks push status, computes the step list, and opens the modal.
func (m *Model) Open(root, hash, subject string) error {
	staged, err := git.StagedFiles(root)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return fmt.Errorf("no staged changes to amend")
	}
	pushed, _ := git.IsCommitPushed(root, hash)

	m.root = root
	m.Hash = hash
	m.Subject = subject
	m.files = staged
	m.pushed = pushed
	m.yes = true
	m.running = false
	m.stepIdx = 0

	steps, err := buildSteps(root, hash)
	if err != nil {
		return err
	}
	m.steps = steps
	m.IsOpen = true
	return nil
}

func buildSteps(root, hash string) ([]execStep, error) {
	isHead, err := git.IsHEAD(root, hash)
	if err != nil {
		return nil, err
	}
	if isHead {
		return []execStep{
			{
				Step: components.Step{
					TitleBefore:  "amend HEAD",
					RunningTitle: "amending HEAD...",
					TitleAfter:   "amended HEAD",
					TitleFailed:  "amend failed",
				},
				run: func() (string, error) { return git.AmendHead(root) },
			},
		}, nil
	}

	// Check for unstaged working-tree changes BEFORE the fixup commit is created.
	// These need to be stashed so the rebase can run on a clean working tree.
	// Staged changes are consumed by the fixup commit (step 1) and don't need stashing.
	needStash, err := git.HasUnstagedChanges(root)
	if err != nil {
		return nil, err
	}

	// Step order: fixup first (consumes staged changes), then stash unstaged changes,
	// then rebase, then pop. This avoids stashing the staged changes.
	steps := []execStep{
		{
			Step: components.Step{
				TitleBefore:  "create fixup commit",
				RunningTitle: "creating fixup commit...",
				TitleAfter:   "created fixup commit",
				TitleFailed:  "fixup commit failed",
			},
			run: func() (string, error) { return git.CommitFixup(root, hash) },
		},
	}
	if needStash {
		steps = append(steps, execStep{
			Step: components.Step{
				TitleBefore:  "stash",
				RunningTitle: "stashing...",
				TitleAfter:   "stashed",
				TitleFailed:  "stash failed",
			},
			run: func() (string, error) { return git.StashPushAuto(root) },
		})
	}
	steps = append(steps,
		execStep{
			Step: components.Step{
				TitleBefore:  "rebase",
				RunningTitle: "rebasing...",
				TitleAfter:   "rebased",
				TitleFailed:  "rebase failed",
			},
			run: func() (string, error) { return git.RebaseAutosquash(root, hash) },
		},
	)
	if needStash {
		steps = append(steps, execStep{
			Step: components.Step{
				TitleBefore:  "restore stash",
				RunningTitle: "restoring stash...",
				TitleAfter:   "restored stash",
				TitleFailed:  "stash pop failed",
			},
			run: func() (string, error) { return git.StashPop(root) },
		})
	}
	return steps, nil
}

// Update handles all messages while the modal is open.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Result) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.running {
			// Only allow dismiss after failure
			if m.hasFailed() {
				switch msg.String() {
				case "esc", "enter", "q":
					m.IsOpen = false
					return m, nil, Result{Done: true, Err: m.stepErr()}
				}
			}
			return m, nil, Result{}
		}
		// Confirm phase
		nextYes, decided, accepted, handled := components.UpdateConfirm(msg, m.yes)
		if !handled {
			return m, nil, Result{}
		}
		m.yes = nextYes
		if !decided {
			return m, nil, Result{}
		}
		if !accepted {
			m.IsOpen = false
			return m, nil, Result{Decided: true, Accepted: false, Done: true}
		}
		// Start running
		m.running = true
		m.steps[0].IsRunning = true
		cmd := tea.Batch(m.cmdRunStep(0), m.spinner.Tick)
		return m, cmd, Result{Decided: true, Accepted: true}

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
			// Return a descriptive error based on which step failed
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

// View renders the modal. When running, shows steps instead of confirm buttons.
func (m Model) View(width int) string {
	hash := m.Hash
	if len(hash) > 7 {
		hash = hash[:7]
	}
	lines := []string{
		"  " + ui.StyleTitle.Render(hash) + " " + m.Subject,
		"",
		"Staged files:",
	}
	limit := 10
	if len(m.files) < limit {
		limit = len(m.files)
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, "  - "+m.files[i])
	}
	if len(m.files) > limit {
		lines = append(lines, "  ...")
	}
	if m.pushed {
		lines = append(lines, "")
		lines = append(lines, ui.StyleWarning.Render("⚠ This commit has been pushed to origin"))
	}

	header := "Amend staged changes into:\n\n" + strings.Join(lines, "\n")

	modalW := width / 2
	if modalW < 56 {
		modalW = 56
	}

	if m.running || m.hasFailed() {
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

	return components.RenderConfirmModal(
		header,
		m.yes,
		ui.ColorYellow,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorSubtle,
		modalW,
	)
}
