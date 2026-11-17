package agentrunner_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// herdrImporters are the packages still allowed to import the herdr package.
// herdr is only a terminal multiplexer now: the worktrees tab opens a herdr
// workspace, and the server preflight and TUI gate ping it. Shrink this list,
// never grow it.
var herdrImporters = []string{
	"github.com/elentok/gx/cmd/serverctl",
	"github.com/elentok/gx/ui/worktrees/launch",
}

func TestOnlyAllowedPackagesImportHerdr(t *testing.T) {
	for _, pkg := range importersOf(t, "github.com/elentok/gx/herdr") {
		if !slices.Contains(herdrImporters, pkg) {
			t.Errorf("%s imports the herdr package; herdr is only a terminal multiplexer, so only the packages in herdrImporters may use it", pkg)
		}
	}
}

// vtImporters may import x/vt. testutil/vtterm is a test-only rig that needs
// the raw emulator for cursor and cell colours, so it does not go through
// agentrunner/screen.
var vtImporters = []string{
	"github.com/elentok/gx/agentrunner/screen",
	"github.com/elentok/gx/testutil/vtterm",
}

func TestOnlyScreenImportsVT(t *testing.T) {
	for _, pkg := range importersOf(t, "github.com/charmbracelet/x/vt") {
		if !slices.Contains(vtImporters, pkg) {
			t.Errorf("%s imports x/vt; go through agentrunner/screen", pkg)
		}
	}
}

func importersOf(t *testing.T, target string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}: {{join .Imports \" \"}}", "github.com/elentok/gx/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, ": ")
		if slices.Contains(strings.Fields(imports), target) {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}
