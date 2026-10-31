package harness

// TOMLString exposes the escaper to the harness_test package.
var TOMLString = tomlString

// NewCodexWatcher builds a Codex watcher without the env and screen Watch
// reads, so tests can run in parallel.
func NewCodexWatcher(sessionFile string, startup func() bool) TitleWatcher {
	return newCodexWatcher(sessionFile, startup)
}

// NewCodexWatcherIn is NewCodexWatcher reading rollouts under codexHome.
func NewCodexWatcherIn(sessionFile, codexHome string, startup func() bool) TitleWatcher {
	w := newCodexWatcher(sessionFile, startup)
	w.home = codexHome
	return w
}
