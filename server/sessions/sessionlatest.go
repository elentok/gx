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
func setLatestModel(mg *Manager, info *wire.SessionInfo, harnessName string) {
	list, ok := mg.Models.Entries(harnessName)
	if !ok {
		return
	}
	latest, ok := modelrules.Latest(info.Model, list)
	if !ok {
		return
	}
	info.LatestModel = latest
	info.LatestFamily = latestFamily(list, latest)
	info.LatestEffort = latestEffort(mg.Models, harnessName, latest, info.Effort)
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

// latestFamily is the family of the latest model. latest may be an alias, so
// it parses the resolved id of its list entry when there is one.
func latestFamily(list []modelrules.Entry, latest string) string {
	id := latest
	for _, e := range list {
		if e.Value == latest && e.ID != "" {
			id = e.ID
			break
		}
	}
	family, _, _ := modelrules.ParseID(id)
	return family
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
