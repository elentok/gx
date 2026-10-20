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
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("docs/web-ui-security.md is missing %q", phrase)
		}
	}
}
