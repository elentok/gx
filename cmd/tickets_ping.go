package cmd

import (
	"context"
	"time"
)

const pingTimeout = time.Second

// pingServer tells a running server that a direct write changed path's ticket,
// so the stream updates at once. Best effort: the file is already written and
// is the truth, so a down server (or any failure) is silent — the server's own
// watch and poll pick the change up when it runs.
func pingServer(path string) {
	cl, err := serverClient()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	_ = cl.TicketChanged(ctx, ticketLabel(path))
}
