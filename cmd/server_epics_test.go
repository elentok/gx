package cmd

import (
	"bytes"
	"context"
	"github.com/elentok/gx/cmd/cmdbase"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/apiclient"
)

func TestPrintEpicDetail_ShowsQueueable(t *testing.T) {
	var out bytes.Buffer
	d := wire.EpicDetail{Address: "proj:epic", Kind: wire.EpicKindLoop, Done: 1, Total: 2, Queueable: true}
	if err := printEpicDetail(&out, d); err != nil {
		t.Fatal(err)
	}
	if want := "proj:epic\tloop\t\t1/2\tqueueable=true\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_OneLinePerEpic(t *testing.T) {
	var out bytes.Buffer
	l := wire.EpicList{Epics: []wire.EpicListRow{
		{Address: "proj:a", Kind: wire.EpicKindLoop, State: wire.EpicListReady, Done: 1, Total: 2, Queueable: true, Section: wire.SectionReadyToQueue},
		{Address: "proj:b", Kind: wire.EpicKindMap, State: wire.EpicListDesigning},
	}}
	if err := printEpicList(&out, l, false); err != nil {
		t.Fatal(err)
	}
	if want := "proj:a\tloop\tready\t1/2\tqueueable=true\tready-to-queue\nproj:b\tmap\tdesigning\t0/0\tqueueable=false\t\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_JoinedCardPrintsTheActivePartsAddress(t *testing.T) {
	var out bytes.Buffer
	l := wire.EpicList{Epics: []wire.EpicListRow{
		{Address: "proj:m", Addresses: []string{"proj:m-impl", "proj:m"}, Kind: wire.EpicKindMap, State: wire.EpicListRunning, Section: wire.SectionImplementing},
	}}
	if err := printEpicList(&out, l, false); err != nil {
		t.Fatal(err)
	}
	if want := "proj:m-impl\tmap\trunning\t0/0\tqueueable=false\timplementing\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_NeedsYouRowsKeepTheServersOldestWaitOrder(t *testing.T) {
	var out bytes.Buffer
	l := wire.EpicList{Epics: []wire.EpicListRow{
		{Address: "proj:old", Kind: wire.EpicKindLoop, State: wire.EpicListPaused, Section: wire.SectionNeedsYou, WaitingSince: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Address: "proj:new", Kind: wire.EpicKindLoop, State: wire.EpicListMergeBlocked, Section: wire.SectionNeedsYou, WaitingSince: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}}
	if err := printEpicList(&out, l, false); err != nil {
		t.Fatal(err)
	}
	want := "proj:old\tloop\tpaused\t0/0\tqueueable=false\tneeds-you\nproj:new\tloop\tmerge-blocked\t0/0\tqueueable=false\tneeds-you\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_OneOffPrintsTicketAddressAndCardKind(t *testing.T) {
	var out bytes.Buffer
	l := wire.EpicList{Epics: []wire.EpicListRow{
		{Address: "proj:one-offs/01", Addresses: []string{"proj:one-offs/01"}, Kind: wire.EpicKindLoop, CardKind: wire.CardKindOneOff, State: wire.EpicListQueued, Section: wire.SectionImplementing},
	}}
	if err := printEpicList(&out, l, false); err != nil {
		t.Fatal(err)
	}
	if want := "proj:one-offs/01\tone-off\tqueued\t0/0\tqueueable=false\timplementing\n"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestPrintEpicList_FinishedIsCappedUnlessAllFinished(t *testing.T) {
	l := wire.EpicList{RecentlyFinished: 2, Epics: []wire.EpicListRow{
		{Address: "proj:a", Kind: wire.EpicKindLoop, State: wire.EpicListFinished, Section: wire.SectionFinished},
		{Address: "proj:b", Kind: wire.EpicKindLoop, State: wire.EpicListFinished, Section: wire.SectionFinished},
		{Address: "proj:c", Kind: wire.EpicKindLoop, State: wire.EpicListFinished, Section: wire.SectionFinished},
	}}
	for _, tc := range []struct {
		all  bool
		want int
	}{{false, 2}, {true, 3}} {
		var out bytes.Buffer
		if err := printEpicList(&out, l, tc.all); err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(out.String(), "\n"); got != tc.want {
			t.Errorf("all=%v printed %d lines, want %d: %q", tc.all, got, tc.want, out.String())
		}
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
		res     wire.QueueResult
		out     string
		errPart string
	}{
		"merged": {res: wire.QueueResult{Merged: true}, out: "merged\n"},
		"forked": {res: wire.QueueResult{MergeTicket: "proj:epic/07"}, out: "proj:epic/07\n"},
		"paused": {res: wire.QueueResult{Paused: true, PauseKind: "conflict", Message: "merge conflict in a.go"},
			errPart: "paused (conflict): merge conflict in a.go"},
		"paused needs-rebase": {res: wire.QueueResult{Paused: true, PauseKind: "needs-rebase", Message: "branch is behind trunk"},
			errPart: "paused (needs-rebase): branch is behind trunk"},
	}
	for _, code := range []string{
		wire.ReasonInvalidAddress, wire.ReasonUnknownTicket, wire.ReasonMapEpic, wire.ReasonAlreadyMerged,
		wire.ReasonMergePending, wire.ReasonNotComplete, wire.ReasonBranchMissing,
	} {
		cases[code] = struct {
			res     wire.QueueResult
			out     string
			errPart string
		}{res: wire.QueueResult{Refused: true, Reason: code, Message: "why " + code}, errPart: "refused (" + code + "): why " + code}
	}
	for name, tc := range cases {
		var out bytes.Buffer
		err := cmdbase.RunServerQueueWrite(context.Background(), nil, &out, false,
			func(context.Context, *apiclient.Client) (wire.QueueResult, error) { return tc.res, nil })
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
	err := cmdbase.RunServerQueueWrite(context.Background(), cl, &bytes.Buffer{}, false,
		func(ctx context.Context, cl *apiclient.Client) (wire.QueueResult, error) {
			return cl.Tickets.EpicMerge(ctx, "proj:epic")
		})
	if err == nil || !strings.Contains(err.Error(), wire.ReasonServerNotRunning) {
		t.Errorf("err = %v; want %s", err, wire.ReasonServerNotRunning)
	}
}
