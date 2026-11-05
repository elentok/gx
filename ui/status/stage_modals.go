package status

import (
	"strings"
	"unicode"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// confirmRequest is everything a confirm modal needs to ask a question and, on
// acceptance, to act on the answer.
type confirmRequest struct {
	title            string
	lines            []string
	yes              bool
	action           stageConfirmAction
	remote           string
	branch           string
	paths            []string
	deletePaths      []string
	patch            string
	patchUnidiffZero bool
	discardUntracked bool
	displayPath      string
	preservePath     string
}

// ModalResult reports what a modal decided. The zero value means nothing
// happened that the Model must act on.
type ModalResult struct {
	// Accepted is set with Confirm when the user accepted a confirm request.
	Accepted bool
	Confirm  confirmRequest
	// Finished is set with Action when the running action produced its result.
	Finished bool
	Action   stageActionResult
	// CredentialOpened is set when the running action asked for a credential.
	CredentialOpened bool
}

// stageModals owns the stage-action modals: confirm, running output,
// credential prompt and error.
type stageModals struct {
	confirmOpen bool
	confirm     confirmRequest

	runningOpen    bool
	runningTitle   string
	runningVP      viewport.Model
	runningContent string
	runningRunner  *stageActionRunner
	runningDone    bool

	credentialOpen   bool
	credentialPrompt string
	credentialInput  textinput.Model
	credentialSecret bool

	errorOpen bool
	errorVP   viewport.Model
}

func (s stageModals) Open() bool {
	return s.credentialOpen || s.runningOpen || s.confirmOpen || s.errorOpen
}

// Update routes a key to the open modal, in the order credential, running,
// confirm, error.
func (s stageModals) Update(msg tea.KeyPressMsg) (stageModals, tea.Cmd, ModalResult) {
	switch {
	case s.credentialOpen:
		return s.updateCredential(msg)
	case s.runningOpen:
		return s.updateRunning(msg)
	case s.confirmOpen:
		return s.updateConfirm(msg)
	case s.errorOpen:
		return s.updateError(msg)
	}
	return s, nil, ModalResult{}
}

func (s stageModals) updateRunning(msg tea.KeyPressMsg) (stageModals, tea.Cmd, ModalResult) {
	if s.runningDone {
		switch msg.String() {
		case "esc", "enter":
			s.runningOpen = false
			s.runningRunner = nil
			return s, nil, ModalResult{}
		}
	}
	var cmd tea.Cmd
	s.runningVP, cmd = s.runningVP.Update(msg)
	return s, cmd, ModalResult{}
}

func (s stageModals) updateCredential(msg tea.KeyPressMsg) (stageModals, tea.Cmd, ModalResult) {
	switch msg.String() {
	case "esc":
		if s.runningRunner != nil {
			s.runningRunner.Cancel()
		}
		s.credentialOpen = false
		s.credentialPrompt = ""
		return s, nil, ModalResult{}
	case "enter":
		if s.runningRunner != nil {
			_ = s.runningRunner.SubmitPromptInput(s.credentialInput.Value())
		}
		s.credentialOpen = false
		s.credentialPrompt = ""
		return s, nil, ModalResult{}
	}
	var cmd tea.Cmd
	s.credentialInput, cmd = s.credentialInput.Update(msg)
	return s, cmd, ModalResult{}
}

func (s stageModals) updateConfirm(msg tea.KeyPressMsg) (stageModals, tea.Cmd, ModalResult) {
	nextYes, decided, accepted, handled := components.UpdateConfirm(msg, s.confirm.yes)
	if !handled {
		return s, nil, ModalResult{}
	}
	s.confirm.yes = nextYes
	if !decided {
		return s, nil, ModalResult{}
	}
	req := s.confirm
	s.confirmOpen = false
	s.confirm.action = confirmNone
	if accepted {
		return s, nil, ModalResult{Accepted: true, Confirm: req}
	}
	return s, nil, ModalResult{}
}

func (s stageModals) updateError(msg tea.KeyPressMsg) (stageModals, tea.Cmd, ModalResult) {
	switch msg.String() {
	case "esc", "enter":
		s.errorOpen = false
		return s, nil, ModalResult{}
	}
	var cmd tea.Cmd
	s.errorVP, cmd = s.errorVP.Update(msg)
	return s, cmd, ModalResult{}
}

// Poll drains the running action's output, opens the credential prompt when it
// asks for one and reports its result once. It reschedules itself while the
// running modal is waiting on the action.
func (s stageModals) Poll() (stageModals, tea.Cmd, ModalResult) {
	var res ModalResult
	if s.runningRunner != nil {
		if chunk := s.runningRunner.Consume(); chunk != "" {
			s.appendRunningOutput(chunk)
		}
		if !s.runningDone {
			if !s.credentialOpen {
				if prompt, ok := s.runningRunner.Prompt(); ok {
					s.credentialPrompt = prompt.Text
					s.credentialSecret = prompt.Kind == components.PromptKindSecret
					s.credentialInput = newCredentialInput(s.credentialSecret)
					s.credentialOpen = true
					res.CredentialOpened = true
				}
			}
			if out, done := s.runningRunner.Result(); done {
				s.runningDone = true
				res.Finished = true
				res.Action = out
			}
		}
	}
	if s.runningOpen && !s.runningDone {
		return s, actionPollCmd(), res
	}
	return s, nil, res
}

func (s *stageModals) appendRunningOutput(chunk string) {
	if chunk == "" {
		return
	}
	s.runningContent += sanitizeTerminalOutputForViewport(chunk)
	s.runningVP.SetContent(s.runningContent)
	s.runningVP.GotoBottom()
}

func sanitizeTerminalOutputForViewport(s string) string {
	s = ansiOSCRe.ReplaceAllString(s, "")
	s = ansiCSIRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func newCredentialInput(secret bool) textinput.Model {
	ti := textinput.New()
	ti.Focus()
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '*'
	}
	return ti
}

func (s *stageModals) openRunning(width, height int, title string, runner *stageActionRunner) {
	vpW := width * 2 / 3
	if vpW < 56 {
		vpW = 56
	}
	if vpW > 110 {
		vpW = 110
	}
	vpH := height/2 - 4
	if vpH < 8 {
		vpH = 8
	}
	vp := viewport.New(viewport.WithWidth(vpW-2), viewport.WithHeight(vpH))
	vp.SetContent("")
	s.runningVP = vp
	s.runningContent = ""
	s.runningOpen = true
	s.runningDone = false
	s.runningTitle = title
	s.runningRunner = runner
	runner.Start()
}

func (s *stageModals) openConfirm(req confirmRequest) {
	req.lines = append([]string{}, req.lines...)
	req.yes = true
	s.confirmOpen = true
	s.confirm = req
}

func (s stageModals) runningView() string {
	title := s.runningTitle
	if title == "" {
		title = "Running"
	}
	hint := ui.HintCancelScroll()
	if s.runningDone {
		hint = ui.HintDismissAndScroll()
	}
	return components.RenderOutputModal(
		title,
		s.runningVP.View(),
		hint,
		ui.ColorYellow,
		ui.ColorYellow,
		ui.ColorSubtle,
		s.runningVP.Width(),
	)
}

func (s stageModals) credentialView() string {
	input := s.credentialInput.View()
	if input == "" {
		input = " "
	}
	return components.RenderInputModal(
		"Credential Required",
		s.credentialPrompt,
		input,
		ui.HintSubmitCancel(),
		ui.ColorBlue,
		ui.ColorBlue,
		ui.ColorSubtle,
		0,
	)
}

func (s stageModals) confirmView(width int) string {
	prompt := s.confirm.title
	if len(s.confirm.lines) > 0 {
		prompt = prompt + "\n\n" + strings.Join(s.confirm.lines, "\n")
	}
	return components.RenderConfirmModal(
		prompt,
		s.confirm.yes,
		ui.ColorYellow,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorSubtle,
		maxInt(56, width/2),
	)
}

func (s stageModals) errorView() string {
	return components.RenderOutputModal(
		"Error",
		s.errorVP.View(),
		ui.HintDismissAndScroll(),
		ui.ColorRed,
		ui.ColorRed,
		ui.ColorSubtle,
		s.errorVP.Width(),
	)
}
