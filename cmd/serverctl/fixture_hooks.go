//go:build !gxfixture

package serverctl

import "time"

// fastSleep is always nil in a release build.
func fastSleep(func(string) string) func(time.Duration) { return nil }

// fastPollInterval is always zero (the default) in a release build.
func fastPollInterval(func(string) string) time.Duration { return 0 }

// wrapLateFinishScan installs no hold in a release build.
func wrapLateFinishScan(func(string) string) func(func()) func() { return nil }
