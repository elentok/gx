package nativerunner_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/syshealth"
)

func writeOwnerMeta(t *testing.T, root, label string, m nativerunner.Meta) {
	t.Helper()
	dir := filepath.Join(root, label)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, nativerunner.MetaFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func unixOf(t *testing.T, lstart string) int64 {
	t.Helper()
	ts, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", lstart, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return ts.Unix()
}

func TestOwnerPIDs(t *testing.T) {
	const holderStart = "Mon Oct  6 09:59:58 2026"
	sess := agentrunner.Session{Label: "agent-1"}
	claude := agentrunner.OwnerPID{PID: agentPID, Start: unixOf(t, agentStart)}
	holder := agentrunner.OwnerPID{PID: 4141, Start: unixOf(t, holderStart)}
	good := nativerunner.Meta{PID: agentPID, PIDStart: agentStart, HolderPID: 4141, HolderPIDStart: holderStart}

	tests := []struct {
		name  string
		meta  *nativerunner.Meta
		owner func(root string) agentrunner.Runner
		want  []agentrunner.OwnerPID
	}{
		{"pty returns claude and holder", &good, func(r string) agentrunner.Runner { return &nativerunner.Pty{Root: r} }, []agentrunner.OwnerPID{claude, holder}},
		{"headless returns claude only", &good, func(r string) agentrunner.Runner { return &nativerunner.Headless{Root: r} }, []agentrunner.OwnerPID{claude}},
		{"missing meta gives nothing", nil, func(r string) agentrunner.Runner { return &nativerunner.Pty{Root: r} }, nil},
		{"unparsable pid_start leaves that pid out", &nativerunner.Meta{PID: agentPID, PIDStart: "garbage", HolderPID: 4141, HolderPIDStart: holderStart},
			func(r string) agentrunner.Runner { return &nativerunner.Pty{Root: r} }, []agentrunner.OwnerPID{holder}},
		{"missing holder start leaves holder out", &nativerunner.Meta{PID: agentPID, PIDStart: agentStart, HolderPID: 4141},
			func(r string) agentrunner.Runner { return &nativerunner.Pty{Root: r} }, []agentrunner.OwnerPID{claude}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.meta != nil {
				writeOwnerMeta(t, root, sess.Label, *tc.meta)
			}
			got, err := agentrunner.OwnerPIDs(tc.owner(root), sess)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestOwnerPIDs_StartMatchesSharedParser(t *testing.T) {
	root := t.TempDir()
	writeOwnerMeta(t, root, "a", nativerunner.Meta{PID: 1, PIDStart: agentStart})
	got, _ := (&nativerunner.Headless{Root: root}).OwnerPIDs(agentrunner.Session{Label: "a"})
	want, _ := syshealth.ParseLstart(agentStart)
	if len(got) != 1 || got[0].Start != want {
		t.Fatalf("got %v, want start %d", got, want)
	}
}
