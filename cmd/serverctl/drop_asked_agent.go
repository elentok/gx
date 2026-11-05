//go:build !gxfixture

package serverctl

// dropAskedAgent is always false in a release build; see drop_asked_agent_fixture.go.
func dropAskedAgent(func(string) string) bool { return false }
