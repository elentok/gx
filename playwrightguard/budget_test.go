package playwrightguard

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// maxFlows caps the number of Playwright flows. Every flow starts its own
// fixture server and Chromium, so the suite's run time grows with this number.
// Lower it whenever flows are removed (docs/plans/slim-e2e-suite.md aims for
// about 100). Raise it only with a reason in the commit, after trying vitest or
// a Go test (skills/gx-playwright-e2es, "Write an e2e only when...").
const maxFlows = 115

var flowStart = regexp.MustCompile(`(?m)^\s*test\(`)

func TestFlowBudget(t *testing.T) {
	files, err := filepath.Glob("../webui/e2e/*.e2e.ts")
	if err != nil || len(files) == 0 {
		t.Fatalf("no flow files found: %v", err)
	}
	total := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		total += len(flowStart.FindAll(src, -1))
	}
	if total > maxFlows {
		t.Fatalf("%d e2e flows, budget is %d. Cover the new check with a vitest or Go test, or add it as a step in an existing flow. See skills/gx-playwright-e2es.", total, maxFlows)
	}
}
