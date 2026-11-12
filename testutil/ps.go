package testutil

import (
	"os/exec"
	"strings"
	"testing"
)

// PsLines is the command line of every live process on the host. env is the
// environment ps runs in; a fixture passes its own so its fake ps is the one used.
func PsLines(t *testing.T, env []string) []string {
	t.Helper()
	cmd := exec.Command("ps", "-axww", "-o", "command=")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}
