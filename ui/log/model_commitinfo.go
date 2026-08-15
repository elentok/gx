package log

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	m.commitInfoScroll = 0
	return m, nil
}

// handleCommitInfoKey closes the popup on esc/i, scrolls its body on
// navigation keys, and swallows every other key so it doesn't leak through
// to the list underneath.
func (m Model) handleCommitInfoKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch kp.String() {
	case "esc", "i":
		m.commitInfoOpen = false
	case "up", "k":
		m.commitInfoScroll = m.clampCommitInfoScroll(m.commitInfoScroll - 1)
	case "down", "j":
		m.commitInfoScroll = m.clampCommitInfoScroll(m.commitInfoScroll + 1)
	case "pgup", "ctrl+u":
		m.commitInfoScroll = m.clampCommitInfoScroll(m.commitInfoScroll - m.commitInfoInnerHeight())
	case "pgdown", "ctrl+d":
		m.commitInfoScroll = m.clampCommitInfoScroll(m.commitInfoScroll + m.commitInfoInnerHeight())
	}
	return m, nil
}

// handleCommitInfoWheel scrolls the popup body in response to a mouse-wheel
// event while it's open, taking priority over the list scroll behind it.
func (m Model) handleCommitInfoWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return m, nil
	}
	m.commitInfoScroll = m.clampCommitInfoScroll(m.commitInfoScroll + dir*ui.WheelScrollLines)
	return m, nil
}

func (m Model) commitInfoInnerHeight() int {
	return maxInt(5, m.height-8) - 2
}

func (m Model) clampCommitInfoScroll(offset int) int {
	lines := commitInfoLines(m.commitInfoDetails, maxInt(20, m.width-20)-2)
	return ui.ClampScrollOffset(offset, len(lines), m.commitInfoInnerHeight())
}

func (m Model) renderCommitInfoPopup() string {
	width := maxInt(20, m.width-20)
	height := maxInt(5, m.height-8)
	lines := commitInfoLines(m.commitInfoDetails, width-2)
	scroll := ui.ClampScrollOffset(m.commitInfoScroll, len(lines), height-2)
	if scroll < len(lines) {
		lines = lines[scroll:]
	} else {
		lines = nil
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width: width, Height: height, Title: "Commit",
		RightTitle:  commitInfoMetaStyle.Render(m.commitInfoDetails.Date.Format("2006-01-02 15:04")),
		Lines:       lines,
		BorderColor: ui.ColorOrange, TitleColor: ui.ColorOrange, TitleBold: true,
	})
}

func commitInfoLines(details git.CommitDetails, contentWidth int) []string {
	metaLine := commitInfoMetaStyle.Render(details.Hash) + " " +
		commitInfoMetaStyle.Render(ui.RelativeTimeCompact(details.Date)) +
		commitInfoMetaStyle.Render(" by ") + commitInfoMetaStyle.Render(details.AuthorName)
	lines := []string{commitInfoSubjectStyle.Render(details.Subject), metaLine}
	if body := strings.TrimSpace(details.Body); body != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Split(ansi.Wordwrap(body, maxInt(1, contentWidth), ""), "\n")...)
	}
	return lines
}
