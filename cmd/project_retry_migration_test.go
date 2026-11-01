package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
)

func TestProjectRetryMigration_StartsAndRefuses(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	ctx := context.Background()
	run := func(name string) (string, error) {
		var out bytes.Buffer
		err := runProjectRetryMigration(ctx, h.Client, &out, false, name)
		return out.String(), err
	}

	out, err := run(server.ScratchProject)
	if err != nil || !strings.Contains(out, "migration of project "+server.ScratchProject+" started") {
		t.Errorf("started = %q, %v", out, err)
	}
	if _, err = run("nope"); err == nil || !strings.Contains(err.Error(), server.ReasonRetryUnknownProject) || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("unknown err = %v", err)
	}
}
