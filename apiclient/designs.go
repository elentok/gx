package apiclient

import (
	"context"
	"net/url"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/wire/ops"
)

// designs holds the Design methods of Sessions. Sessions embeds it, so callers
// still reach them as c.Sessions.OpenDesign and so on.
type designs struct {
	t *transport
}

// OpenDesign returns the Design's live session or starts one. A refusal is a
// result (Refused set), not an error.
func (s *designs) OpenDesign(ctx context.Context, req wire.DesignOpenRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/designs/open", req, &res)
	return res, err
}

// DeleteDesign deletes an uncharted design's epic. A refusal is a result
// (Refused set), not an error.
func (s *designs) DeleteDesign(ctx context.Context, req wire.DesignDeleteRequest) (wire.SessionResult, error) {
	var res wire.SessionResult
	err := s.t.post(ctx, "/v1/designs/delete", req, &res)
	return res, err
}

// DesignWorktree returns a map's design worktree, made and rebased if needed,
// or the trunk checkout after Branch. A refusal is a result (Refused set), not
// an error.
func (s *designs) DesignWorktree(ctx context.Context, req wire.DesignWorktreeRequest) (wire.DesignWorktreeResult, error) {
	var res wire.DesignWorktreeResult
	err := s.t.post(ctx, "/v1/designs/worktree", req, &res)
	return res, err
}

// DesignDrafts lists the New Design drafts, most recently edited first.
func (s *designs) DesignDrafts(ctx context.Context) ([]ops.DesignDraftItem, error) {
	var items []ops.DesignDraftItem
	err := s.t.get(ctx, "/v1/designs/drafts", &items)
	return items, err
}

// DesignDraft reads one draft. A gone draft is a refused result, not an error.
func (s *designs) DesignDraft(ctx context.Context, id string) (ops.DesignDraftSaveResult, error) {
	var res ops.DesignDraftSaveResult
	err := s.t.get(ctx, "/v1/designs/drafts/read?id="+url.QueryEscape(id), &res)
	return res, err
}

// SaveDesignDraft saves a draft's text fields against req.Revision. A stale
// revision or a gone draft is a refused result, not an error.
func (s *designs) SaveDesignDraft(ctx context.Context, req ops.DesignDraftSaveRequest) (ops.DesignDraftSaveResult, error) {
	var res ops.DesignDraftSaveResult
	err := s.t.post(ctx, "/v1/designs/drafts/save", req, &res)
	return res, err
}

// AddDesignDraftImage saves one image, as its raw bytes, on a draft. A refusal
// is a result (Refused set), not an error.
func (s *designs) AddDesignDraftImage(ctx context.Context, id string, data []byte) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.postBody(ctx, "/v1/designs/drafts/images?id="+url.QueryEscape(id), "application/octet-stream", data, &res)
	return res, err
}

// DesignDraftImage reads one image of a draft: its bytes and detected Content-Type.
func (s *designs) DesignDraftImage(ctx context.Context, id, image string) ([]byte, string, error) {
	return s.t.getBytes(ctx, "/v1/designs/drafts/images?id="+url.QueryEscape(id)+"&image="+url.QueryEscape(image))
}

// DesignImages lists the ids and file names in a map epic's images/ folder.
func (s *designs) DesignImages(ctx context.Context, epic string) ([]wire.SessionImageResult, error) {
	var res []wire.SessionImageResult
	err := s.t.get(ctx, "/v1/designs/images?epic="+url.QueryEscape(epic), &res)
	return res, err
}

// DesignImage reads one image of a map epic's images/ folder.
func (s *designs) DesignImage(ctx context.Context, epic, id string) ([]byte, string, error) {
	return s.t.getBytes(ctx, "/v1/designs/images/read?epic="+url.QueryEscape(epic)+"&id="+url.QueryEscape(id))
}

// RemoveDesignDraftImage deletes one image of a draft. A refusal is a result.
func (s *designs) RemoveDesignDraftImage(ctx context.Context, id, image string) (wire.SessionImageResult, error) {
	var res wire.SessionImageResult
	err := s.t.post(ctx, "/v1/designs/drafts/images/delete", wire.DraftImageRequest{ID: id, Image: image}, &res)
	return res, err
}

// DeleteDesignDraftdeletes a draft. A draft already gone is not an error.
func (s *designs) DeleteDesignDraft(ctx context.Context, id string) (ops.DesignDraftSaveResult, error) {
	var res ops.DesignDraftSaveResult
	err := s.t.post(ctx, "/v1/designs/drafts/delete", ops.DesignDraftDeleteRequest{ID: id}, &res)
	return res, err
}

// DesignLastProject reads the project the New Design form last used ("" on a
// fresh server).
func (s *designs) DesignLastProject(ctx context.Context) (ops.DesignLastProject, error) {
	var res ops.DesignLastProject
	err := s.t.get(ctx, "/v1/designs/drafts/last-project", &res)
	return res, err
}

// SetDesignLastProject records the last-used project; the newest write wins.
func (s *designs) SetDesignLastProject(ctx context.Context, project string) (ops.DesignLastProject, error) {
	var res ops.DesignLastProject
	err := s.t.post(ctx, "/v1/designs/drafts/last-project", ops.DesignLastProject{Project: project}, &res)
	return res, err
}
