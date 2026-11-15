package sessions

import (
	"github.com/elentok/gx/commandlist"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/modelrules"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"
)

// setLatestModel fills the latest-model fields of info from the cached model
// list. It reads memory only: the cache, and the global agents settings. The
// project's own overrides sit in a file, which is not read on every list.
func (mg *Manager) setLatestModel(info *wire.SessionInfo, harnessName string) {
	list, ok := mg.chat.Models().Entries(harnessName)
	if !ok {
		return
	}
	latest, ok := modelrules.Latest(info.Model, list)
	if !ok {
		return
	}
	info.LatestModel = latest
	info.LatestEffort = latestEffort(mg.chat.Models(), harnessName, latest, info.Effort)
	if mg.settings == nil {
		return
	}
	set := mg.settings.Load()
	agents := config.AgentsConfig{Claude: set.AgentsClaude, Codex: set.AgentsCodex}
	kind := agent.AgentFor(harnessName)
	for _, tier := range agent.AllTiers {
		if configured, _ := agent.ResolveTier(agents, kind, tier); sameModel(list, configured, info.Model) {
			info.LatestTiers = append(info.LatestTiers, string(tier))
		}
	}
}

// latestEffort is the effort a switch to latest would run with.
func latestEffort(models *commandlist.ModelCache, harnessName, latest, current string) string {
	all, _ := models.Models(harnessName)
	for _, m := range all {
		if m.Value == latest {
			return modelrules.Effort(current, m.Efforts, harnessName == string(harness.Codex), m.DefaultEffort)
		}
	}
	return ""
}
