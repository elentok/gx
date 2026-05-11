package diffview

import (
	"strings"

	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/search"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type RenderMode int

const (
	RenderModeUnified RenderMode = iota
	RenderModeSideBySide
)

type NavMode int

const (
	NavModeHunk NavMode = iota
	NavModeLine
)

// Model owns one diff pane state (unstaged or staged), including local search.
type Model struct {
	data       DiffBuffer
	viewport   viewport.Model
	search     search.Model
	renderMode RenderMode
	navMode    NavMode
	wrapSoft   bool
}

func NewModel() Model {
	return Model{
		data:       NewDiffBuffer(),
		viewport:   viewport.New(),
		search:     search.NewModel(),
		renderMode: RenderModeUnified,
		navMode:    NavModeHunk,
		wrapSoft:   true,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Data() DiffBuffer {
	return m.data
}

func (m *Model) SetData(data DiffBuffer) {
	m.data = data
}

func (m *Model) Search() *search.Model {
	return &m.search
}

func (m Model) RenderMode() RenderMode {
	return m.renderMode
}

func (m *Model) SetRenderMode(mode RenderMode) {
	m.renderMode = mode
}

func (m Model) IsSideBySide() bool {
	return m.renderMode == RenderModeSideBySide
}

func (m Model) NavMode() NavMode {
	return m.navMode
}

func (m *Model) SetNavMode(mode NavMode) {
	m.navMode = mode
}

func (m Model) WrapEnabled() bool {
	return m.wrapSoft
}

func (m *Model) EnableWrap(enabled bool) {
	m.wrapSoft = enabled
}

func (m Model) HasContent() bool {
	return len(m.data.ViewLines) > 0 || diffrender.SectionHasBinaryDiff(m.data.Parsed)
}

func (m *Model) BuildFromRaw(raw, color string) {
	prevOffset := m.viewport.YOffset()
	m.data = BuildDiffBuffer(raw, color, m.data, m.IsSideBySide())

	if strings.TrimSpace(raw) == "" {
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)
		return
	}

	m.viewport.SetContentLines(m.data.ViewLines)
	m.viewport.SetYOffset(prevOffset)
}

func (m *Model) Reflow(wrapWidth int) {
	prevOffset := m.viewport.YOffset()
	ReflowDiffBuffer(&m.data, wrapWidth, m.wrapSoft)
	if len(m.data.BaseLines) == 0 {
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)
		return
	}
	m.viewport.SetContentLines(m.data.ViewLines)
	m.viewport.SetYOffset(prevOffset)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	if nextSearch, cmd, handled := m.search.Update(msg); handled {
		m.search = nextSearch
		return m, cmd, true
	}
	return m, nil, false
}
