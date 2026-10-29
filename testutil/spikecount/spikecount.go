// Package spikecount counts process starts for the server-tests-pty spike.
// Each re-run of the test binary appends one line naming its kind to the file
// in $GX_SPIKE_COUNT_FILE. One O_APPEND write per process, no lock needed.
package spikecount

import "os"

// Env names the file the counts go to; unset means no counting.
const Env = "GX_SPIKE_COUNT_FILE"

// Record appends kind as one line.
func Record(kind string) {
	path := os.Getenv(Env)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(kind + "\n")
	_ = f.Close()
}
