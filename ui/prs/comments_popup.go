package prs

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/components"
)

// commentsPopup shows one PR's comment timeline (issue comments plus any
// non-empty review-summary bodies), fetched on demand only while it's open —
// see issues/13-comments-popup.md. Dismissed via the same esc/q/enter
// convention as ui/help.Model.
type commentsPopup struct {
	isOpen   bool
	loading  bool
	err      error
	comments []git.PRComment
	width    int
}

// open resets the popup into its loading state, sized to fit within the
// container (clamped so it neither overflows a narrow terminal nor grows
// unreasonably wide on a large one).
func (p *commentsPopup) open(containerWidth int) {
	*p = commentsPopup{
		isOpen:  true,
		loading: true,
		width:   min(max(containerWidth-8, 40), 76),
	}
}

func (p *commentsPopup) close() {
	*p = commentsPopup{}
}

// loaded records a completed (successful or failed) comment fetch.
func (p *commentsPopup) loaded(comments []git.PRComment, err error) {
	p.loading = false
	p.comments = comments
	p.err = err
}

func (p *commentsPopup) handleKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc", "q", "enter":
		p.close()
	}
}

func (p commentsPopup) view() string {
	if !p.isOpen {
		return ""
	}

	body := ""
	switch {
	case p.loading:
		body = ui.StyleMuted.Render("loading…")
	case p.err != nil:
		body = ui.StyleWarning.Render("error: " + p.err.Error())
	case len(p.comments) == 0:
		body = ui.StyleMuted.Render("no comments")
	default:
		body = p.renderComments()
	}

	return components.RenderOutputModal("Comments", body, ui.HintDismiss(), ui.ColorBlue, ui.ColorBlue, ui.ColorGray, p.width)
}

// renderComments joins each comment as an author/relative-time header
// followed by its body, oldest first, blank-line separated.
func (p commentsPopup) renderComments() string {
	parts := make([]string, 0, len(p.comments)*2)
	for i, c := range p.comments {
		if i > 0 {
			parts = append(parts, "")
		}
		header := ui.StyleTitle.Render(c.Author) + " " + ui.StyleHint.Render(ui.RelativeTimeCompact(c.CreatedAt))
		parts = append(parts, header, c.Body)
	}
	return strings.Join(parts, "\n")
}
