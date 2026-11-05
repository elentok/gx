package cmdbase

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/elentok/gx/server/wire"
)

// WriteConfigBlock renders the Config block that both `gx server
// config-status` and `gx server status` print.
func WriteConfigBlock(w io.Writer, st wire.ConfigStatus, now time.Time) error {
	lines := []string{"config:"}
	if r := st.LastReload; r != nil {
		lines = append(lines, fmt.Sprintf("last reload: %s (%s, %s ago): %s", r.Outcome, r.Trigger, now.Sub(r.At).Round(time.Second), r.Message))
	}
	if len(st.PendingRestart) > 0 {
		lines = append(lines, "restart needed for: "+strings.Join(st.PendingRestart, ", "))
	}
	if !st.HardStopAt.IsZero() {
		lines = append(lines, fmt.Sprintf("hard limit: agents stop in %s unless it is raised", max(st.HardStopAt.Sub(now), 0).Round(time.Second)))
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n  "))
	return err
}

// SetupCodeLine is the Setup code and the time it stops working, for text
// output; `gx server web-*` and `gx server status` both print it.
func SetupCodeLine(code string, expires time.Time) string {
	return "setup code: " + code + " (expires " + expires.Local().Format("15:04") + ")"
}
