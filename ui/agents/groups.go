package agents

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/ui"
)

// agentGroup is the sidebar group a session lists under.
type agentGroup int

const (
	groupRegular agentGroup = iota
	groupQueue
	groupDesign
)

func groupOf(s wire.SessionInfo) agentGroup {
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

func (s sidebar) saveGroups() {
	if s.groupsPath == "" {
		return
	}
	var out string
	if s.showQueue {
		out += "queue\n"
	}
	if s.showDesign {
		out += "design\n"
	}
	_ = os.MkdirAll(filepath.Dir(s.groupsPath), 0o755)
	_ = os.WriteFile(s.groupsPath, []byte(out), 0o644)
}

func (s sidebar) groupShown(g agentGroup) bool {
	switch g {
	case groupQueue:
		return s.showQueue
	case groupDesign:
		return s.showDesign
	}
	return true
}

// onSidebar: the session's group is shown, or it is the selected one, which
// stays on screen so an agent opened on purpose never vanishes.
func (s sidebar) onSidebar(in sidebarInput, i int) bool {
	return i == in.cursor || s.groupShown(groupOf(in.sessions[i]))
}

func (s sidebar) hiddenCount(in sidebarInput, g agentGroup) int {
	n := 0
	for i, sess := range in.sessions {
		if groupOf(sess) == g && !s.onSidebar(in, i) {
			n++
		}
	}
	return n
}

// stepCursor moves the cursor one session in dir (+1 or -1) over those on the
// sidebar.
func (s sidebar) stepCursor(in sidebarInput, dir int) int {
	for i := in.cursor + dir; i >= 0 && i < len(in.sessions); i += dir {
		if s.onSidebar(in, i) {
			return i
		}
	}
	return in.cursor
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
func (s sidebar) toggleGroup(k string) sidebar {
	switch k {
	case "q":
		s.showQueue = !s.showQueue
	case "d":
		s.showDesign = !s.showDesign
	default:
		return s
	}
	s.saveGroups()
	return s
}
