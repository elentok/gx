package git

import (
	"fmt"
	"strconv"
	"strings"
)

// WithConfigEnv returns env with key=value added to its GIT_CONFIG_COUNT /
// GIT_CONFIG_KEY_n / GIT_CONFIG_VALUE_n entries, after any already there. A key
// that is already set is left alone. env itself is not changed.
func WithConfigEnv(env []string, key, value string) []string {
	get := func(name string) string {
		var v string
		for _, kv := range env {
			if k, val, ok := strings.Cut(kv, "="); ok && k == name {
				v = val // the last entry wins, as in exec
			}
		}
		return v
	}
	n, _ := strconv.Atoi(get("GIT_CONFIG_COUNT"))
	for i := range n {
		if get(fmt.Sprintf("GIT_CONFIG_KEY_%d", i)) == key {
			return env
		}
	}
	out := append([]string{}, env...)
	return append(out,
		fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", n, key),
		fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n, value),
		"GIT_CONFIG_COUNT="+strconv.Itoa(n+1),
	)
}

// WithMaintenanceOff returns env with git's auto maintenance turned off. A
// commit otherwise starts a detached `git maintenance run --auto` that can
// outlive a test: its lock file under .git/objects makes t.TempDir's cleanup
// fail with "directory not empty".
func WithMaintenanceOff(env []string) []string {
	return WithConfigEnv(env, "maintenance.auto", "false")
}
