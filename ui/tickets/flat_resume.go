package tickets

import (
	tea "charm.land/bubbletea/v2"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
	"github.com/elentok/gx/ui/notify"
)

// resumeConfirmState is the confirm modal ticket 06b opens on `r`, for a
// paused row's resume prompt or a needs-attention row's recheck prompt —
// wording differs by pauseKind, but confirming either makes the same
// resumeControl call (see handleResumeConfirmUpdate).
type resumeConfirmState struct {
	open      bool
	label     string
	pauseKind ralphloop.PauseKind
	yes       bool
}

func (s resumeConfirmState) isOpen() bool { return s.open }

// isRecheck reports whether s is a needs-attention row's recheck prompt
// rather than a paused row's resume prompt.
func (s resumeConfirmState) isRecheck() bool {
	return s.pauseKind == ralphloop.PauseNeedsAttention
}

// openResumeConfirm opens ticket 06b's confirm modal for identifier's live
// paused state, defaulting the choice to "yes". ok is false if identifier
// has no live paused/needs-attention state to confirm against, in which case
// `r` is a no-op (see handleFlatKey).
func (m FlatModel) openResumeConfirm(identifier string) (FlatModel, bool) {
	live, ok := m.live[identifier]
	if !ok || !live.paused {
		return m, false
	}
	m.resumeConfirm = resumeConfirmState{
		open: true, label: live.label, pauseKind: live.pauseKind, yes: true,
	}
	return m, true
}

// handleResumeConfirmUpdate applies the shared y/n/enter/esc/h/l confirm
// pattern (ui/components/modal_confirm.go, the same one ui/commit's
// amend-confirm and ui/worktrees/model_confirm_modal.go use) while ticket
// 06b's modal is open. Confirming calls resumeControl in-process — 06a's
// Gate.ForceResume, with no poll-interval delay — for both the resume and
// recheck case, which share the same underlying control path; only the
// modal's copy differs by pauseKind.
func (m FlatModel) handleResumeConfirmUpdate(msg tea.Msg) (FlatModel, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	nextYes, decided, accepted, handled := components.UpdateConfirm(kp, m.resumeConfirm.yes)
	if !handled {
		return m, nil
	}
	if !decided {
		m.resumeConfirm.yes = nextYes
		return m, nil
	}
	label, recheck := m.resumeConfirm.label, m.resumeConfirm.isRecheck()
	m.resumeConfirm = resumeConfirmState{}
	if !accepted {
		return m, nil
	}
	if m.resumeControl == nil {
		return m, nil
	}
	pastTense, presentTense := "resumed", "resume"
	if recheck {
		pastTense, presentTense = "rechecked", "recheck"
	}
	if !m.resumeControl(label) {
		return m, notify.Warning("could not " + presentTense + ": ticket was not paused")
	}
	return m, notify.Success(pastTense)
}

func (m FlatModel) resumeConfirmView() string {
	var prompt string
	borderColor := ui.ColorMauve
	if m.resumeConfirm.isRecheck() {
		prompt = "Re-check this ticket?"
		borderColor = ui.ColorOrange
	} else {
		prompt = "Resume this ticket?"
	}
	modalW := max(m.width/2, 48)
	return components.RenderConfirmModal(prompt, m.resumeConfirm.yes, borderColor, ui.ColorGreen, ui.ColorRed, ui.ColorSubtle, modalW)
}

// WithResumeControl overrides the resume/recheck control-path call
// confirming ticket 06b's modal makes — nil unless wired, in which case
// confirming is a no-op besides closing the modal (every test but the ones
// exercising this exact flow). cmd/ralphloop.go's production wiring passes
// the running loop's shared ralphloop.Gate.ForceResume, matched by signature.
func (m FlatModel) WithResumeControl(fn func(label string) bool) FlatModel {
	m.resumeControl = fn
	return m
}
