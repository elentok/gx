package main

import (
	"context"
	"fmt"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/gxfixture/output"
)

// pages names the draft and session pages the seeded server holds, so an agent
// reads them from the output instead of looking them up.
func (f *fixture) pages(ctx context.Context, webURL string) (map[string]string, error) {
	cl := apiclient.New(server.SocketPath(f.stateD))
	snap, err := cl.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("pages: snapshot: %w", err)
	}
	list, err := cl.Sessions.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("pages: sessions: %w", err)
	}
	var design, chat, sessions []string
	for _, d := range snap.DesignDrafts {
		design = append(design, d.ID)
	}
	for _, d := range snap.ChatDrafts {
		chat = append(chat, d.ID)
	}
	for _, si := range list {
		sessions = append(sessions, si.ID)
	}
	return output.BuildPages(webURL, design, chat, sessions), nil
}
