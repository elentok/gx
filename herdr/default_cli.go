package herdr

// The package-level functions run herdr under the process environment: they
// call the zero CLI, so callers that don't carry an environment are unchanged.

func FindWorkspace(label string) (string, error) { return CLI{}.FindWorkspace(label) }
func FindOrCreateWorkspace(label, cwd string) (string, error) {
	return CLI{}.FindOrCreateWorkspace(label, cwd)
}
func Ping() error { return CLI{}.Ping() }
