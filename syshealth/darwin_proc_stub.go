//go:build !darwin || !cgo

package syshealth

// DarwinProcessReader reports unavailable: this build has no libproc (not
// darwin, or darwin without cgo).
type DarwinProcessReader struct{}

// NewDarwinProcessReader returns the unavailable reader.
func NewDarwinProcessReader() *DarwinProcessReader { return &DarwinProcessReader{} }

func (*DarwinProcessReader) Read() (ProcessTable, error) { return ProcessTable{}, ErrUnavailable }

func (*DarwinProcessReader) Argv(int) []string { return nil }

func darwinProcessStart(int) (int64, error) { return 0, ErrUnavailable }
