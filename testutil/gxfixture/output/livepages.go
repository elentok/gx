package output

import (
	"context"
	"fmt"

	"github.com/elentok/gx/apiclient"
)

// LivePages names the draft and session pages the server behind cl holds, so
// an agent reads them from the output instead of looking them up.
func LivePages(ctx context.Context, cl *apiclient.Client, webURL string) (map[string]string, error) {
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
	return BuildPages(webURL, design, chat, sessions), nil
}
