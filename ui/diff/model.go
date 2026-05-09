package diff

import (
	"strings"

	"github.com/elentok/gx/ui/diff/diffrender"
	"github.com/elentok/gx/ui/explorer"
	"github.com/elentok/gx/ui/search"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
)

// Model owns one diff pane state (unstaged or staged), including local search.
type Model struct {
	data     explorer.SectionData
	viewport viewport.Model
	search   search.Model
}

func NewModel() Model {
	return Model{
		data:     explorer.NewSectionData(),
		viewport: viewport.New(),
		search:   search.NewModel(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Data() explorer.SectionData {
	return m.data
}

func (m *Model) SetData(data explorer.SectionData) {
	m.data = data
}

func (m Model) Viewport() viewport.Model {
	return m.viewport
}

func (m *Model) Search() *search.Model {
	return &m.search
}

func (m Model) HasContent() bool {
	return len(m.data.ViewLines) > 0 || diffrender.SectionHasBinaryDiff(m.data.Parsed)
}

func (m *Model) BuildFromRaw(raw, color string, sideBySide bool) {
	prevOffset := m.viewport.YOffset()
	m.data = explorer.BuildSectionData(raw, color, m.data, sideBySide)

	if strings.TrimSpace(raw) == "" {
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)
		return
	}

	m.viewport.SetContentLines(m.data.ViewLines)
	m.viewport.SetYOffset(prevOffset)
}

func (m *Model) Reflow(wrapWidth int, wrapSoft bool) {
	prevOffset := m.viewport.YOffset()
	explorer.ReflowSectionData(&m.data, wrapWidth, wrapSoft)
	if len(m.data.BaseLines) == 0 {
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)
		return
	}
	m.viewport.SetContentLines(m.data.ViewLines)
	m.viewport.SetYOffset(prevOffset)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if nextSearch, cmd, handled := m.search.Update(msg); handled {
		m.search = nextSearch
		return m, cmd
	}
	return m, nil
}
