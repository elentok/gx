package cmd

import (
	"context"
	"time"

	"github.com/elentok/gx/apiclient"
)

const pingTimeout = time.Second

// pingServer tells a running server that a direct write changed path's ticket,
// so the stream updates at once. Best effort: the file is already written and
// is the truth, so a down server (or any failure) is silent — the server's own
// watch and poll pick the change up when it runs.
func pingServer(path string) {
	pingLabels(serverClient, ticketLabel(path))
}

// pingLabels is pingServer for callers that already hold the labels, such as a
// path that no longer exists, through the given dialer.
func pingLabels(dial func() (*apiclient.Client, error), labels ...string) {
	cl, err := dial()
	if err != nil {
		return
	}
	for _, l := range labels {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		_ = cl.TicketChanged(ctx, l)
		cancel()
	}
}
