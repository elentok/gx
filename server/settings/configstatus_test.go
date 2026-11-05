package settings_test

import (
	"context"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/servertest"
)

func TestConfigStatus_FreshServerIsEmpty(t *testing.T) {
	t.Parallel()
	h := servertest.Start(t)
	st, err := h.Client.Admin.ConfigStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Empty() {
		t.Errorf("status = %+v, want empty", st)
	}
}

func TestConfigStatus_KeepsLastReloadAndPendingKeysUntilRestart(t *testing.T) {
	t.Parallel()
	store := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.json")
	h := servertest.StartWithStore(t, store, servertest.WithConfigFile(path))
	ctx := context.Background()

	servertest.WriteConfig(t, path, store, map[string]any{"server": map[string]any{"tcp-listen": true}})
	before := time.Now()
	reload(t, h)
	st, err := h.Client.Admin.ConfigStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := st.LastReload
	if last == nil || last.Outcome != wire.EventLevelWarning || last.Trigger != "command" ||
		!strings.Contains(last.Message, "restart needed for server.tcp-listen") || last.At.Before(before) {
		t.Errorf("last reload = %+v, want a command warning about server.tcp-listen", last)
	}
	if !slices.Equal(st.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("pending = %v, want [server.tcp-listen]", st.PendingRestart)
	}

	servertest.WriteConfig(t, path, store, nil)
	reload(t, h)
	st, err = h.Client.Admin.ConfigStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastReload == nil || st.LastReload.Outcome != wire.EventLevelSuccess {
		t.Errorf("last reload = %+v, want success", st.LastReload)
	}
	if !slices.Equal(st.PendingRestart, []string{"server.tcp-listen"}) {
		t.Errorf("pending after revert = %v, want it kept until restart", st.PendingRestart)
	}
}
