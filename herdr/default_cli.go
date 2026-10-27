package herdr

// The package-level functions run herdr under the process environment: they
// call the zero CLI, so callers that don't carry an environment are unchanged.

func AgentPrompt(opts AgentPromptOptions) (Agent, error) { return CLI{}.AgentPrompt(opts) }
func AgentStart(opts AgentStartOptions) (Agent, error)   { return CLI{}.AgentStart(opts) }
func AgentGet(target string) (Agent, error)              { return CLI{}.AgentGet(target) }
func AgentWait(opts AgentWaitOptions) (Agent, error)     { return CLI{}.AgentWait(opts) }
func AgentList() ([]Agent, error)                        { return CLI{}.AgentList() }
func AgentExplain(target string) (AgentExplainResult, error) {
	return CLI{}.AgentExplain(target)
}
func AgentSendKeys(target string, keys ...string) error {
	return CLI{}.AgentSendKeys(target, keys...)
}
func AgentRead(target string, opts AgentReadOptions) (string, error) {
	return CLI{}.AgentRead(target, opts)
}

func TabCreate(opts TabCreateOptions) (CreatedTab, error) { return CLI{}.TabCreate(opts) }
func TabList(workspaceID string) ([]Tab, error)           { return CLI{}.TabList(workspaceID) }
func TabFocus(tabID string) (Tab, error)                  { return CLI{}.TabFocus(tabID) }
func TabClose(tabID string) error                         { return CLI{}.TabClose(tabID) }

func FindWorkspace(label string) (string, error) { return CLI{}.FindWorkspace(label) }
func FindOrCreateWorkspace(label, cwd string) (string, error) {
	return CLI{}.FindOrCreateWorkspace(label, cwd)
}
func EnsureWorkspace(label, cwd string) (string, error) {
	return CLI{}.EnsureWorkspace(label, cwd)
}
func Ping() error { return CLI{}.Ping() }

func WorktreeCreate(opts WorktreeCreateOptions) (Worktree, error) {
	return CLI{}.WorktreeCreate(opts)
}
func WorktreeOpen(opts WorktreeOpenOptions) (Worktree, error) { return CLI{}.WorktreeOpen(opts) }
func WorktreeRemove(workspaceID string, force bool) error {
	return CLI{}.WorktreeRemove(workspaceID, force)
}
