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

// commitInfoPopup is the commit-info popup state. Its methods take the
// terminal size from the caller instead of reaching into Model.
type commitInfoPopup struct {
	open    bool
	details git.CommitDetails
	scroll  int
}

// fetch reads the selected list row and fetches full commit details for the
// popup, mirroring cmdFetchRewordDetails (model_reword.go) - the list row
// itself doesn't carry the commit body the popup wants to show.
func (p commitInfoPopup) fetch(root string, rows []row, cursor int) tea.Cmd {
	if cursor < 0 || cursor >= len(rows) {
		return nil
	}
	row := rows[cursor]
	if row.kind == rowPseudoStatus {
		return nil
	}
	hash := row.commit.FullHash
	return func() tea.Msg {
		details, err := git.CommitDetailsForRef(root, hash)
		return commitInfoMsg{details: details, err: err}
	}
}

func (p *commitInfoPopup) handle(msg commitInfoMsg) {
	if msg.err != nil {
		return
	}
	p.details = msg.details
	p.open = true
	p.scroll = 0
}

// key closes the popup on esc/i, scrolls its body on navigation keys, and
// swallows every other key so it doesn't leak through to the list underneath.
func (p *commitInfoPopup) key(msg tea.Msg, width, height int) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return
	}
	switch kp.String() {
	case "esc", "i":
		p.open = false
	case "up", "k":
		p.scroll = p.clamp(p.scroll-1, width, height)
	case "down", "j":
		p.scroll = p.clamp(p.scroll+1, width, height)
	case "pgup", "ctrl+u":
		p.scroll = p.clamp(p.scroll-p.innerHeight(height), width, height)
	case "pgdown", "ctrl+d":
		p.scroll = p.clamp(p.scroll+p.innerHeight(height), width, height)
	}
}

// wheel scrolls the popup body in response to a mouse-wheel event while it's
// open, taking priority over the list scroll behind it.
func (p *commitInfoPopup) wheel(msg tea.MouseWheelMsg, width, height int) {
	dir, ok := ui.WheelDirection(msg)
	if !ok {
		return
	}
	p.scroll = p.clamp(p.scroll+dir*ui.WheelScrollLines, width, height)
}

func (p commitInfoPopup) innerHeight(height int) int {
	return maxInt(5, height-8) - 2
}

func (p commitInfoPopup) clamp(offset, width, height int) int {
	lines := commitInfoLines(p.details, maxInt(20, width-20)-2)
	return ui.ClampScrollOffset(offset, len(lines), p.innerHeight(height))
}

func (p commitInfoPopup) render(termWidth, termHeight int) string {
	width := maxInt(20, termWidth-20)
	height := maxInt(5, termHeight-8)
	lines := commitInfoLines(p.details, width-2)
	scroll := ui.ClampScrollOffset(p.scroll, len(lines), height-2)
	if scroll < len(lines) {
		lines = lines[scroll:]
	} else {
		lines = nil
	}
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width: width, Height: height, Title: "Commit",
		RightTitle:  commitInfoMetaStyle.Render(p.details.Date.Format("2006-01-02 15:04")),
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
