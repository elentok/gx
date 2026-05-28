package diffarea

import (
	"github.com/elentok/gx/ui/diffview"
	"github.com/elentok/gx/ui/keys"

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
	ActiveSection    Section
	navMode          diffview.NavMode
	renderMode       diffview.RenderMode
	Fullscreen       bool
	wrap             bool
	useNerdFontIcons bool
	Unstaged         diffview.Model
	Staged           diffview.Model
	Flash            FlashState
	keys             keys.Manager
}

func NewModel(useNerdFontIcons bool) Model {
	area := Model{
		ActiveSection:    SectionUnstaged,
		navMode:          diffview.NavModeHunk,
		renderMode:       diffview.RenderModeUnified,
		wrap:             true,
		useNerdFontIcons: useNerdFontIcons,
		Unstaged:         diffview.NewModel(useNerdFontIcons),
		Staged:           diffview.NewModel(useNerdFontIcons),
		keys:             keys.New(diffBindings),
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
	d.ActiveSectionModel().DisableVisual()
}

func (d *Model) ToggleVisual() bool {
	return d.ActiveSectionModel().ToggleVisual()
}

func (d *Model) ToggleSection() {
	if d.ActiveSection == SectionUnstaged {
		d.ActiveSection = SectionStaged
		return
	}
	d.ActiveSection = SectionUnstaged
}

func (d *Model) ResetSections() {
	d.Unstaged = diffview.NewModel(d.useNerdFontIcons)
	d.Staged = diffview.NewModel(d.useNerdFontIcons)
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

func (d *Model) moveActive(delta int) bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.MoveActive(delta, true)
}

func (d *Model) scrollPage(delta int) {
	diffviewModel := d.ActiveSectionModel()
	diffviewModel.ScrollPage(delta)
}

func (d *Model) jumpTop() bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.JumpTop()
}

func (d *Model) jumpBottom() bool {
	diffviewModel := d.ActiveSectionModel()
	return diffviewModel.JumpBottom()
}

func (d *Model) UpdateActive(msg tea.Msg) (tea.Cmd, diffview.UpdateResult) {
	active := d.ActiveSectionModel()
	updated, cmd, result := active.Update(msg)
	if !result.Handled {
		return nil, diffview.UpdateResult{}
	}
	*active = updated
	return cmd, result
}

func (d *Model) Keys() *keys.Manager {
	return &d.keys
}
