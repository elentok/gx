package testutil

import (
	"fmt"
	"os"
	"strconv"
)

// init turns off git's auto maintenance for every git the test process runs,
// including repos that product code creates itself (such as the ticket
// store). A commit otherwise starts a detached `git maintenance run --auto`
// that can outlive the test: its lock file under .git/objects then makes
// t.TempDir's cleanup fail with "directory not empty". CI sets the same
// variables for packages that don't import testutil.
func init() {
	appendGitConfig("maintenance.auto", "false")
}

// appendGitConfig adds key=value to the GIT_CONFIG_COUNT/KEY/VALUE
// environment config, after any entries already set.
func appendGitConfig(key, value string) {
	n, _ := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	for i := range n {
		if os.Getenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", i)) == key {
			return
		}
	}
	os.Setenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", n), key)
	os.Setenv(fmt.Sprintf("GIT_CONFIG_VALUE_%d", n), value)
	os.Setenv("GIT_CONFIG_COUNT", strconv.Itoa(n+1))
}
