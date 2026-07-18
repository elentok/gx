package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const ansiReset = "\x1b[0m"

type ModalFrameOptions struct {
	Title           string
	RightTitle      string
	Body            string
	Hint            string
	Width           int
	BorderColor     color.Color
	TitleColor      color.Color
	RightTitleColor color.Color
	HintColor       color.Color
	PaddingX        int
	TitleInBorder   bool
}

func RenderModalFrame(opts ModalFrameOptions) string {
	paddingX := opts.PaddingX
	if paddingX == 0 {
		paddingX = 1
	}
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(opts.BorderColor).
		Padding(0, paddingX)
	if opts.Width > 0 {
		borderStyle = borderStyle.Width(opts.Width)
	}

	parts := make([]string, 0, 5)
	if !opts.TitleInBorder && strings.TrimSpace(opts.Title) != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(opts.TitleColor).Bold(true).Render(opts.Title))
	}
	if strings.TrimSpace(opts.Body) != "" {
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, opts.Body)
	}
	if strings.TrimSpace(opts.Hint) != "" {
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(opts.HintColor).Render(opts.Hint))
	}
	rendered := borderStyle.Render(strings.Join(parts, "\n"))

	if opts.TitleInBorder && (strings.TrimSpace(opts.Title) != "" || strings.TrimSpace(opts.RightTitle) != "") {
		rightColor := opts.RightTitleColor
		if rightColor == nil {
			rightColor = opts.TitleColor
		}
		rendered = injectBorderTitle(rendered, opts.Title, opts.RightTitle, opts.TitleColor, rightColor, opts.BorderColor)
	}
	return rendered
}

// injectBorderTitle replaces the top border line of a rendered frame with one
// that embeds titles, e.g.  ╭─ Title ───── 2/5 ─╮.
func injectBorderTitle(frame, title, rightTitle string, titleColor, rightTitleColor, borderColor color.Color) string {
	lines := strings.Split(frame, "\n")
	if len(lines) == 0 {
		return frame
	}
	frameW := ansi.StringWidth(lines[0])
	borderS := lipgloss.NewStyle().Foreground(borderColor)
	titleS := lipgloss.NewStyle().Foreground(titleColor).Bold(true)
	rightS := lipgloss.NewStyle().Foreground(rightTitleColor)

	leftStr := ""
	if strings.TrimSpace(title) != "" {
		leftStr = titleS.Render(" " + title + " ")
	}
	rightStr := ""
	if strings.TrimSpace(rightTitle) != "" {
		rightStr = rightS.Render(" " + rightTitle + " ")
	}

	leftW := ansi.StringWidth(leftStr)
	rightW := ansi.StringWidth(rightStr)
	dashes := maxInt(0, frameW-2-leftW-rightW) // -2 for ╭ and ╮
	lines[0] = borderS.Render("╭") + leftStr + borderS.Render(strings.Repeat("─", dashes)) + rightStr + borderS.Render("╮")
	return strings.Join(lines, "\n")
}

type PanelFrameOptions struct {
	Width       int
	Height      int
	Title       string
	RightTitle  string
	Lines       []string
	BorderColor color.Color
	TitleColor  color.Color
	TitleBold   bool
	Background  color.Color
}

func RenderPanelFrame(opts PanelFrameOptions) string {
	if opts.Width < 2 || opts.Height < 2 {
		return ""
	}
	innerW := opts.Width - 2
	innerH := opts.Height - 2
	border := lipgloss.NewStyle().Foreground(opts.BorderColor)
	titleStyle := lipgloss.NewStyle().Foreground(opts.TitleColor)
	if opts.TitleBold {
		titleStyle = titleStyle.Bold(true)
	}
	if opts.Background != nil {
		border = border.Background(opts.Background)
		titleStyle = titleStyle.Background(opts.Background)
	}

	titleSeg := ""
	if opts.Title != "" {
		titleSeg = titleStyle.Render(" " + opts.Title + " ")
	}
	rightSeg := ""
	if opts.RightTitle != "" {
		rightSeg = titleStyle.Render(" " + opts.RightTitle + " ")
	}
	titleW := ansi.StringWidth(titleSeg)
	rightW := ansi.StringWidth(rightSeg)
	topInner := ""
	if rightW >= innerW {
		topInner = ansi.Truncate(rightSeg, innerW, "")
	} else if titleW+rightW >= innerW {
		titleSeg = ansi.Truncate(titleSeg, innerW-rightW, "")
		titleW = ansi.StringWidth(titleSeg)
		topInner = titleSeg + rightSeg
	} else if titleW >= innerW {
		topInner = ansi.Truncate(titleSeg, innerW, "")
		titleW = ansi.StringWidth(topInner)
	} else {
		topInner = titleSeg + border.Render(strings.Repeat("─", innerW-titleW-rightW)) + rightSeg
	}
	if titleW+rightW < innerW && !strings.Contains(topInner, "─") {
		topInner += border.Render(strings.Repeat("─", innerW-titleW-rightW))
	}

	lines := opts.Lines
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	body := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], innerW, "")
		}
		line += strings.Repeat(" ", maxInt(0, innerW-ansi.StringWidth(line)))
		if opts.Background != nil {
			line = lipgloss.NewStyle().Background(opts.Background).Render(line)
		}
		body = append(body, border.Render("│")+line+ansiReset+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", innerW) + "╯")
	top := border.Render("╭") + topInner + border.Render("╮")
	return strings.Join(append([]string{top}, append(body, bottom)...), "\n")
}

