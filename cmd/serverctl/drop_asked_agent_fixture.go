//go:build gxfixture

package serverctl

// dropAskedAgent lets gxfixture force the cleanup a herdr-hosted run does when
// its agent asks, which a fixture's pty runner never takes. Only the gxfixture
// build reads the variable, so a release binary cannot be switched into it.
func dropAskedAgent(getenv func(string) string) bool {
	return getenv("GX_DROP_ASKED_AGENT") == "1"
}

// lateFinishScanHold is a file that, while it exists, stops the server's
// late-finish rescan, so a flow can send its answer before the scan adopts.
func lateFinishScanHold(getenv func(string) string) string {
	return getenv("GX_HOLD_LATE_FINISH_SCAN")
}
