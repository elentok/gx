package diffarea

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keybindings"

	tea "charm.land/bubbletea/v2"
)

type Section int

const (
	SectionUnstaged Section = iota
	SectionStaged
)

type FlashState struct {
	Active  bool
	Section Section
	NavMode diffview.NavMode
	Hunk    int
	Line    int
	Frames  int
}

type Model struct {
	ActiveSection  Section
	navMode        diffview.NavMode
	renderMode     diffview.RenderMode
	Fullscreen     bool
	wrap           bool
	Unstaged       diffview.Model
	Staged         diffview.Model
	Flash          FlashState
	keys           keybindings.Manager
}

func NewModel() Model {
	area := Model{
		ActiveSection: SectionUnstaged,
		navMode:       diffview.NavModeHunk,
		renderMode:    diffview.RenderModeUnified,
		wrap:          true,
		Unstaged:      diffview.NewModel(),
		Staged:        diffview.NewModel(),
		keys:          keybindings.New(diffBindings),
	}
	area.SetRenderMode(area.renderMode)
	area.SetNavMode(area.navMode)
	area.SetWrap(area.wrap)
	return area
}

func (d Model) RenderMode() diffview.RenderMode {
	return d.renderMode
}

func (d *Model) SetRenderMode(mode diffview.RenderMode) {
	d.renderMode = mode
	d.Unstaged.SetRenderMode(mode)
	d.Staged.SetRenderMode(mode)
}

func (d Model) NavMode() diffview.NavMode {
	return d.navMode
}

func (d *Model) SetNavMode(mode diffview.NavMode) {
	d.navMode = mode
	d.Unstaged.SetNavMode(mode)
	d.Staged.SetNavMode(mode)
}

func (d *Model) ToggleNavMode() {
	if d.navMode == diffview.NavModeHunk {
		d.SetNavMode(diffview.NavModeLine)
		return
	}
	d.SetNavMode(diffview.NavModeHunk)
}

func (d Model) Wrap() bool {
	return d.wrap
}

func (d *Model) SetWrap(enabled bool) {
	d.wrap = enabled
	d.Unstaged.EnableWrap(enabled)
	d.Staged.EnableWrap(enabled)
}

func (d *Model) SectionModel(section Section) *diffview.Model {
	if section == SectionStaged {
		return &d.Staged
	}
	return &d.Unstaged
}

func (d *Model) ActiveSectionModel() *diffview.Model {
	return d.SectionModel(d.ActiveSection)
}

func (d *Model) DisableVisual() {
	active := d.ActiveSectionModel().DataRef()
	active.VisualActive = false
	active.VisualAnchor = active.ActiveLine
}

func (d *Model) ToggleVisual() bool {
	active := d.ActiveSectionModel().DataRef()
	if len(active.Parsed.Changed) == 0 {
		return false
	}
	if !active.VisualActive {
		active.VisualActive = true
		active.VisualAnchor = active.ActiveLine
		return true
	}
	active.VisualActive = false
	active.VisualAnchor = active.ActiveLine
	return true
}

func (d *Model) ToggleSection() {
	if d.ActiveSection == SectionUnstaged {
		d.ActiveSection = SectionStaged
		return
	}
	d.ActiveSection = SectionUnstaged
}

func (d *Model) ResetSections() {
	d.Unstaged = diffview.NewModel()
	d.Staged = diffview.NewModel()
	d.SetRenderMode(d.renderMode)
	d.SetNavMode(d.navMode)
	d.SetWrap(d.wrap)
}

func (d *Model) SyncViewports(vpW, expandedH, collapsedH int) {
	unstagedH, stagedH := expandedH-3, collapsedH-3
	if d.ActiveSection == SectionStaged {
		unstagedH, stagedH = collapsedH-3, expandedH-3
	}
	d.Unstaged.SyncViewport(vpW, max(0, unstagedH))
	d.Staged.SyncViewport(vpW, max(0, stagedH))
}

func (d *Model) MoveActive(delta int) bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.MoveActive(delta, true)
}

func (d *Model) ScrollPage(direction int) {
	diffviewModel := d.ActiveSectionModel()
	diffviewModel.ScrollPage(direction)
}

