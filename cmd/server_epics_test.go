package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func TestPrintEpicDetail_ShowsQueueable(t *testing.T) {
	var out bytes.Buffer
	d := server.EpicDetail{Address: "proj:epic", Kind: server.EpicKindLoop, Done: 1, Total: 2, Queueable: true}
	if err := printEpicDetail(&out, d); err != nil {
		t.Fatal(err)
	}
	if want := "proj:epic\tloop\t\t1/2\tqueueable=true\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_OneLinePerEpic(t *testing.T) {
	var out bytes.Buffer
	l := server.EpicList{Epics: []server.EpicListRow{
		{Address: "proj:a", Kind: server.EpicKindLoop, State: server.EpicListReady, Done: 1, Total: 2, Queueable: true, Section: server.SectionReadyToQueue},
		{Address: "proj:b", Kind: server.EpicKindMap, State: server.EpicListDesigning},
	}}
	if err := printEpicList(&out, l); err != nil {
		t.Fatal(err)
	}
	if want := "proj:a\tloop\tready\t1/2\tqueueable=true\tready-to-queue\nproj:b\tmap\tdesigning\t0/0\tqueueable=false\t\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_NeedsYouRowsKeepTheServersOldestWaitOrder(t *testing.T) {
	var out bytes.Buffer
	l := server.EpicList{Epics: []server.EpicListRow{
		{Address: "proj:old", Kind: server.EpicKindLoop, State: server.EpicListPaused, Section: server.SectionNeedsYou, WaitingSince: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Address: "proj:new", Kind: server.EpicKindLoop, State: server.EpicListMergeBlocked, Section: server.SectionNeedsYou, WaitingSince: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}}
	if err := printEpicList(&out, l); err != nil {
		t.Fatal(err)
	}
	want := "proj:old\tloop\tpaused\t0/0\tqueueable=false\tneeds-you\nproj:new\tloop\tmerge-blocked\t0/0\tqueueable=false\tneeds-you\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestServerEpicsList_HasJSONFlag(t *testing.T) {
	root := newRootCmd(deps{})
	c, _, err := root.Find([]string{"server", "epics"})
	if err != nil || c.Flags().Lookup("json") == nil || c.RunE == nil {
		t.Errorf("server epics = %v, err %v; want a runnable verb with --json", c, err)
	}
}

func TestServerEpicsMerge_RejectsNonEpicAddresses(t *testing.T) {
	for _, args := range [][]string{{}, {"proj:epic/01"}, {"epic"}, {"proj:a", "proj:b"}} {
		root := newRootCmd(deps{})
		root.SetArgs(append([]string{"server", "epics", "merge"}, args...))
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		if err := root.Execute(); err == nil {
			t.Errorf("args %q: want an error", args)
		}
	}
}

func TestServerEpicsMerge_PrintsEachOutcome(t *testing.T) {
	cases := map[string]struct {
		res     server.QueueResult
		out     string
		errPart string
	}{
		"merged": {res: server.QueueResult{Merged: true}, out: "merged\n"},
		"forked": {res: server.QueueResult{MergeTicket: "proj:epic/07"}, out: "proj:epic/07\n"},
		"paused": {res: server.QueueResult{Paused: true, PauseKind: "conflict", Message: "merge conflict in a.go"},
			errPart: "paused (conflict): merge conflict in a.go"},
		"paused needs-rebase": {res: server.QueueResult{Paused: true, Message: "branch is behind trunk"},
			errPart: "paused (needs-rebase): branch is behind trunk"},
	}
	for _, code := range []string{
		server.ReasonInvalidAddress, server.ReasonUnknownTicket, server.ReasonMapEpic, server.ReasonAlreadyMerged,
		server.ReasonMergePending, server.ReasonNotComplete, server.ReasonBranchMissing,
	} {
		cases[code] = struct {
			res     server.QueueResult
			out     string
			errPart string
		}{res: server.QueueResult{Refused: true, Reason: code, Message: "why " + code}, errPart: "refused (" + code + "): why " + code}
	}
	for name, tc := range cases {
		var out bytes.Buffer
		err := runServerQueueWrite(context.Background(), nil, &out, false,
			func(context.Context, *apiclient.Client) (server.QueueResult, error) { return tc.res, nil })
		if tc.errPart == "" && (err != nil || out.String() != tc.out) {
			t.Errorf("%s: out = %q, err = %v; want %q", name, out.String(), err, tc.out)
		}
		if tc.errPart != "" && (err == nil || err.Error() != tc.errPart) {
			t.Errorf("%s: err = %v; want %q", name, err, tc.errPart)
		}
	}
}

func TestServerEpicsMerge_RefusesWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	err := runServerQueueWrite(context.Background(), cl, &bytes.Buffer{}, false,
		func(ctx context.Context, cl *apiclient.Client) (server.QueueResult, error) {
			return cl.EpicMerge(ctx, "proj:epic")
		})
	if err == nil || !strings.Contains(err.Error(), server.ReasonServerNotRunning) {
		t.Errorf("err = %v; want %s", err, server.ReasonServerNotRunning)
	}
}
