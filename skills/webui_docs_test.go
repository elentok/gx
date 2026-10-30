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
		"server.web.allowed-hosts",
		"server.web.totp",
		"before the first start with this version",
		"trust the root certificate",
		"sign in again",
		"gx server web-reset-authenticator",
		"IP addresses and `localhost` need no entry",
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
