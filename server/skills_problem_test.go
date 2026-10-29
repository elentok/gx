package server_test

import (
	"context"
	"testing"

	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/skills"
)

func TestSnapshot_CarriesTheRecordedSkillsInstallProblem(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := context.Background()

	snap, err := h.Client.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.SkillsInstallProblem != "" {
		t.Fatalf("problem with no record = %q, want none", snap.SkillsInstallProblem)
	}

	// `gx server start` may record it while the server already runs.
	if err := skills.WriteProblem(skills.StatusPath(h.StateDir), "install refused"); err != nil {
		t.Fatal(err)
	}
	if snap, err = h.Client.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if snap.SkillsInstallProblem != "install refused" {
		t.Errorf("problem = %q, want %q", snap.SkillsInstallProblem, "install refused")
	}
}
