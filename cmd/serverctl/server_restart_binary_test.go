package serverctl

import (
	"os"
	"testing"
)

func TestInstalledBinary_ReadsTheProgramFromThePlist(t *testing.T) {
	t.Parallel()
	a, _ := newTestAgent(t)
	a.exe = "/opt/gx & co/gx"
	if _, ok := a.installedBinary(); ok {
		t.Fatal("installedBinary ok before install")
	}
	if err := os.MkdirAll(a.agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := a.plist()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.plistPath(), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok := a.installedBinary()
	if !ok || got != "/opt/gx & co/gx" {
		t.Errorf("installedBinary = %q, %v", got, ok)
	}
}
