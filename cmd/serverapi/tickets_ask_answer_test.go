package serverapi

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/cmd/cmdtest"
	"github.com/elentok/gx/server/wire"
)

func TestAskCmd_ConfirmsTheRecordedQuestion(t *testing.T) {
	sock := filepath.Join(cmdtest.ShortTempDir(t), "a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tickets/ask", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wire.QueueResult{})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	var out bytes.Buffer
	var d Deps
	d.NewClient = func() (*apiclient.Client, error) { return apiclient.New(sock), nil }
	c := NewAskCmd(d)
	c.SetOut(&out)
	c.SetArgs([]string{"p:e/01", "--body", "## Question\nPick a look\n\n## Handoff\nnothing yet"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "p:e/01") || !strings.Contains(got, "Pick a look") {
		t.Errorf("output = %q; want the address and the question summary", got)
	}
}

func TestAskCmd_RefusesLongSummaryBeforeTheServerAndSavesTheBody(t *testing.T) {
	var d Deps
	d.NewClient = func() (*apiclient.Client, error) {
		t.Fatal("server dialed for an over-long summary")
		return nil, nil
	}
	body := "## Question\n" + strings.Repeat("x", 130) + "\n\n## Handoff\nnothing yet"
	c := NewAskCmd(d)
	c.SetArgs([]string{"p:e/01", "--body", body})
	c.SilenceUsage = true
	err := c.Execute()
	if err == nil {
		t.Fatal("over-long summary accepted")
	}
	msg := err.Error()
	if !strings.Contains(msg, "max 120") {
		t.Errorf("error = %q; want the limit", msg)
	}
	_, rest, ok := strings.Cut(msg, "saved to ")
	if !ok {
		t.Fatalf("error = %q; want a saved body path", msg)
	}
	path := strings.Fields(rest)[0]
	saved, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	if string(saved) != body {
		t.Errorf("saved body = %q; want the original", saved)
	}
}

func TestAskCmd_HelpStatesTheSummaryLimit(t *testing.T) {
	if long := NewAskCmd(Deps{}).Long; !strings.Contains(long, "120") {
		t.Errorf("Long = %q; want the 120-character limit", long)
	}
}

func TestAskAndAnswerCmds_ArgAndJSONFlag(t *testing.T) {
	cases := map[string]*cobra.Command{
		"ask":               NewAskCmd(Deps{}),
		"answer":            NewAnswerCmd(Deps{}, false),
		"answer serverTree": NewAnswerCmd(Deps{}, true),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.Flags().Lookup("json") == nil {
				t.Fatal("missing --json")
			}
			if err := c.Args(c, nil); err == nil {
				t.Error("no argument accepted")
			}
			if err := c.Args(c, []string{"p:e/01"}); err != nil {
				t.Errorf("one argument refused: %v", err)
			}
			if err := c.Args(c, []string{"a", "b"}); err == nil {
				t.Error("two arguments accepted")
			}
		})
	}
	if NewAnswerCmd(Deps{}, false).Flags().Lookup("state") != nil {
		t.Error("tickets answer has --state")
	}
	if NewAnswerCmd(Deps{}, true).Flags().Lookup("state") == nil {
		t.Error("server tickets answer lacks --state")
	}
}
