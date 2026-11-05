package commit

import (
	"image/color"
	"regexp"
	"strings"

	"github.com/elentok/gx/git"
	"github.com/elentok/gx/ui"
	"github.com/elentok/gx/ui/list"
	"github.com/elentok/gx/ui/pushstate"
	"github.com/elentok/gx/ui/refrules"
)

// headerPane owns the commit header popup state: scroll offset, whether the
// body is expanded, and the compiled ref rules for decoration badges. Model
// owns the inputs and passes them in as a headerInput on every call.
type headerPane struct {
	headerOffset int
	bodyExpanded bool

	// compiledRefRules/compiledHideRefs sort and filter decoration badges the
	// same way ui/log does, so tags appear in the same order in both views.
	compiledRefRules []refrules.CompiledRefRule
	compiledHideRefs []*regexp.Regexp
}

// headerInput is the Model state the header pane reads. focused is true when
// the popup is open and its container owns keyboard focus.
type headerInput struct {
	details   git.CommitDetails
	trunk     string
	pushState pushstate.PushState
	width     int
	height    int
	focused   bool
	nerdFont  bool
}

func (m Model) headerInput() headerInput {
	return headerInput{
		details:   m.details,
		trunk:     m.trunk,
		pushState: m.pushState,
		width:     m.width,
		height:    m.height,
		focused:   m.isContainerFocused() && m.focusHeader,
		nerdFont:  m.settings.UseNerdFontIcons,
	}
}

func (p *headerPane) scrollHeader(in headerInput, delta int) {
	visible := p.headerViewportRowsCount(in)
	total := len(p.headerLines(in))
	maxOffset := max(0, total-visible)
	p.headerOffset += delta
	if p.headerOffset < 0 {
		p.headerOffset = 0
	}
	if p.headerOffset > maxOffset {
		p.headerOffset = maxOffset
	}
}

func (p *headerPane) scrollHeaderPage(in headerInput, direction int) {
	p.scrollHeader(in, direction*list.DefaultScroll)
}

func (p headerPane) headerLines(in headerInput) []string {
	subjectLine := p.subjectLine(in)
	metaLine := p.metaLine(in)
	badges := decorationBadgeParts(in.details.Decorations, p.compiledHideRefs, p.compiledRefRules, in.trunk)
	lines := append([]string{subjectLine}, metaLineWithBadges(metaLine, badges, max(1, in.width-2))...)
	if p.bodyExpanded {
		body := p.commitMessageBody(in)
		if body != "" {
			lines = append(lines, "")
			lines = append(lines, strings.Split(body, "\n")...)
		}
	}
	return lines
}

// subjectLine renders the commit subject, colored by push/pull state when
// known, with a parenthetical icon + label spelling out that state (there's
// no surrounding list to infer it from, unlike the log view's rows).
func (p headerPane) subjectLine(in headerInput) string {
	subject := commitSubjectStyle.Render(in.details.Subject)
	if in.pushState.Label == "" {
		return subject
	}
	return in.pushState.Style.Render(in.details.Subject) + " " +
		commitMetaStyle.Render("(") + in.pushState.Style.Render(in.pushState.Icon) +
		commitMetaStyle.Render(" "+in.pushState.Label+")")
}

// metaLine renders the hash, relative date, and author on their own line
// below the subject.
func (p headerPane) metaLine(in headerInput) string {
	return commitMetaStyle.Render(in.details.Hash) + " " +
		commitMetaStyle.Render(ui.RelativeTimeCompact(in.details.Date)) +
		commitMetaStyle.Render(" by ") + commitMetaStyle.Render(in.details.AuthorName)
}

func (p headerPane) headerTitle(in headerInput) string {
	if !p.bodyExpanded && p.commitMessageBody(in) != "" {
		return "Commit " + commitMetaStyle.Render("(b to expand)")
	}
	return "Commit"
}

// renderHeaderPopup renders the commit header (subject/meta/badges/expanded
// body) as a standalone popup panel; the caller centers it via
// ui.OverlayCenter when the popup is open (m.focusHeader).
func (p headerPane) renderHeaderPopup(in headerInput) string {
	width := p.headerPopupWidth(in)
	height := p.headerPopupHeight(in)
	title := p.headerTitle(in)
	rightTitle := p.headerRightTitle(in)
	lines := p.visibleHeaderLines(in, max(1, height-2))
	return ui.RenderPanelFrame(ui.PanelFrameOptions{
		Width: width, Height: height, Title: title, RightTitle: rightTitle, Lines: lines,
		BorderColor: p.headerPaneBorderColor(in), TitleColor: p.headerPaneTitleColor(in), TitleBold: in.focused,
	})
}

func (p headerPane) headerPopupWidth(in headerInput) int {
	return max(20, in.width-4)
}

// headerPopupHeight caps the popup at half the available body height (mirroring
// the space this header pane used to reserve when it rendered inline), so a
// long commit body scrolls inside the popup instead of covering the screen.
func (p headerPane) headerPopupHeight(in headerInput) int {
	maxBody := max(1, (in.height-2)/2)
	natural := len(p.headerLines(in)) + 2
	return min(maxBody, natural)
}

func (p headerPane) headerPaneTitleColor(in headerInput) color.Color {
	if in.focused {
		return ui.ColorOrange
	}
	return ui.ColorBlue
}

func (p headerPane) headerPaneBorderColor(in headerInput) color.Color {
	if in.focused {
		return ui.ColorOrange
	}
	return ui.ColorBorder
}

func (p headerPane) commitMessageBody(in headerInput) string {
	body := normalizeCommitBody(in.details.Body)
	if body == "" {
		return ""
	}
	subject := strings.TrimSpace(in.details.Subject)
	if subject == "" {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return ""
	}
	if strings.TrimSpace(lines[0]) == subject {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (p headerPane) headerRightTitle(in headerInput) string {
	return commitMetaStyle.Render(in.details.Date.Format("2006-01-02 15:04"))
}

func (p headerPane) headerViewportRowsCount(in headerInput) int {
	return max(1, p.headerPopupHeight(in)-2)
}

func (p headerPane) visibleHeaderLines(in headerInput, viewportRows int) []string {
	all := p.headerLines(in)
	if len(all) == 0 {
		return []string{""}
	}
	if viewportRows < 1 {
		viewportRows = 1
	}
	if viewportRows > len(all) {
		viewportRows = len(all)
	}
	offset := p.headerOffset
	maxOffset := max(0, len(all)-viewportRows)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	out := make([]string, 0, viewportRows)
	for i := 0; i < viewportRows; i++ {
		out = append(out, all[offset+i])
	}
	if len(all) > viewportRows {
		topMark, bottomMark := "↑ ", "↓ "
		if in.nerdFont {
			topMark, bottomMark = " ", " "
		}
		markerStyle := commitMetaStyle.Bold(true)
		if offset > 0 && len(out) > 0 {
			out[0] = markerStyle.Render(topMark) + out[0]
		}
		if offset+viewportRows < len(all) && len(out) > 0 {
			out[len(out)-1] = markerStyle.Render(bottomMark) + out[len(out)-1]
		}
	}
	return out
}
