package app

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/ui/notify"
)

const notifyTTL = 5 * time.Second
const notifyCap = 4

type notification struct {
	id        string
	kind      notify.NotifyKind
	message   string
	expiresAt time.Time // zero means never (progress)
	addedAt   time.Time // used to match expiry ticks
}

type notifyExpireMsg struct {
	id      string
	addedAt time.Time
}

func (m *Model) handleNotifyMsg(msg notify.NotifyMsg) tea.Cmd {
	now := time.Now()

	n := notification{
		id:      msg.ID,
		kind:    msg.Kind,
		message: msg.Message,
		addedAt: now,
	}
	if msg.Kind != notify.KindProgress {
		n.expiresAt = now.Add(notifyTTL)
	}

	// Replace existing notification with the same ID if present.
	if msg.ID != "" {
		for i, existing := range m.notifications {
			if existing.id == msg.ID {
				m.notifications[i] = n
				return m.expireCmd(n)
			}
		}
	}

	// Append, dropping oldest if over cap.
	m.notifications = append(m.notifications, n)
	if len(m.notifications) > notifyCap {
		m.notifications = m.notifications[len(m.notifications)-notifyCap:]
	}

	var cmds []tea.Cmd
	cmds = append(cmds, m.expireCmd(n))

	// Start spinner on 0→1 progress transition.
	if msg.Kind == notify.KindProgress && m.countProgress() == 1 {
		cmds = append(cmds, m.spinner.Tick)
	}

	return tea.Batch(cmds...)
}

func (m *Model) handleCloseMsg(msg notify.CloseMsg) {
	m.notifications = removeNotificationByID(m.notifications, msg.ID)
}

func (m *Model) handleSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if m.countProgress() == 0 {
		return nil // let the tick die
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return cmd
}


func (m *Model) expireCmd(n notification) tea.Cmd {
	if n.expiresAt.IsZero() {
		return nil
	}
	id, addedAt := n.id, n.addedAt
	return tea.Tick(notifyTTL, func(_ time.Time) tea.Msg {
		return notifyExpireMsg{id: id, addedAt: addedAt}
	})
}

func (m *Model) handleExpireMsg(msg notifyExpireMsg) {
	now := time.Now()
	kept := m.notifications[:0]
	for _, n := range m.notifications {
		expired := !n.expiresAt.IsZero() && !now.Before(n.expiresAt) && n.id == msg.id && n.addedAt.Equal(msg.addedAt)
		if !expired {
			kept = append(kept, n)
		}
	}
	m.notifications = kept
}

func (m *Model) countProgress() int {
	count := 0
	for _, n := range m.notifications {
		if n.kind == notify.KindProgress {
			count++
		}
	}
	return count
}

func removeNotificationByID(ns []notification, id string) []notification {
	kept := ns[:0]
	for _, n := range ns {
		if n.id != id {
			kept = append(kept, n)
		}
	}
	return kept
}

func newSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return sp
}
