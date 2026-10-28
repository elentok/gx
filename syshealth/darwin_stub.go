//go:build !darwin || !cgo

package syshealth

// readDarwin reports unavailable: this build has no Mach host calls (not
// darwin, or darwin without cgo).
func readDarwin() (Reading, error) { return Reading{}, ErrUnavailable }
