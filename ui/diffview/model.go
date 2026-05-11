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
	data       DiffData
	viewport   viewport.Model
	search     search.Model
	renderMode RenderMode
	navMode    NavMode
	wrapSoft   bool
}

func NewModel() Model {
	return Model{
		data:       NewDiffData(),
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

func (m Model) Data() DiffData {
	return m.data
}

func (m *Model) DataRef() *DiffData {
	return &m.data
}

func (m *Model) SetData(data DiffData) {
	m.data = data
}

func (m *Model) Viewport() *viewport.Model {
	return &m.viewport
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

func (m *Model) BuildFromRaw(raw, color string) {
	prevOffset := m.viewport.YOffset()
	m.data = BuildDiffData(raw, color, m.data, m.IsSideBySide())

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
	reflowDiffData(&m.data, wrapWidth, m.wrapSoft)
	if len(m.data.BaseLines) == 0 {
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)
		return
	}
	m.viewport.SetContentLines(m.data.ViewLines)
	m.viewport.SetYOffset(prevOffset)
}

func (m *Model) SyncViewport(width, height int) {
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(height)
	m.viewport.SetContentLines(m.data.ViewLines)
}

func (m *Model) EnsureActiveVisible(navMode NavMode) {
	if navMode == NavModeHunk && m.data.ActiveHunk >= 0 && m.data.ActiveHunk < len(m.data.HunkDisplayRange) {
		r := m.data.HunkDisplayRange[m.data.ActiveHunk]
		m.viewport.EnsureVisible(r[0], 0, 0)
		return
	}
	if navMode == NavModeLine && m.data.ActiveLine >= 0 && m.data.ActiveLine < len(m.data.ChangedDisplay) && m.data.ChangedDisplay[m.data.ActiveLine] >= 0 {
		m.viewport.EnsureVisible(m.data.ChangedDisplay[m.data.ActiveLine], 0, 0)
		return
	}
	active := activeRawLineIndex(m.data, navMode)
	if active >= 0 {
		display := active
		if active < len(m.data.RawToDisplay) && m.data.RawToDisplay[active] >= 0 {
			display = m.data.RawToDisplay[active]
		}
		m.viewport.EnsureVisible(display, 0, 0)
	}
}

func (m Model) ComputeSearchMatches(query string) []DiffSearchMatch {
	return computeDiffSearchMatches(m.data.ViewLines, m.data.DisplayToRaw, query)
}

func (m Model) ActiveRawLineIndex() int {
	return activeRawLineIndex(m.data, m.navMode)
}

func (m *Model) VisibleRows(bodyH int, active bool) []VisibleDiffRow {
	viewportY := m.viewport.YOffset()
	visible := m.viewport.VisibleLineCount()
	activeRaw := m.ActiveRawLineIndex()
	data := m.data

	rows := make([]VisibleDiffRow, 0, maxInt(0, bodyH))
	if bodyH <= 0 {
		return rows
	}

	hunkStart, hunkEnd := -1, -1
	if m.navMode == NavModeHunk && data.ActiveHunk >= 0 && data.ActiveHunk < len(data.Parsed.Hunks) {
		hunkStart = data.Parsed.Hunks[data.ActiveHunk].StartLine
		hunkEnd = data.Parsed.Hunks[data.ActiveHunk].EndLine
	}

	overflowTopDisplay := -1
	overflowBottomDisplay := -1
	if m.navMode == NavModeHunk && active && data.ActiveHunk >= 0 {
		if start, end, ok := hunkDisplayBounds(data.HunkDisplayRange, data.Parsed, data.DisplayToRaw, data.ActiveHunk); ok && visible > 0 {
			vpBottom := viewportY + visible - 1
			if start < viewportY {
				overflowTopDisplay = viewportY
			}
			if end > vpBottom {
				overflowBottomDisplay = vpBottom
			}
		}
	}

	for i := 0; i < bodyH; i++ {
		displayIdx := viewportY + i
		if displayIdx >= len(data.ViewLines) {
			rows = append(rows, VisibleDiffRow{DisplayIndex: displayIdx, RawIndex: -1})
			continue
		}
		rawIdx := -1
		if displayIdx >= 0 && displayIdx < len(data.DisplayToRaw) {
			rawIdx = data.DisplayToRaw[displayIdx]
		}
		rowKind := diffrender.RowPlain
		if displayIdx >= 0 && displayIdx < len(data.ViewLineKinds) {
			rowKind = data.ViewLineKinds[displayIdx]
		}

		inActiveHunk := false
		if m.navMode == NavModeHunk {
			if len(data.HunkDisplayRange) > 0 && data.ActiveHunk >= 0 && data.ActiveHunk < len(data.HunkDisplayRange) {
				r := data.HunkDisplayRange[data.ActiveHunk]
				inActiveHunk = displayIdx >= r[0] && displayIdx <= r[1]
			} else {
				inActiveHunk = rawIdx >= 0 && rawIdx >= hunkStart && rawIdx <= hunkEnd
			}
		}

		isChanged := rawIdx < 0 && m.navMode == NavModeLine && active && data.ActiveLine >= 0 && data.ActiveLine < len(data.ChangedDisplay) && data.ChangedDisplay[data.ActiveLine] == displayIdx

		rows = append(rows, VisibleDiffRow{
			DisplayIndex:       displayIdx,
			RawIndex:           rawIdx,
			Text:               data.ViewLines[displayIdx],
			Kind:               rowKind,
			InActiveHunk:       inActiveHunk,
			IsActiveRaw:        rawIdx >= 0 && rawIdx == activeRaw && active,
			IsActiveChangedRaw: isChanged,
			OverflowTop:        displayIdx == overflowTopDisplay && inActiveHunk,
			OverflowBottom:     displayIdx == overflowBottomDisplay && inActiveHunk,
		})
	}

	return rows
}

func (m *Model) MoveActive(delta int, allowViewportScroll bool) bool {
	return moveActive(&m.data, &m.viewport, m.navMode, delta, allowViewportScroll)
}

func (m *Model) ScrollPage(direction int) {
	scrollPage(&m.viewport, direction)
}

func (m *Model) JumpTop() bool {
	return jumpTop(&m.data, &m.viewport, m.navMode)
}

func (m *Model) JumpBottom() bool {
	return jumpBottom(&m.data, &m.viewport, m.navMode)
}

func (m *Model) ApplySearchMatch(match search.Match) {
	applyDiffSearchMatch(&m.data, &m.viewport, match)
}

func (m *Model) FocusSearchMatch(match search.Match) {
	m.navMode = NavModeLine
	m.ApplySearchMatch(match)
}

func (m Model) CurrentSearchMatchIndex(matches []DiffSearchMatch) int {
	return currentDiffSearchMatchIndex(m.data, matches, NavModeLine)
}

func (m *Model) RestoreViewportYOffset(y int) {
	restoreViewportYOffset(&m.viewport, y)
}

func (m Model) CurrentSearchCursor(matches []search.Match) int {
	diffMatches := make([]DiffSearchMatch, 0, len(matches))
	for _, match := range matches {
		diffMatches = append(diffMatches, DiffSearchMatch{
			DisplayIndex: match.DisplayIndex,
			RawIndex:     match.Index,
		})
	}
	return m.CurrentSearchMatchIndex(diffMatches)
}

func (m Model) FocusedLocationAndBody() (string, []string, FocusedYankError) {
	return FocusedLocationAndBody(m.data, m.navMode)
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		return m, cmd, true
	}
	return m, nil, false
}
