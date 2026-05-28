package diffview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/elentok/gx/ui/diffview/diffrender"
	"github.com/elentok/gx/ui/keys"
	"github.com/elentok/gx/ui/list"
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

// UpdateResult is returned by Model.Update to indicate how the message was handled.
type UpdateResult struct {
	Handled         bool
	NeedsReload     bool // render mode was toggled; caller must reload diff content
	ChordInProgress bool // child consumed the first key of a multi-key chord and expects the completing key next
}

const (
	navBindingMoveDown   keys.BindingID = "move-down"
	navBindingMoveUp     keys.BindingID = "move-up"
	navBindingScrollDown keys.BindingID = "scroll-down"
	navBindingScrollUp   keys.BindingID = "scroll-up"
	navBindingPageDown   keys.BindingID = "page-down"
	navBindingPageUp     keys.BindingID = "page-up"
	navBindingNavMode    keys.BindingID = "nav-mode"
	navBindingVisual     keys.BindingID = "visual"
	navBindingWrap       keys.BindingID = "wrap"
	navBindingBottom     keys.BindingID = "bottom"
	navBindingTop        keys.BindingID = "top"
	navBindingRenderMode keys.BindingID = "render-mode"
)

func newNavManager() keys.Manager {
	return keys.New([]keys.Binding{
		{ID: navBindingMoveDown, Seq: []string{"j"}},
		{ID: navBindingMoveDown, Seq: []string{"down"}},
		{ID: navBindingMoveUp, Seq: []string{"k"}},
		{ID: navBindingMoveUp, Seq: []string{"up"}},
		{ID: navBindingScrollDown, Seq: []string{"J"}},
		{ID: navBindingScrollUp, Seq: []string{"K"}},
		{ID: navBindingPageDown, Seq: []string{"ctrl+d"}},
		{ID: navBindingPageUp, Seq: []string{"ctrl+u"}},
		{ID: navBindingNavMode, Seq: []string{"a"}},
		{ID: navBindingVisual, Seq: []string{"v"}},
		{ID: navBindingWrap, Seq: []string{"w"}},
		{ID: navBindingBottom, Seq: []string{"G"}},
		{ID: navBindingBottom, Seq: []string{"shift+g"}},
		{ID: navBindingTop, Seq: []string{"g", "g"}},
		{ID: navBindingRenderMode, Seq: []string{"s"}},
	})
}

// Model owns one diff pane state (unstaged or staged), including local search.
type Model struct {
	data             DiffData
	viewport         viewport.Model
	search           search.Model
	keys             keys.Manager
	renderMode       RenderMode
	navMode          NavMode
	wrapSoft         bool
	useNerdFontIcons bool
}

