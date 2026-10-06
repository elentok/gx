package cmd

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestServerStart_AlreadyRunningReportsPidAndDoesNotSpawn(t *testing.T) {
	h := servertest.Start(t)
	var out bytes.Buffer
	spawn := func() error { t.Error("spawned a second server"); return nil }

	if err := runServerStart(context.Background(), h.Client, &out, spawn); err != nil {
		t.Fatal(err)
	}
	if want := "already running (pid " + strconv.Itoa(os.Getpid()) + ")\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
