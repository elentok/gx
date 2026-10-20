package skills

import (
	"os"
	"strings"
	"testing"
)

func TestWebUISecurityDocsKeepWarnings(t *testing.T) {
	b, err := os.ReadFile("../docs/web-ui-security.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, phrase := range []string{
		"plain HTTP",
		"sniffed cookie",
		"permission approvals",
		"full-permission agent",
		"server.web.allowed-hosts",
		"before you upgrade",
		"IP addresses and `localhost` are unaffected",
		"gx tickets migrate",
		"once after you upgrade",
		"shows no open question",
		"cannot answer it",
		"gx tickets ask",
		"gx tickets answer",
		"gx tickets log",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("docs/web-ui-security.md is missing %q", phrase)
		}
	}
}
