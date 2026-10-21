package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

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
		"parked": {res: server.QueueResult{Parked: true, ParkKind: "conflict", Message: "merge conflict in a.go"},
			errPart: "parked (conflict): merge conflict in a.go"},
		"parked needs-rebase": {res: server.QueueResult{Parked: true, Message: "branch is behind trunk"},
			errPart: "parked (needs-rebase): branch is behind trunk"},
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
