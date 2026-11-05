// Package stagemodal owns the stage page's action modals: confirm, running
// output, credential prompt and error.
package stagemodal

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

var (
	ansiCSIRe = regexp.MustCompile(`\x1b\[[0-9:;<=>?]*[ -/]*[@-~]`)
	ansiOSCRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`) // OSC ... BEL/ST
)

// ConfirmRequest is everything a confirm modal needs to ask a question and, on
// acceptance, to act on the answer.
type ConfirmRequest struct {
	Title            string
	Lines            []string
	Yes              bool
	Action           ConfirmAction
	Remote           string
	Branch           string
	Paths            []string
	DeletePaths      []string
	Patch            string
	PatchUnidiffZero bool
	DiscardUntracked bool
	DisplayPath      string
	PreservePath     string
}

// Result reports what a modal decided. The zero value means nothing
// happened that the owner must act on.
type Result struct {
	// Accepted is set with Confirm when the user accepted a confirm request.
	Accepted bool
	Confirm  ConfirmRequest
	// Finished is set with Action when the running action produced its result.
	Finished bool
	Action   ActionResult
	// CredentialOpened is set when the running action asked for a credential.
	CredentialOpened bool
}

// Model owns the stage-action modals.
type Model struct {
	confirmOpen bool
	confirm     ConfirmRequest

	runningOpen    bool
	runningTitle   string
	runningVP      viewport.Model
	runningContent string
	runningRunner  *Runner
	runningDone    bool

	credentialOpen   bool
	credentialPrompt string
	credentialInput  textinput.Model
	credentialSecret bool

	errorOpen bool
	errorVP   viewport.Model
}

func (s Model) Open() bool {
	return s.credentialOpen || s.runningOpen || s.confirmOpen || s.errorOpen
}

func (s Model) ConfirmOpen() bool    { return s.confirmOpen }
func (s Model) RunningOpen() bool    { return s.runningOpen }
func (s Model) CredentialOpen() bool { return s.credentialOpen }
func (s Model) ErrorOpen() bool      { return s.errorOpen }

// Confirm returns the current confirm request.
func (s Model) Confirm() ConfirmRequest { return s.confirm }

// RunningContent returns the sanitized output shown in the running modal.
func (s Model) RunningContent() string { return s.runningContent }

// Cancellable reports whether a started action is still running.
func (s Model) Cancellable() bool {
	return s.runningRunner != nil && !s.runningDone
}

// Cancel asks the running action to stop, if there is one.
func (s Model) Cancel() {
	if s.Cancellable() {
		s.runningRunner.Cancel()
	}
}

// CloseRunning hides the running modal.
func (s *Model) CloseRunning() {
	s.runningOpen = false
}

// OpenError shows vp in the error modal.
func (s *Model) OpenError(vp viewport.Model) {
	s.errorVP = vp
	s.errorOpen = true
}

// Update routes a key to the open modal, in the order credential, running,
// confirm, error.
func (s Model) Update(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
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
	return s, nil, Result{}
}

func (s Model) updateRunning(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	if s.runningDone {
		switch msg.String() {
		case "esc", "enter":
			s.runningOpen = false
			s.runningRunner = nil
			return s, nil, Result{}
		}
	}
	var cmd tea.Cmd
	s.runningVP, cmd = s.runningVP.Update(msg)
	return s, cmd, Result{}
}

func (s Model) updateCredential(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch msg.String() {
	case "esc":
		if s.runningRunner != nil {
			s.runningRunner.Cancel()
		}
		s.credentialOpen = false
		s.credentialPrompt = ""
		return s, nil, Result{}
	case "enter":
		if s.runningRunner != nil {
			_ = s.runningRunner.SubmitPromptInput(s.credentialInput.Value())
		}
		s.credentialOpen = false
		s.credentialPrompt = ""
		return s, nil, Result{}
	}
	var cmd tea.Cmd
	s.credentialInput, cmd = s.credentialInput.Update(msg)
	return s, cmd, Result{}
}

func (s Model) updateConfirm(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	nextYes, decided, accepted, handled := components.UpdateConfirm(msg, s.confirm.Yes)
	if !handled {
		return s, nil, Result{}
	}
	s.confirm.Yes = nextYes
	if !decided {
		return s, nil, Result{}
	}
	req := s.confirm
	s.confirmOpen = false
	s.confirm.Action = ConfirmNone
	if accepted {
		return s, nil, Result{Accepted: true, Confirm: req}
	}
	return s, nil, Result{}
}

func (s Model) updateError(msg tea.KeyPressMsg) (Model, tea.Cmd, Result) {
	switch msg.String() {
	case "esc", "enter":
		s.errorOpen = false
		return s, nil, Result{}
	}
	var cmd tea.Cmd
	s.errorVP, cmd = s.errorVP.Update(msg)
	return s, cmd, Result{}
}

// Poll reads the running action's output, opens the credential prompt when it
// asks for one and reports its result once. It reschedules itself while the
// running modal is waiting on the action.
func (s Model) Poll() (Model, tea.Cmd, Result) {
	var res Result
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
		return s, PollCmd(), res
	}
	return s, nil, res
}

func (s *Model) appendRunningOutput(chunk string) {
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

// OpenRunning shows the running modal for runner. The caller starts the
// runner.
func (s *Model) OpenRunning(width, height int, title string, runner *Runner) {
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
}

func (s *Model) OpenConfirm(req ConfirmRequest) {
	req.Lines = append([]string{}, req.Lines...)
	req.Yes = true
	s.confirmOpen = true
	s.confirm = req
}

func (s Model) RunningView() string {
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

func (s Model) CredentialView() string {
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

func (s Model) ConfirmView(width int) string {
	prompt := s.confirm.Title
	if len(s.confirm.Lines) > 0 {
		prompt = prompt + "\n\n" + strings.Join(s.confirm.Lines, "\n")
	}
	return components.RenderConfirmModal(
		prompt,
		s.confirm.Yes,
		ui.ColorYellow,
		ui.ColorGreen,
		ui.ColorRed,
		ui.ColorSubtle,
		max(56, width/2),
	)
}

func (s Model) ErrorView() string {
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
