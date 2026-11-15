package registry

func minOf(v float64) *float64 { return &v }

// coreEntries lists the Ralph loop, Notifications, Web UI and System keys.
// Types and minimums mirror what config.parseWith accepts.
func coreEntries() []Entry {
	var out []Entry
	add := func(p Place, e Entry) {
		e.Category, e.Section, e.Group = p.Category, p.Section, p.Group
		e.Label = labelFor(e)
		out = append(out, e)
	}
	queueInt := func(key, desc string) Entry {
		return Entry{Key: Key("execution-queue", key), Type: TypeNumber, Min: minOf(1), Integer: true, Description: desc}
	}
	dur := func(key KeyPath, desc string) Entry {
		return Entry{Key: key, Type: TypeDuration, Description: desc}
	}
	risky := func(e Entry, confirm string) Entry {
		e.Risky, e.ConfirmText = true, confirm
		return e
	}
	ralph := func(s Section, g string) Place { return Place{CategoryRalphLoop, s, g} }
	web := func(s Section, g string) Place { return Place{CategoryWebUI, s, g} }
	system := func(s Section, g string) Place { return Place{CategorySystem, s, g} }

	limits := ralph(SectionExecutionQueue, "Agent limits")
	e := queueInt("max-agents", "The most agents that run at the same time across all projects.")
	e.ProjectKey = Key("max-agents")
	add(limits, e)
	e = queueInt("max-agents-per-epic", "The most agents that work on one epic at the same time.")
	e.ProjectOverridable = true
	add(limits, e)
	e = queueInt("max-one-off-agents", "The most agents running one-off tickets at the same time.")
	e.ProjectOverridable = true
	add(limits, e)
	waiting := ralph(SectionExecutionQueue, "Waiting and stalled agents")
	add(waiting, dur(Key("execution-queue", "waiting-slot-grace"), "How long an agent that waits for an answer keeps its slot before the slot is freed."))
	add(waiting, Entry{Key: Key("execution-queue", "max-released-agents"), Type: TypeNumber, Min: minOf(0), Integer: true,
		Description: "The most agents that may wait with their slot released. 0 means none."})
	add(waiting, dur(Key("execution-queue", "stalled-nudge-delay"), "How long an agent may be silent before it is nudged."))

	spend := ralph(SectionBudget, "Spend limits")
	add(spend, Entry{Key: Key("budget", "soft-limit"), Type: TypeNumber, Min: minOf(0),
		Description: "Estimated spend in dollars at which gx warns you. 0 turns it off."})
	add(spend, Entry{Key: Key("budget", "hard-limit"), Type: TypeNumber, Min: minOf(0),
		Description: "Estimated spend in dollars at which gx stops launching agents. 0 turns it off."})
	alerts := ralph(SectionBudget, "Alerts")
	add(alerts, Entry{Key: Key("budget", "long-ticket-turns"), Type: TypeNumber, Min: minOf(0), Integer: true,
		Description: "Number of agent turns after which a ticket counts as long and you are alerted. 0 turns it off."})
	add(alerts, Entry{Key: Key("subscription", "suppress-extra-usage-warning"), Type: TypeBool,
		Description: "Hide the warning that appears when a subscription runs into extra paid usage."})

	desktop := Place{CategoryNotifications, SectionNotifications, "Desktop"}
	add(desktop, Entry{Key: Key("agents", "banners", "enabled"), Type: TypeBool,
		Description: "Show an OS banner when an interactive agent session needs you."})
	add(desktop, Entry{Key: Key("agents", "sounds", "enabled"), Type: TypeBool,
		Description: "Play a sound when an interactive agent session finishes or needs you."})
	add(desktop, Entry{Key: Key("agents", "sounds", "done"), Type: TypeString,
		Description: "The sound file played when a session finishes, relative to the config directory. Empty uses the built-in sound."})
	add(desktop, Entry{Key: Key("agents", "sounds", "attention"), Type: TypeString,
		Description: "The sound file played when a session needs you, relative to the config directory. Empty uses the built-in sound."})
	telegram := Place{CategoryNotifications, SectionNotifications, "Telegram"}
	add(telegram, Entry{Key: Key("notifications", "telegram", "bot-token"), Type: TypeSecret,
		Description: "The token of the Telegram bot that sends notifications."})
	add(telegram, Entry{Key: Key("notifications", "telegram", "chat-id"), Type: TypeString,
		Description: "The Telegram chat that receives notifications."})
	add(Place{CategoryNotifications, SectionNotifications, "Slack"}, Entry{Key: Key("notifications", "slack", "webhook-url"), Type: TypeSecret,
		Description: "The Slack webhook address that receives notifications."})

	network := web(SectionAccess, "Network")
	add(network, risky(Entry{Key: Key("server", "web", "listen"), Type: TypeString, Restart: true,
		Description: "The address the web page is served on."},
		"Changing the address can expose the web page to other machines or lock you out of it."))
	add(network, risky(Entry{Key: Key("server", "web", "allowed-hosts"), Type: TypeStringList,
		Description: "Host names the web page accepts requests for."},
		"A wrong list can lock you out of the web page, and a broad one lets other sites reach it."))
	add(network, risky(Entry{Key: Key("server", "web", "files", "roots"), Type: TypeStringList,
		Description: "Extra folders the web page may host files from, besides repos, worktrees, the tickets store and temp folders."},
		"Every file under a listed folder becomes readable by anyone who signs in to the web page."))
	add(web(SectionAccess, "Sign-in"), Entry{Key: Key("server", "web", "totp"), Type: TypeBool, Risky: true, RiskyOffOnly: true,
		ConfirmText: "Turning off the one-time code lets anyone who can reach the web page sign in.",
		Description: "Ask for a one-time code when signing in to the web page."})
	leader := web(SectionKeybindings, "Leader key")
	add(leader, Entry{Key: Key("server", "web", "leader-key-mac"), Type: TypeString, CheckString: checkLeaderKey,
		Description: "The Leader key on a Mac, such as cmd+e. Press it to see the chords. Empty turns chords off."})
	add(leader, Entry{Key: Key("server", "web", "leader-key"), Type: TypeString, CheckString: checkLeaderKey,
		Description: "The Leader key on other platforms, such as ctrl+e. Press it to see the chords. Empty turns chords off."})
	add(web(SectionKeybindings, "Editors"), Entry{Key: Key("server", "web", "vi-escape"), Type: TypeString,
		Description: "A key sequence, such as \"jk\", that leaves insert mode in the web page's vi editors. Empty turns it off."})
	add(web(SectionDisplayDev, "Display"), Entry{Key: Key("server", "recently-finished"), Type: TypeNumber, Min: minOf(0), Integer: true,
		Description: "How many recently finished tickets stay listed. 0 hides them."})
	dev := web(SectionDisplayDev, "Development")
	add(dev, Entry{Key: Key("server", "web", "dev-frontend"), Type: TypeBool,
		Description: "Turn on Dev mode: gx runs its own Vite from the Dev checkout and serves the web UI from it."})
	add(dev, Entry{Key: Key("server", "web", "dev-dir"), Type: TypeString,
		Description: "The Dev checkout path, overriding the one stamped into the binary. A leading ~ is expanded; otherwise it must be absolute."})

	runtime := system(SectionServer, "Runtime")
	add(runtime, Entry{Key: Key("server", "watch-config"), Type: TypeBool,
		Description: "Reload the config automatically when config.json changes."})
	add(runtime, Entry{Key: Key("server", "tab-env"), Type: TypeStringList,
		Description: "Environment variable names copied into every new terminal tab."})
	add(system(SectionServer, "Logs"), dur(Key("server", "log-retention"), "How long server logs are kept before they are deleted."))

	disk := system(SectionDiskSpace, "Thresholds")
	add(disk, Entry{Key: Key("disk-space", "warn-below"), Type: TypeSize,
		Description: "Warn when free disk space drops below this size. 0 turns it off."})
	add(disk, Entry{Key: Key("disk-space", "clean-below"), Type: TypeSize,
		Description: "Run the cleanup command when free disk space drops below this size. 0 turns it off."})
	add(system(SectionDiskSpace, "Cleanup"), Entry{Key: Key("disk-space", "cleanup-command"), Type: TypeStringList, ProjectOverridable: true,
		Description: "The command, with its arguments, that frees disk space."})

	cpu := system(SectionWatchdogs, "CPU")
	add(cpu, Entry{Key: Key("cpu-watchdog", "enabled"), Type: TypeBool,
		Description: "Watch the server's CPU use and capture a profile when it stays high."})
	add(cpu, Entry{Key: Key("cpu-watchdog", "threshold-percent"), Type: TypeNumber, Min: minOf(1),
		Description: "CPU use, in percent of one core, that counts as high."})
	add(cpu, dur(Key("cpu-watchdog", "sustain"), "How long CPU use must stay high before the watchdog acts."))
	add(cpu, dur(Key("cpu-watchdog", "cooldown"), "The least time between two CPU captures."))
	proc := system(SectionWatchdogs, "Process")
	add(proc, Entry{Key: Key("watchdog", "max-tree-processes"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "The most processes one agent's tree may hold before the watchdog stops the agent."})
	add(proc, dur(Key("watchdog", "interval"), "How often the watchdog counts each agent's processes. Default: 1s."))
	add(proc, Entry{Key: Key("watchdog", "ulimit-headroom"), Type: TypeNumber, Min: minOf(1), Integer: true,
		Description: "How many processes above your current count an agent may start. The cap is fixed when an agent launches, so restart the agent if legitimate forks start failing."})
	return out
}
