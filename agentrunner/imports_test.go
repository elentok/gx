package agentrunner_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// herdrImporters are the packages still allowed to import the herdr package:
// the adapter, its test tooling, the herdr-only TUI features, and the wiring
// and herdr-only cleanup that haven't moved behind Runner yet. Shrink this
// list, never grow it: a new caller goes through agentrunner.Runner.
var herdrImporters = []string{
	"github.com/elentok/gx/agentrunner/herdrrunner",
	"github.com/elentok/gx/cmd",
	"github.com/elentok/gx/ralphloop",
	"github.com/elentok/gx/server",
	"github.com/elentok/gx/testutil/herdrctl",
	"github.com/elentok/gx/ui/tickets",
	"github.com/elentok/gx/ui/worktrees",
}

func TestOnlyAllowedPackagesImportHerdr(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}}: {{join .Imports \" \"}}", "github.com/elentok/gx/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, ": ")
		if slices.Contains(strings.Fields(imports), "github.com/elentok/gx/herdr") && !slices.Contains(herdrImporters, pkg) {
			t.Errorf("%s imports the herdr package; go through agentrunner.Runner", pkg)
		}
	}
}
