package ralphloop

import (
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/herdr"
)

// FindIterationTab returns identifier's live iteration tab in epicName's
// herdr workspace, for a caller that needs to focus or inspect it (the
// tickets tab's needs-answer menu). Any lookup failure reads as "no live
// tab": a transient herdr hiccup must only demote the menu to its
// pane-gone variant, never block it.
func FindIterationTab(
	findWorkspace func(label string) (string, error),
	tabList func(workspaceID string) ([]herdr.Tab, error),
	epicName, identifier string,
) (herdr.Tab, bool) {
	workspaceID, err := findWorkspace(epicName)
	if err != nil || workspaceID == "" {
		return herdr.Tab{}, false
	}
	tab, found, err := herdrrunner.FindTab(tabList, workspaceID, iterLabel(epicName, identifier))
	return tab, found && err == nil
}
