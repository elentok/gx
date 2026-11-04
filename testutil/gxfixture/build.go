package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/elentok/gx/testutil"
)

// removeAll retries while a stopped process finishes writing into dir. Go
// marks its module cache read-only, so the dirs are made writable first.
func removeAll(dir string) {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		testutil.MakeWritable(dir)
		if os.RemoveAll(dir) == nil {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return
			}
		}
	}
}

// binary returns given when set, and otherwise builds pkg into out.
func binary(given, out, pkg string) (string, error) {
	if given != "" {
		return filepath.Abs(given)
	}
	// The gxfixture tag adds the fake non-Claude harness to the gx binary.
	if b, err := exec.Command("go", "build", "-tags", "gxfixture", "-o", out, pkg).CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", pkg, err, b)
	}
	return out, nil
}
