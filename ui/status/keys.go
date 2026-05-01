package status

import (
	"fmt"

	"github.com/elentok/gx/ui"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

type stageKeyMap struct {
	m Model
}

var (
	stageKeyUp         = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	stageKeyDown       = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	stageKeyTop        = key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "top"))
	stageKeyBottom     = key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom"))
	stageKeyHelp       = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	stageKeyQuit       = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit"))
	stageKeySearch     = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	stageKeySearchNext = key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match"))
	stageKeySearchPrev = key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match"))
	stageKeyCommit     = key.NewBinding(key.WithKeys("cc"), key.WithHelp("cc", "git commit"))
	stageKeyOutput     = key.NewBinding(key.WithKeys("go"), key.WithHelp("go", "view output"))
	stageKeyGoWorktree = key.NewBinding(key.WithKeys("gw"), key.WithHelp("gw", "goto worktrees"))
	stageKeyGoLog      = key.NewBinding(key.WithKeys("gl"), key.WithHelp("gl", "goto log"))
	stageKeyGoStatus   = key.NewBinding(key.WithKeys("gs"), key.WithHelp("gs", "goto status"))
	stageKeyLog        = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "lazygit log"))
	stageKeyYank       = key.NewBinding(key.WithKeys("yy", "yl", "ya", "yf"), key.WithHelp("yy/yl/ya/yf", "yank"))
	stageKeyYankText   = key.NewBinding(key.WithKeys("yy"), key.WithHelp("yy", "content"))
	stageKeyYankPath   = key.NewBinding(key.WithKeys("yl"), key.WithHelp("yl", "location"))
	stageKeyYankAll    = key.NewBinding(key.WithKeys("ya"), key.WithHelp("ya", "all"))
	stageKeyYankName   = key.NewBinding(key.WithKeys("yf"), key.WithHelp("yf", "filename"))
	stageKeyPull       = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull"))
	stageKeyPush       = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "push"))
	stageKeyRebase     = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "rebase"))
	stageKeyAmend      = key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "amend"))
	stageKeyEdit       = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	stageKeyRefresh    = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	stageKeyContextDec = key.NewBinding(key.WithKeys("["), key.WithHelp("[", "less context"))
	stageKeyContextInc = key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "more context"))
	stageKeyPageDown   = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down"))
	stageKeyPageUp     = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up"))
	stageKeyOpenDiff   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open diff"))
	stageKeyStage      = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "stage/unstage"))
	stageKeyDiscard    = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "discard"))
	stageKeyLeft       = key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "back"))
	stageKeyRight      = key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "open"))
	stageKeyTab        = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch"))
	stageKeyMode       = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "mode"))
	stageKeyVisual     = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "visual mode"))
	stageKeyPrevFile   = key.NewBinding(key.WithKeys(","), key.WithHelp(",", "prev file"))
	stageKeyNextFile   = key.NewBinding(key.WithKeys("."), key.WithHelp(".", "next file"))
	stageKeyScrollDown = key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "scroll down"))
	stageKeyScrollUp   = key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "scroll up"))
	stageKeyRenderMode = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "render"))
	stageKeyFullscreen = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fullscreen"))
	stageKeyWrap       = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "soft wrap"))
	stageKeyDiffBack   = key.NewBinding(key.WithKeys("esc", "h"), key.WithHelp("esc/h", "back to status"))
)

func newStageHelpModel() help.Model {
	h := help.New()
	h.ShortSeparator = " · "
	h.FullSeparator = "  "
	h.Styles.ShortKey = h.Styles.ShortKey.Foreground(catBlue).Bold(true)
	h.Styles.ShortDesc = h.Styles.ShortDesc.Foreground(catSubtle)
	h.Styles.ShortSeparator = h.Styles.ShortSeparator.Foreground(catSubtle)
	h.Styles.FullKey = h.Styles.FullKey.Foreground(catBlue).Bold(true)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(catText)
	h.Styles.FullSeparator = h.Styles.FullSeparator.Foreground(catSubtle)
	return h
}

func (m Model) currentKeyMap() stageKeyMap {
	return stageKeyMap{m: m}
}

func (k stageKeyMap) ShortHelp() []key.Binding {
	if k.m.focus == focusStatus {
		return []key.Binding{stageKeySearch, stageKeyOutput}
	}
	return []key.Binding{stageKeyTab, stageKeyMode, stageKeyVisual, stageKeyRenderMode}
}

func (k stageKeyMap) FullHelp() [][]key.Binding {
	global := []key.Binding{
		stageKeyHelp, stageKeyQuit, stageKeyCommit, stageKeyOutput, stageKeyGoWorktree, stageKeyGoLog, stageKeyGoStatus, stageKeyLog,
		stageKeyYank, stageKeyPull, stageKeyPush, stageKeyRebase, stageKeyAmend,
	}
	search := []key.Binding{stageKeySearch, stageKeySearchNext, stageKeySearchPrev}
	status := []key.Binding{
		stageKeyUp, stageKeyDown, stageKeyTop, stageKeyBottom,
		stageKeyPageUp, stageKeyPageDown, stageKeyLeft, stageKeyRight,
		stageKeyStage, stageKeyDiscard, stageKeyEdit, stageKeyOpenDiff,
		stageKeyContextDec, stageKeyContextInc, stageKeyRefresh,
	}
	diff := []key.Binding{
		stageKeyDiffBack, stageKeyTop, stageKeyBottom,
		stageKeyPageUp, stageKeyPageDown, stageKeyTab, stageKeyMode, stageKeyVisual,
		stageKeyUp, stageKeyDown, stageKeyPrevFile, stageKeyNextFile,
		stageKeyScrollDown, stageKeyScrollUp, stageKeyRenderMode,
		stageKeyContextDec, stageKeyContextInc, stageKeyStage, stageKeyDiscard,
		stageKeyEdit, stageKeyFullscreen, stageKeyWrap, stageKeyRefresh,
	}
	if k.m.focus == focusStatus {
		return [][]key.Binding{global, status, search}
	}
	return [][]key.Binding{global, diff, search}
}

func (m Model) helpShortView() string {
	return m.help.ShortHelpView(m.currentKeyMap().ShortHelp())
}

func (m Model) helpFullView(width int) string {
	h := m.help
	h.ShowAll = true
	if width > 0 {
		h.SetWidth(width)
	}
	return h.View(m.currentKeyMap())
}

func (m Model) inlineHints(bindings ...key.Binding) string {
	return ui.RenderInlineBindings(bindings...)
}

func (m Model) helpSectionLabel() string {
	if m.focus == focusStatus {
		return "status"
	}
	return fmt.Sprintf("diff:%s:%s", m.navModeLabel(), m.renderModeLabel())
}

func (m Model) navModeLabel() string {
	if m.navMode == navLine {
		return "line"
	}
	return "hunk"
}
