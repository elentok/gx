package log

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
)

var commitInfoSubjectStyle = lipgloss.NewStyle().Foreground(ui.ColorYellow).Bold(true)
var commitInfoMetaStyle = lipgloss.NewStyle().Foreground(ui.ColorSubtle).Italic(true)

type commitInfoMsg struct {
	details git.CommitDetails
	err     error
}

// cmdFetchCommitInfo reads the selected list row and fetches full commit
// details for the popup, mirroring cmdFetchRewordDetails (model_reword.go) -
// the list row itself doesn't carry the commit body the popup wants to show.
func (m Model) cmdFetchCommitInfo() tea.Cmd {
	rows := m.listPanel.Rows()
	cursor := m.listPanel.Selected()
	if cursor < 0 || cursor >= len(rows) {
		return nil
	}
	row := rows[cursor]
	if row.kind == rowPseudoStatus {
		return nil
	}
	hash := row.commit.FullHash
	root := m.worktreeRoot
	return func() tea.Msg {
		details, err := git.CommitDetailsForRef(root, hash)
		return commitInfoMsg{details: details, err: err}
	}
}

func (m Model) handleCommitInfo(msg commitInfoMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.commitInfoDetails = msg.details
	m.commitInfoOpen = true
	return m, nil
}

// handleCommitInfoKey closes the popup on esc/i and swallows every other key
// so it doesn't leak through to the list underneath.
func (m Model) handleCommitInfoKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch kp.String() {
		case "esc", "i":
			m.commitInfoOpen = false
		}
	}
	return m, nil
}

func (m Model) renderCommitInfoPopup() string {
	width := maxInt(20, m.width-4)
	lines := commitInfoLines(m.commitInfoDetails)
	height := len(lines) + 2
	if maxH := maxInt(1, (m.height-2)/2); height > maxH {
		height = maxH
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width: width, Height: height, Title: "Commit",
		RightTitle:  commitInfoMetaStyle.Render(m.commitInfoDetails.Date.Format("2006-01-02 15:04")),
		Lines:       lines,
		BorderColor: ui.ColorOrange, TitleColor: ui.ColorOrange, TitleBold: true,
	})
}

func commitInfoLines(details git.CommitDetails) []string {
	metaLine := commitInfoMetaStyle.Render(details.Hash) + " " +
		commitInfoMetaStyle.Render(ui.RelativeTimeCompact(details.Date)) +
		commitInfoMetaStyle.Render(" by ") + commitInfoMetaStyle.Render(details.AuthorName)
	return []string{commitInfoSubjectStyle.Render(details.Subject), metaLine}
}
