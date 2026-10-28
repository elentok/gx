package server_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/tickets"
)

// answerSource is the source of the ticket's only pause.answer line.
func answerSource(t *testing.T, ticketPath string) string {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Type == tickets.EventPauseAnswer {
			return ev.Source
		}
	}
	t.Fatal("no pause.answer line in the log")
	return ""
}

func TestOrigin_WebAnswerLogsWeb(t *testing.T) {
	t.Parallel()
	h, ticketPath, _ := startWaitingRestartFake(t, 0, func(c *server.Config) { c.WebAddr = "127.0.0.1:0" })
	waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
	state := pauseWithQuestion(t, ticketPath, optionsBlock)
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake = %+v, %v; want a web_url", hs, err)
	}
	body, _ := json.Marshal(server.QueueRequest{Address: answerAddr, State: state, Choice: "a", Source: "cli"})
	cookie := login(t, h.StateDir, hs.WebURL)
	if code, out := webPostJSON(t, hs.WebURL+"/v1/tickets/answer", string(body), cookie, "", jsonPost(hs.WebURL)); code != 200 {
		t.Fatalf("web answer = %d %s", code, out)
	}
	if got := answerSource(t, ticketPath); got != "web" {
		t.Errorf("source = %q, want web (the body's claim is ignored)", got)
	}
}

func TestOrigin_SocketAnswerLogsTUIOrCLI(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mark func(*testing.T, *servertest.Harness)
		want string
	}{
		"unmarked client": {func(*testing.T, *servertest.Harness) {}, "cli"},
		"tui client":      {func(_ *testing.T, h *servertest.Harness) { h.Client.AsTUI() }, "tui"},
	} {
		t.Run(name, func(t *testing.T) {
			h, ticketPath, _ := startWaitingRestartFake(t, 0)
			waitFor(t, func() bool { return len(h.Server.Runs()) == 1 })
			state := pauseWithQuestion(t, ticketPath, optionsBlock)
			tc.mark(t, h)
			res, err := h.Client.TicketAnswer(context.Background(), server.QueueRequest{Address: answerAddr, State: state, Choice: "a"})
			if err != nil || res.Refused {
				t.Fatalf("answer = %+v, %v", res, err)
			}
			if got := answerSource(t, ticketPath); got != tc.want {
				t.Errorf("source = %q, want %q", got, tc.want)
			}
		})
	}
}
