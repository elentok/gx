//go:build gxfixture

package serverctl

// dropAskedAgent lets gxfixture force the cleanup a herdr-hosted run does when
// its agent asks, which a fixture's pty runner never takes. Only the gxfixture
// build reads the variable, so a release binary cannot be switched into it.
func dropAskedAgent(getenv func(string) string) bool {
	return getenv("GX_DROP_ASKED_AGENT") == "1"
}