func NewModel(useNerdFontIcons bool) Model {
	return Model{
		data:             NewDiffData(),
		viewport:         viewport.New(),
		search:           search.NewModel(),
		keys:             newNavManager(),
		renderMode:       RenderModeUnified,
		navMode:          NavModeHunk,
		wrapSoft:         true,
		useNerdFontIcons: useNerdFontIcons,
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

func (m Model) HasPendingChord() bool {
	return len(m.keys.Prefix()) > 0
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
	active := m.data.ActiveRawLineIndex(navMode)
	if active >= 0 {
		display := active
		if active < len(m.data.RawToDisplay) && m.data.RawToDisplay[active] >= 0 {
			display = m.data.RawToDisplay[active]
		}
		m.viewport.EnsureVisible(display, 0, 0)
	}
}

func (m Model) ComputeSearchMatches(query string) []DiffSearchMatch {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	matches := make([]DiffSearchMatch, 0)
	for i := 0; i < len(m.data.ViewLines) && i < len(m.data.DisplayToRaw); i++ {
		line := strings.ToLower(ansi.Strip(m.data.ViewLines[i]))
		if strings.Contains(line, q) {
			matches = append(matches, DiffSearchMatch{
				DisplayIndex: i,
				RawIndex:     m.data.DisplayToRaw[i],
			})
		}
	}
	return matches
}

func (m Model) ActiveRawLineIndex() int {
	return m.data.ActiveRawLineIndex(m.navMode)
}

func (m *Model) visibleRows(bodyH int, active bool) []visibleDiffRow {
	viewportY := m.viewport.YOffset()
	activeRaw := m.ActiveRawLineIndex()
	data := m.data

	rows := make([]visibleDiffRow, 0, maxInt(0, bodyH))
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
		if start, end, ok := data.HunkDisplayBounds(data.ActiveHunk); ok {
			vpBottom := viewportY + bodyH - 1
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
			rows = append(rows, visibleDiffRow{DisplayIndex: displayIdx, RawIndex: -1})
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

		text := data.ViewLines[displayIdx]
		rows = append(rows, visibleDiffRow{
			DisplayIndex:       displayIdx,
			RawIndex:           rawIdx,
			Text:               text,
			Kind:               rowKind,
			InActiveHunk:       inActiveHunk,
			IsActiveRaw:        rawIdx >= 0 && rawIdx == activeRaw && active,
			IsActiveChangedRaw: isChanged,
			OverflowTop:        displayIdx == overflowTopDisplay && inActiveHunk,
			OverflowBottom:     displayIdx == overflowBottomDisplay && inActiveHunk,
			IsSeparator:        isSeparatorRow(text, m.renderMode),
		})
	}

	return rows
}

func (m *Model) moveActive(delta int, allowViewportScroll bool) bool {
	if m.navMode == NavModeHunk {
		if len(m.data.Parsed.Hunks) == 0 {
			return false
		}
		old := m.data.ActiveHunk
		if allowViewportScroll && m.data.ActiveHunk >= 0 && m.data.ActiveHunk < len(m.data.Parsed.Hunks) {
			if start, end, ok := m.data.HunkDisplayBounds(m.data.ActiveHunk); ok {
				visible := m.viewport.VisibleLineCount()
				y := m.viewport.YOffset()
				if visible > 0 {
					last := y + visible - 1
					if delta > 0 && end >= last {
						m.viewport.ScrollDown(1)
						return false
					}
					if delta < 0 && start <= y {
						m.viewport.ScrollUp(1)
						return false
					}
				}
			}
		}
		m.data.ActiveHunk += delta
		if m.data.ActiveHunk < 0 {
			m.data.ActiveHunk = 0
		}
		if m.data.ActiveHunk >= len(m.data.Parsed.Hunks) {
			m.data.ActiveHunk = len(m.data.Parsed.Hunks) - 1
		}
		return m.data.ActiveHunk != old
	}

	if len(m.data.Parsed.Changed) == 0 {
		return false
	}
	old := m.data.ActiveLine
	m.data.ActiveLine += delta
	if m.data.ActiveLine < 0 {
		m.data.ActiveLine = 0
	}
	if m.data.ActiveLine >= len(m.data.Parsed.Changed) {
		m.data.ActiveLine = len(m.data.Parsed.Changed) - 1
	}
	return m.data.ActiveLine != old
}

// ScrollPage scrolls the viewport by delta display lines and co-scrolls the
// active hunk/line to the nearest one at the new display position (vim-style
// ctrl+d/ctrl+u). In unified mode (HunkDisplayRange/ChangedDisplay are nil)
// only the viewport scrolls.
func (m *Model) scrollPage(delta int) {
	if delta > 0 {
		m.viewport.ScrollDown(delta)
	} else if delta < 0 {
		m.viewport.ScrollUp(-delta)
	} else {
		return
	}
	m.coScrollActive(delta)
}

func (m *Model) ScrollViewport(delta int) {
	if delta > 0 {
		m.viewport.ScrollDown(delta)
	} else if delta < 0 {
		m.viewport.ScrollUp(-delta)
	} else {
		return
	}
	m.snapActiveToViewport()
}

func (m *Model) snapActiveToViewport() {
	yOffset := m.viewport.YOffset()
	visibleH := m.viewport.VisibleLineCount()
	if visibleH <= 0 {
		return
	}
	bottom := yOffset + visibleH

	if m.navMode == NavModeHunk {
		if len(m.data.Parsed.Hunks) == 0 {
			return
		}
		if m.data.ActiveHunk >= 0 && m.data.ActiveHunk < len(m.data.HunkDisplayRange) {
			r := m.data.HunkDisplayRange[m.data.ActiveHunk]
			if r[0] >= yOffset && r[0] < bottom {
				return // still visible
			}
			if r[0] < yOffset {
				// active is above — find first hunk visible
				for i, hr := range m.data.HunkDisplayRange {
					if hr[0] >= yOffset {
						m.data.ActiveHunk = i
						return
					}
				}
				m.data.ActiveHunk = len(m.data.Parsed.Hunks) - 1
			} else {
				// active is below — find last hunk visible
				last := 0
				for i, hr := range m.data.HunkDisplayRange {
					if hr[0] < bottom {
						last = i
					}
				}
				m.data.ActiveHunk = last
			}
		}
		return
	}

	// NavModeLine
	if len(m.data.Parsed.Changed) == 0 {
		return
	}
	if m.data.ActiveLine >= 0 && m.data.ActiveLine < len(m.data.ChangedDisplay) {
		displayRow := m.data.ChangedDisplay[m.data.ActiveLine]
		if displayRow >= yOffset && displayRow < bottom {
			return // still visible
		}
		if displayRow < yOffset {
			// active is above — find first changed line visible
			for i, d := range m.data.ChangedDisplay {
				if d >= yOffset {
					m.data.ActiveLine = i
					return
				}
			}
			m.data.ActiveLine = len(m.data.Parsed.Changed) - 1
		} else {
			// active is below — find last changed line visible
			last := 0
			for i, d := range m.data.ChangedDisplay {
				if d < bottom {
					last = i
				}
			}
			m.data.ActiveLine = last
		}
	}
}

// coScrollActive moves the active hunk/line to the nearest one at activeDisplay+delta.
// Works in both unified and side-by-side modes.
func (m *Model) coScrollActive(delta int) {
	if m.navMode == NavModeHunk {
		// Use HunkDisplayRange count in side-by-side, Parsed.Hunks count in unified.
		n := len(m.data.HunkDisplayRange)
		if n == 0 {
			n = len(m.data.Parsed.Hunks)
		}
		if n == 0 {
			return
		}
		activeDisplay := 0
		if start, _, ok := m.data.HunkDisplayBounds(m.data.ActiveHunk); ok {
			activeDisplay = start
		}
		m.data.ActiveHunk = nearestIndex(n, func(i int) int {
			start, _, ok := m.data.HunkDisplayBounds(i)
			if !ok {
				return -1
			}
			return start
		}, activeDisplay+delta)
		return
	}

	// Use ChangedDisplay count in side-by-side, Parsed.Changed count in unified.
	n := len(m.data.ChangedDisplay)
	if n == 0 {
		n = len(m.data.Parsed.Changed)
	}
	if n == 0 {
		return
	}
	activeDisplay := m.changedLineDisplay(m.data.ActiveLine)
	m.data.ActiveLine = nearestIndex(n, func(i int) int {
		return m.changedLineDisplay(i)
	}, activeDisplay+delta)
}

// changedLineDisplay returns the display row for changed line i, using
// ChangedDisplay (side-by-side) or RawToDisplay (unified) as available.
func (m *Model) changedLineDisplay(i int) int {
	if i < 0 {
		return 0
	}
	if i < len(m.data.ChangedDisplay) {
		return m.data.ChangedDisplay[i]
	}
	if i < len(m.data.Parsed.Changed) {
		rawIdx := m.data.Parsed.Changed[i].LineIndex
		if rawIdx >= 0 && rawIdx < len(m.data.RawToDisplay) {
			return m.data.RawToDisplay[rawIdx]
		}
	}
	return 0
}

// nearestIndex returns the index in [0, n) whose displayAt value is closest to target.
func nearestIndex(n int, displayAt func(i int) int, target int) int {
	if n == 0 {
		return 0
	}
	best := 0
	bestDist := absInt(displayAt(0) - target)
	for i := 1; i < n; i++ {
		d := absInt(displayAt(i) - target)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (m *Model) jumpTop() bool {
	m.viewport.SetYOffset(0)
	if m.navMode == NavModeHunk {
		if len(m.data.Parsed.Hunks) == 0 {
			return false
		}
		m.data.ActiveHunk = 0
		return true
	}
	if len(m.data.Parsed.Changed) == 0 {
		return false
	}
	m.data.ActiveLine = 0
	return true
}

func (m *Model) jumpBottom() bool {
	maxOffset := m.viewport.TotalLineCount() - m.viewport.VisibleLineCount()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.viewport.SetYOffset(maxOffset)
	if m.navMode == NavModeHunk {
		if len(m.data.Parsed.Hunks) == 0 {
			return false
		}
		m.data.ActiveHunk = len(m.data.Parsed.Hunks) - 1
		return true
	}
	if len(m.data.Parsed.Changed) == 0 {
		return false
	}
	m.data.ActiveLine = len(m.data.Parsed.Changed) - 1
	return true
}

func (m *Model) ApplySearchMatch(match search.Match) {
	applyDiffSearchMatch(&m.data, &m.viewport, match)
}

func (m *Model) FocusSearchMatch(match search.Match) {
	m.navMode = NavModeLine
	m.ApplySearchMatch(match)
}

func (m Model) CurrentSearchMatchIndex(matches []DiffSearchMatch) int {
	if m.data.ActiveLine < 0 || m.data.ActiveLine >= len(m.data.Parsed.Changed) {
		return -1
	}
	raw := m.data.Parsed.Changed[m.data.ActiveLine].LineIndex
	for i, match := range matches {
		if match.RawIndex == raw {
			return i
		}
	}
	return -1
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
	hunkIdx := ActiveHunkIndexForYank(m.data, m.navMode)
	if hunkIdx < 0 || hunkIdx >= len(m.data.Parsed.Hunks) {
		return "", nil, FocusedYankErrNoHunk
	}
	body := FocusedYankBody(m.data, m.navMode)
	if len(body) == 0 {
		return "", nil, FocusedYankErrNoLines
	}
	loc := FocusedLocation(m.data, m.navMode)
	return loc, body, ""
}

// ScrollPercentText returns a "N%" string when the viewport is scrollable,
// or "" when all content fits on screen.
func (m Model) ScrollPercentText() string {
	vp := &m.viewport
	if vp.TotalLineCount() <= vp.VisibleLineCount() || vp.VisibleLineCount() <= 0 {
		return ""
	}
	pct := int(vp.ScrollPercent()*100 + 0.5)
	return fmt.Sprintf("%d%%", pct)
}

// StatusText returns the model-owned right-title fragments for a diff panel:
// nav mode (when focused) and scroll percentage. Callers prepend context lines
// and append search counters using JoinDot.
func (m Model) StatusText(focused bool) string {
	parts := []string{}
	if focused {
		mode := "hunk"
		if m.navMode == NavModeLine {
			mode = "line"
		}
		parts = append(parts, mode)
	}
	if pct := m.ScrollPercentText(); pct != "" {
		parts = append(parts, pct)
	}
	return JoinDot(parts...)
}

// JoinDot joins non-empty strings with " · ".
func JoinDot(parts ...string) string {
	filtered := parts[:0]
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, " · ")
}

func (m *Model) DisableVisual() {
	m.data.VisualActive = false
	m.data.VisualAnchor = m.data.ActiveLine
}

func (m *Model) toggleVisual() bool {
	if len(m.data.Parsed.Changed) == 0 {
		return false
	}
	if !m.data.VisualActive {
		m.data.VisualActive = true
		m.data.VisualAnchor = m.data.ActiveLine
		return true
	}
	m.data.VisualActive = false
	m.data.VisualAnchor = m.data.ActiveLine
	return true
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, UpdateResult) {
	if nextSearch, cmd, result := m.search.Update(msg); result.Handled {
		m.search = nextSearch
		return m, cmd, UpdateResult{Handled: true}
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, UpdateResult{}
	}

	match, consumed := m.keys.Process(key)
	if !consumed {
		return m, nil, UpdateResult{}
	}
	if match == nil {
		return m, nil, UpdateResult{Handled: true, ChordInProgress: true}
	}

	result := UpdateResult{Handled: true}
	switch match.ID {
	case navBindingMoveDown:
		if m.moveActive(1, true) {
			m.EnsureActiveVisible(m.navMode)
		}
	case navBindingMoveUp:
		if m.moveActive(-1, true) {
			m.EnsureActiveVisible(m.navMode)
		}
	case navBindingScrollDown:
		m.ScrollViewport(3)
	case navBindingScrollUp:
		m.ScrollViewport(-3)
	case navBindingPageDown:
		m.scrollPage(list.DefaultScroll)
	case navBindingPageUp:
		m.scrollPage(-list.DefaultScroll)
	case navBindingNavMode:
		m.DisableVisual()
		if m.navMode == NavModeHunk {
			m.navMode = NavModeLine
		} else {
			m.navMode = NavModeHunk
		}
		m.EnsureActiveVisible(m.navMode)
	case navBindingVisual:
		if m.navMode == NavModeHunk {
			m.navMode = NavModeLine
		}
		m.toggleVisual()
		m.EnsureActiveVisible(m.navMode)
	case navBindingWrap:
		m.wrapSoft = !m.wrapSoft
	case navBindingBottom:
		if m.jumpBottom() {
			m.EnsureActiveVisible(m.navMode)
		}
	case navBindingTop:
		if m.jumpTop() {
			m.EnsureActiveVisible(m.navMode)
		}
	case navBindingRenderMode:
		if m.renderMode == RenderModeUnified {
			m.renderMode = RenderModeSideBySide
		} else {
			m.renderMode = RenderModeUnified
		}
		result.NeedsReload = true
	}

	return m, nil, result
}