// PanelOptions is the frame-free sibling of PanelFrameOptions, used for
// persistent layout panels (see docs/plans/frame-free-panel-design-map.md).
// Panels render edge-to-edge: separation between adjacent panels is not a
// property of the panel itself (a single full-screen panel needs none) - it's
// a seam drawn by the layout that composes panels together, see RenderSeamRow
// / RenderSeamColumn.
type PanelOptions struct {
	Width      int
	Height     int
	Title      string
	RightTitle string
	Lines      []string
	TitleColor color.Color
	TitleBold  bool
	Background color.Color // body fill
	// HeaderBackground fills the title row only, distinct from the body so an
	// active panel's header can read as more prominent than an inactive
	// one's. Defaults to Background when unset.
	HeaderBackground color.Color
	PaddingX         int // inner gap painted with Background
	PaddingY         int
}

// RenderPanel renders a panel with no border glyphs: separation comes
// from the header/body background split, a 1-cell margin between them, and
// the surrounding seam drawn by the layout.
func RenderPanel(opts PanelOptions) string {
	if opts.Width < 2 || opts.Height < 2 {
		return ""
	}
	headerBg := opts.HeaderBackground
	if headerBg == nil {
		headerBg = opts.Background
	}
	titleStyle := lipgloss.NewStyle().Foreground(opts.TitleColor).Background(headerBg)
	if opts.TitleBold {
		titleStyle = titleStyle.Bold(true)
	}

	contentW := opts.Width - 2*opts.PaddingX

	renderRow := func(rowBg color.Color, content string, contentIsRaw bool) string {
		fillStyle := lipgloss.NewStyle().Background(rowBg)
		blank := func(w int) string {
			if w <= 0 {
				return ""
			}
			return fillStyle.Render(strings.Repeat(" ", w))
		}
		var mid string
		if contentIsRaw {
			text := ansi.Truncate(content, contentW, "")
			text += strings.Repeat(" ", maxInt(0, contentW-ansi.StringWidth(text)))
			mid = RenderRowWithBackground(text, rowBg)
		} else {
			mid = content
			w := ansi.StringWidth(content)
			mid += fillStyle.Render(strings.Repeat(" ", maxInt(0, contentW-w)))
		}
		return blank(opts.PaddingX) + mid + blank(opts.PaddingX)
	}

	titleSeg := ""
	if opts.Title != "" {
		titleSeg = titleStyle.Render(opts.Title)
	}
	rightSeg := ""
	if opts.RightTitle != "" {
		rightSeg = lipgloss.NewStyle().Foreground(ColorSubtle).Background(headerBg).Render(opts.RightTitle)
	}
	titleW := ansi.StringWidth(titleSeg)
	rightW := ansi.StringWidth(rightSeg)
	gap := maxInt(0, contentW-titleW-rightW)
	titleLine := titleSeg + lipgloss.NewStyle().Background(headerBg).Render(strings.Repeat(" ", gap)) + rightSeg

	rows := make([]string, 0, opts.Height)
	rows = append(rows, renderRow(headerBg, titleLine, false))
	for i := 0; i < opts.PaddingY; i++ {
		rows = append(rows, renderRow(opts.Background, "", true))
	}
	bodyH := opts.Height - 2*opts.PaddingY - 1 // header row
	lines := opts.Lines
	for i := 0; i < bodyH; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		rows = append(rows, renderRow(opts.Background, line, true))
	}
	for i := 0; i < opts.PaddingY; i++ {
		rows = append(rows, renderRow(opts.Background, "", true))
	}
	return strings.Join(rows, "\n")
}

// RenderSeamRow draws a full-width horizontal gap between two vertically
// stacked frameless panels. The seam has its own background so it reads as a
// visible (but low-contrast) separator rather than empty space.
func RenderSeamRow(width int, seamColor color.Color) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(seamColor).Render(strings.Repeat(" ", width))
}

// RenderSeamColumn draws a full-height vertical gap between two
// horizontally adjacent frameless panels.
func RenderSeamColumn(height int, seamColor color.Color) string {
	if height <= 0 {
		return ""
	}
	row := lipgloss.NewStyle().Background(seamColor).Render(" ")
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
