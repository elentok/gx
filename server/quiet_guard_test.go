package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Banner, sound and chat go quiet at the server's construction seam, so a
// notifier, player or chat transport built anywhere else would skip the
// Rest period's quiet predicate.
func TestQuietGuard_NotifierPlayerAndChatTransportAreBuiltOnlyAtTheServerSeam(t *testing.T) {
	t.Parallel()
	root := ".."
	// file -> why it may build one. The CLI's direct test-send helpers build a
	// transport inside the transport files; a person asked for those sends.
	allowed := map[string]string{
		"server/server.go":                       "the server's construction seam",
		"ralphloop/notify/server_chat.go":        "the server's chat seam; the sink there carries the quiet check",
		"ralphloop/notify/telegram_transport.go": "defines the transport and the CLI test-send helpers",
		"ralphloop/notify/slack_transport.go":    "defines the transport and the CLI test-send helpers",
		"osnotify/osnotify.go":                   "defines the constructor",
		"sound/sound.go":                         "defines the constructor",
		"server/quiet_guard_test.go":             "this guard",
	}
	build := regexp.MustCompile(`\b(osnotify\.NewSystemNotifier|sound\.NewSystemPlayer|newTelegramTransport|newSlackTransport)\(`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "webui", ".scratch":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if _, ok := allowed[filepath.ToSlash(rel)]; ok {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(src), "\n") {
			if trimmed := strings.TrimSpace(line); !strings.HasPrefix(trimmed, "//") && build.MatchString(line) {
				t.Errorf("%s builds a notifier, player or chat transport outside the server's construction seam: %s", rel, trimmed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
