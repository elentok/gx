package notify

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

const ttl = 5 * time.Second
const cap = 4

type notification struct {
	id        string
	kind      NotifyKind
	message   string
	expiresAt time.Time
	addedAt   time.Time
}

type expireMsg struct {
	id      string
	addedAt time.Time
}

type Model struct {
	useNerdFont   bool
	notifications []notification
	spinner       spinner.Model
}

func New(useNerdFont bool) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{
		useNerdFont: useNerdFont,
		spinner:     sp,
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch v := msg.(type) {
	case NotifyMsg:
		return m.handleNotifyMsg(v)
	case CloseMsg:
		m.notifications = removeByID(m.notifications, v.ID)
		return m, nil
	case expireMsg:
		m.handleExpire(v)
		return m, nil
	case spinner.TickMsg:
		if m.countProgress() == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleNotifyMsg(msg NotifyMsg) (Model, tea.Cmd) {
	now := time.Now()
	n := notification{
		id:      msg.ID,
		kind:    msg.Kind,
		message: msg.Message,
		addedAt: now,
	}
	if msg.Kind != KindProgress {
		n.expiresAt = now.Add(ttl)
	}

	if msg.ID != "" {
		for i, existing := range m.notifications {
			if existing.id == msg.ID {
				m.notifications[i] = n
				return m, m.expireCmd(n)
			}
		}
	}

	m.notifications = append(m.notifications, n)
	if len(m.notifications) > cap {
		m.notifications = m.notifications[len(m.notifications)-cap:]
	}

	var cmds []tea.Cmd
	cmds = append(cmds, m.expireCmd(n))
	if msg.Kind == KindProgress && m.countProgress() == 1 {
		cmds = append(cmds, m.spinner.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) handleExpire(msg expireMsg) {
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

func (m Model) expireCmd(n notification) tea.Cmd {
	if n.expiresAt.IsZero() {
		return nil
	}
	id, addedAt := n.id, n.addedAt
	return tea.Tick(ttl, func(_ time.Time) tea.Msg {
		return expireMsg{id: id, addedAt: addedAt}
	})
}

func (m Model) countProgress() int {
	count := 0
	for _, n := range m.notifications {
		if n.kind == KindProgress {
			count++
		}
	}
	return count
}

func removeByID(ns []notification, id string) []notification {
	kept := ns[:0]
	for _, n := range ns {
		if n.id != id {
			kept = append(kept, n)
		}
	}
	return kept
}
