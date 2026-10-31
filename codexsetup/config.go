package codexsetup

import (
	"fmt"
	"regexp"
	"strings"
)

const approveLine = `default_tools_approval_mode = "approve"`

var (
	enabledKey     = regexp.MustCompile(`^\s*enabled\s*=`)
	approvalKey    = regexp.MustCompile(`^\s*default_tools_approval_mode\s*=`)
	toolListKey    = regexp.MustCompile(`^\s*(enabled_tools|disabled_tools)\s*=`)
	enabledOK      = regexp.MustCompile(`^\s*enabled\s*=\s*true\s*(#.*)?$`)
	approvalOK     = regexp.MustCompile(`^\s*default_tools_approval_mode\s*=\s*"approve"\s*(#.*)?$`)
	defaultServers = []string{"gopls-mcp", "gopls"}
)

func serverHeaders(server string) (plain, quoted string) {
	return "[mcp_servers." + server + "]", `[mcp_servers."` + server + `"]`
}

func hasServer(lines []string, server string) bool {
	plain, quoted := serverHeaders(server)
	for _, l := range lines {
		if l == plain || l == quoted {
			return true
		}
	}
	return false
}

// resolveServer picks the gopls MCP table: the named one, else gopls-mcp,
// else gopls. It returns "" when none is configured.
func resolveServer(lines []string, named string) string {
	if named != "" {
		if hasServer(lines, named) {
			return named
		}
		return ""
	}
	for _, s := range defaultServers {
		if hasServer(lines, s) {
			return s
		}
	}
	return ""
}

// approveGopls sets enabled and default_tools_approval_mode in the server's
// table and drops its enabled_tools/disabled_tools arrays. Every other line,
// comments included, is kept as is.
func approveGopls(text, server string) string {
	plain, quoted := serverHeaders(server)
	lines := strings.Split(text, "\n")
	var out []string
	inTarget, skipping := false, false
	sawEnabled, sawApproval := false, false
	lastContent := 0 // len(out) after the table's last non-blank line

	finish := func() {
		var add []string
		if !sawEnabled {
			add = append(add, "enabled = true")
		}
		if !sawApproval {
			add = append(add, approveLine)
		}
		out = append(out[:lastContent], append(add, out[lastContent:]...)...)
	}

	for _, l := range lines {
		if strings.HasPrefix(l, "[") {
			if inTarget {
				finish()
			}
			inTarget = l == plain || l == quoted
			sawEnabled, sawApproval, skipping = false, false, false
			out = append(out, l)
			lastContent = len(out)
			continue
		}
		if inTarget {
			switch {
			case skipping:
				if strings.Contains(l, "]") {
					skipping = false
				}
				continue
			case enabledKey.MatchString(l):
				sawEnabled = true
				if !enabledOK.MatchString(l) {
					l = "enabled = true"
				}
			case approvalKey.MatchString(l):
				sawApproval = true
				if !approvalOK.MatchString(l) {
					l = approveLine
				}
			case toolListKey.MatchString(l):
				skipping = !strings.Contains(l, "]")
				continue
			}
		}
		out = append(out, l)
		if inTarget && strings.TrimSpace(l) != "" {
			lastContent = len(out)
		}
	}
	if inTarget {
		finish()
	}
	return strings.Join(out, "\n")
}

func missingServerMessage(named, path string) string {
	if named != "" {
		return fmt.Sprintf("MCP server %q is not configured in %s; skipped gopls approvals", named, path)
	}
	return fmt.Sprintf("no gopls-mcp or gopls MCP server is configured in %s; skipped gopls approvals", path)
}