func (d *Model) JumpTop() bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.JumpTop()
}

func (d *Model) JumpBottom() bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.JumpBottom()
}

func (d *Model) UpdateActive(msg tea.Msg) (tea.Cmd, bool) {
	active := d.ActiveSectionModel()
	updated, cmd, handled := active.Update(msg)
	if !handled {
		return nil, false
	}
	*active = updated
	return cmd, true
}

const (
	diffCat = "Diff"

	BindingMoveDown   keybindings.BindingID = "move-down"
	BindingMoveUp     keybindings.BindingID = "move-up"
	BindingScrollDown keybindings.BindingID = "scroll-down"
	BindingScrollUp   keybindings.BindingID = "scroll-up"
	BindingPageDown   keybindings.BindingID = "page-down"
	BindingPageUp     keybindings.BindingID = "page-up"
	BindingNavMode    keybindings.BindingID = "nav-mode"
	BindingVisual     keybindings.BindingID = "visual"
	BindingFullscreen keybindings.BindingID = "fullscreen"
	BindingWrap       keybindings.BindingID = "wrap"
	BindingSearchNext keybindings.BindingID = "search-next"
	BindingSearchPrev keybindings.BindingID = "search-prev"
	BindingBack       keybindings.BindingID = "back"
	BindingApply      keybindings.BindingID = "apply"
	BindingDiscard    keybindings.BindingID = "discard"
	BindingNextFile   keybindings.BindingID = "next-file"
	BindingPrevFile   keybindings.BindingID = "prev-file"
)

var diffBindings = []keybindings.Binding{
	{ID: BindingMoveDown, Seq: []string{"j"}, Categories: []string{diffCat}, Title: "move down", Display: "↓/j"},
	{ID: BindingMoveUp, Seq: []string{"k"}, Categories: []string{diffCat}, Title: "move up", Display: "↑/k"},
	{ID: BindingScrollDown, Seq: []string{"J"}, Categories: []string{diffCat}, Title: "scroll down"},
	{ID: BindingScrollUp, Seq: []string{"K"}, Categories: []string{diffCat}, Title: "scroll up"},
	{ID: BindingPageDown, Seq: []string{"ctrl+d"}, Categories: []string{diffCat}, Title: "half page down"},
	{ID: BindingPageUp, Seq: []string{"ctrl+u"}, Categories: []string{diffCat}, Title: "half page up"},
	{ID: BindingNavMode, Seq: []string{"a"}, Categories: []string{diffCat}, Title: "toggle hunk/line mode"},
	{ID: BindingVisual, Seq: []string{"v"}, Categories: []string{diffCat}, Title: "visual mode"},
	{ID: BindingFullscreen, Seq: []string{"f"}, Categories: []string{diffCat}, Title: "fullscreen"},
	{ID: BindingWrap, Seq: []string{"w"}, Categories: []string{diffCat}, Title: "soft wrap"},
	{ID: BindingSearchNext, Seq: []string{"n"}, Categories: []string{diffCat}, Title: "next match"},
	{ID: BindingSearchPrev, Seq: []string{"N"}, Categories: []string{diffCat}, Title: "prev match"},
	{ID: BindingMoveDown, Seq: []string{"down"}, Categories: []string{}, Title: ""},
	{ID: BindingMoveUp, Seq: []string{"up"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"esc"}, Categories: []string{diffCat}, Title: "back to filetree"},
	{ID: BindingBack, Seq: []string{"q"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"h"}, Categories: []string{}, Title: ""},
	{ID: BindingBack, Seq: []string{"left"}, Categories: []string{}, Title: ""},
	{ID: BindingApply, Seq: []string{"space"}, Categories: []string{diffCat}, Title: "apply selection"},
	{ID: BindingDiscard, Seq: []string{"d"}, Categories: []string{diffCat}, Title: "discard"},
	{ID: BindingNextFile, Seq: []string{"."}, Categories: []string{diffCat}, Title: "next file"},
	{ID: BindingPrevFile, Seq: []string{","}, Categories: []string{diffCat}, Title: "prev file"},
}

func (d *Model) Keys() *keybindings.Manager {
	return &d.keys
}
