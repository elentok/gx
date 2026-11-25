// Package draftseed creates a scenario's drafts through the running server's
// real create routes. It is apart from the fixture program to keep that package
// under its size limit.
package draftseed

import (
	"context"
	"fmt"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// Late creates the AfterStart drafts. Scenario order matters: creating a draft
// sweeps older unleased empty drafts, so list the empty one last.
func Late(ctx context.Context, stateD string, list []spec.Draft) error {
	cl := apiclient.New(server.SocketPath(stateD))
	for _, d := range list {
		if !d.AfterStart {
			continue
		}
		if res, err := cl.Sessions.SaveDesignDraft(ctx, ops.DesignDraftSaveRequest{Idea: d.Idea}); err != nil || res.Refused {
			return fmt.Errorf("late draft %q: %v %+v", d.Slug, err, res)
		}
	}
	return nil
}

// Chat creates the scenario's chat drafts.
func Chat(ctx context.Context, stateD string, list []spec.ChatDraft) error {
	cl := apiclient.New(server.SocketPath(stateD))
	for _, d := range list {
		res, err := cl.Sessions.CreateChatDraft(ctx, ops.ChatDraftCreateRequest{
			Project: d.Project, Brief: d.Brief, Name: d.Name, Harness: d.Harness,
			Model: d.Model, Effort: d.Effort, Folder: d.Folder, Workspace: d.Workspace,
		})
		if err != nil || res.Refused {
			return fmt.Errorf("chat draft %q: %v %+v", d.Name, err, res)
		}
	}
	return nil
}
