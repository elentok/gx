package rigs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/ghfake"
	"github.com/elentok/gx/tickets"
)

// PRFlowGrace is the no-checks grace window StartPRFlow writes to config.json.
const PRFlowGrace = "3s"

// PRFlowOpts tweaks StartPRFlow.
type PRFlowOpts struct {
	// Trunk is the project's trunk; default "main".
	Trunk string
	// Server options run last, on the config the rig has already set up.
	Server []func(*server.Config)
}

// PRFlow is a running server over a `pr`-delivery project (proj, epic-a/01)
// whose gh is the fake and whose origin is a local bare repo.
type PRFlow struct {
	H      *servertest.Harness
	Origin *ghfake.Origin
	// GH edits the fake's state and reads its call log.
	GH    *ghfake.Fake
	Store string
	// Env is the server's Config.Env, with the fake gh first on its PATH.
	Env []string
	// ConfigPath is the config.json the server was started with.
	ConfigPath string
}

// EpicDir is the directory of the rig's epic.
func (f *PRFlow) EpicDir() string { return filepath.Join(f.Store, "proj", "epic-a") }

// StampPR gives the rig's epic a PR identity, as opening the PR would, and has
// the server re-read the store.
func (f *PRFlow) StampPR(t *testing.T, number int, url string) {
	t.Helper()
	if err := tickets.StampEpicFields(f.EpicDir(), map[string]any{"pr_number": number, "pr_url": url}); err != nil {
		t.Fatal(err)
	}
	f.Rescan(t)
}

// Rescan has the server re-read the ticket store now.
func (f *PRFlow) Rescan(t *testing.T) {
	t.Helper()
	if err := f.H.Server.PR.RescanIndex(); err != nil {
		t.Fatal(err)
	}
}

// Tick forces one pass of the PR watcher.
func (f *PRFlow) Tick(t *testing.T) {
	t.Helper()
	f.H.Server.PR.Watcher().Tick(context.Background())
}

// StartPRFlow starts the server paused-by-default like any harness; tests drive
// it through the client. The project repo is Origin.Local.
func StartPRFlow(t *testing.T, opts PRFlowOpts) *PRFlow {
	t.Helper()
	trunk := opts.Trunk
	if trunk == "" {
		trunk = "main"
	}
	origin := ghfake.NewOrigin(t, trunk)
	store := t.TempDir()
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	if err := servertest.WriteProjectRepo(store, "proj", origin.Local, `,"delivery":"pr"`); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	servertest.WriteConfig(t, configPath, store, map[string]any{"pr": map[string]any{"no-checks-grace": PRFlowGrace}})
	var env []string
	setup := func(c *server.Config) {
		c.ConfigPath = configPath
		// Test servers take their settings from Config, not from config.json.
		c.NoChecksGrace, _ = time.ParseDuration(PRFlowGrace)
		// The base PATH comes from the server env: reading the process's is banned.
		c.Env = origin.GH.Env(c.Env, harness.LookupEnv(c.Env, "PATH"))
	}
	record := func(c *server.Config) { env = c.Env }
	h := servertest.StartWithStore(t, store, append(append([]func(*server.Config){setup}, opts.Server...), record)...)
	return &PRFlow{H: h, Origin: origin, GH: origin.GH, Store: store, ConfigPath: configPath, Env: env}
}
