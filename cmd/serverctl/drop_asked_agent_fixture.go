//go:build gxfixture

package serverctl

import "os"

// dropAskedAgent lets gxfixture force the cleanup a herdr-hosted run does when
// its agent asks, which a fixture's pty runner never takes. Only the gxfixture
// build reads the variable, so a release binary cannot be switched into it.
func dropAskedAgent(getenv func(string) string) bool {
	return getenv("GX_DROP_ASKED_AGENT") == "1"
}

// wrapLateFinishScan holds the late-finish rescan while the file named by
// GX_HOLD_LATE_FINISH_SCAN exists, so a flow can send its answer before the
// scan adopts. It returns nil (no hold) when the variable is unset.
func wrapLateFinishScan(getenv func(string) string) func(func()) func() {
	hold := getenv("GX_HOLD_LATE_FINISH_SCAN")
	if hold == "" {
		return nil
	}
	return func(scan func()) func() {
		return func() {
			if _, err := os.Stat(hold); err == nil {
				return
			}
			scan()
		}
	}
}
