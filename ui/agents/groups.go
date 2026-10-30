package agents

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/ui"
)

// agentGroup is the sidebar group a session lists under.
type agentGroup int

const (
	groupRegular agentGroup = iota
	groupQueue
	groupDesign
)

func groupOf(s server.SessionInfo) agentGroup {
	switch {
	case s.Queue:
		return groupQueue
	case s.Design:
		return groupDesign
	}
	return groupRegular
}

func (g agentGroup) title() string {
	if g == groupQueue {
		return "Queue agent"
	}
	return "Design agent"
}

// groupsFile is where the shown Queue and Design groups are remembered; empty
// when the state dir is unknown.
func groupsFile(dirs config.Dirs) string {
	dir, err := dirs.StateDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "agents-groups")
}

// loadGroups reads the groups shown by an earlier TUI: one name per line.
func loadGroups(path string) (queue, design bool) {
	data, err := os.ReadFile(path)
	if path == "" || err != nil {
		return false, false
	}
	for _, line := range strings.Fields(string(data)) {
		queue = queue || line == "queue"
		design = design || line == "design"
	}
	return queue, design
}

func (m Model) saveGroups() {
	if m.groupsPath == "" {
		return
	}
	var out string
	if m.showQueue {
		out += "queue\n"
	}
	if m.showDesign {
		out += "design\n"
	}
	_ = os.MkdirAll(filepath.Dir(m.groupsPath), 0o755)
	_ = os.WriteFile(m.groupsPath, []byte(out), 0o644)
}

func (m Model) groupShown(g agentGroup) bool {
	switch g {
	case groupQueue:
		return m.showQueue
	case groupDesign:
		return m.showDesign
	}
	return true
}

// onSidebar: the session's group is shown, or it is the selected one, which
// stays on screen so an agent opened on purpose never vanishes.
func (m Model) onSidebar(i int) bool {
	return i == m.cursor || m.groupShown(groupOf(m.sessions[i]))
}

func (m Model) hiddenCount(g agentGroup) int {
	n := 0
	for i, s := range m.sessions {
		if groupOf(s) == g && !m.onSidebar(i) {
			n++
		}
	}
	return n
}

// stepCursor moves the cursor one session in dir (+1 or -1) over those on the
// sidebar.
func (m Model) stepCursor(dir int) int {
	for i := m.cursor + dir; i >= 0 && i < len(m.sessions); i += dir {
		if m.onSidebar(i) {
			return i
		}
	}
	return m.cursor
}

// hiddenRow is the dim row standing in for a hidden group.
func hiddenRow(g agentGroup, n int) sidebarRow {
	label := strconv.Itoa(n) + " " + g.title()
	if n != 1 {
		label += "s"
	}
	return sidebarRow{text: " " + ui.StyleMuted.Render(label+" hidden"), session: -1}
}

// toggleGroup runs the second key of the `t` chord; other keys drop it.
func (m Model) toggleGroup(k string) Model {
	switch k {
	case "q":
		m.showQueue = !m.showQueue
	case "d":
		m.showDesign = !m.showDesign
	default:
		return m
	}
	m.saveGroups()
	return m
}
