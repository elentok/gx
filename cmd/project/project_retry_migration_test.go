package project

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/projects"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
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

	// The start-up migration may still be running under load; the server
	// refuses with ReasonRetryRunning until it ends, so retry until it accepts.
	var out string
	var err error
	testutil.WaitFor(t, 30*time.Second, "the start-up migration to end", func() bool {
		out, err = run(projects.ScratchProject)
		return err == nil || !strings.Contains(err.Error(), wire.ReasonRetryRunning)
	})
	if err != nil || !strings.Contains(out, "migration of project "+projects.ScratchProject+" started") {
		t.Errorf("started = %q, %v", out, err)
	}
	if _, err = run("nope"); err == nil || !strings.Contains(err.Error(), wire.ReasonRetryUnknownProject) || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("unknown err = %v", err)
	}
}
