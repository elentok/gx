package ops

import "time"

// Refusal reasons of a New Design draft text save.
const (
	// ReasonDraftRevisionConflict: the caller saved against a revision that is no longer current.
	ReasonDraftRevisionConflict = "revision-conflict"
	// ReasonDraftGone: the draft was deleted (or emptied) since the caller loaded it.
	ReasonDraftGone = "draft-gone"
	// ReasonDraftUnknownWorkspace: a create named a workspace that does not exist; no draft was stored.
	ReasonDraftUnknownWorkspace = "unknown-workspace"
	// ReasonDraftNotEmpty: remove-if-empty found content, so the draft stays.
	ReasonDraftNotEmpty = "draft-not-empty"
)

// DesignDraft is one New Design form kept on the server. Slug may be empty;
// Images lists image ids the server alone writes.
type DesignDraft struct {
	ID       string    `json:"id"`
	Project  string    `json:"project"`
	Slug     string    `json:"slug"`
	Idea     string    `json:"idea"`
	Images   []string  `json:"images"`
	Revision int       `json:"revision"`
	EditedAt time.Time `json:"edited_at"`
	// Label is computed by the server on every read (never stored).
	Label string `json:"label"`
}

// DesignDraftItem is one row of the draft list: the inputs of its label and
// how many images it holds.
type DesignDraftItem struct {
	ID         string    `json:"id"`
	Project    string    `json:"project"`
	Slug       string    `json:"slug"`
	Idea       string    `json:"idea"`
	Label      string    `json:"label"`
	ImageCount int       `json:"image_count"`
	EditedAt   time.Time `json:"edited_at"`
}

// DesignDraftSaveRequest saves the prompt of a draft. Revision is the one the
// caller last saw. It carries neither the slug, the project nor the image
// list: those have their own writes, so a stale save cannot overwrite them. ID
// names the draft on the save route; the store takes the id separately.
type DesignDraftSaveRequest struct {
	ID       string `json:"id,omitempty"`
	Idea     string `json:"idea"`
	Revision int    `json:"revision"`
	// KeepEditedAt saves without moving the edited-at time, for a change the
	// person did not make (the form moving a draft off a deleted project).
	KeepEditedAt bool `json:"keep_edited_at,omitempty"`
	// Workspace is the workspace a newly created draft joins. Read only when the
	// request creates a draft (no ID); ignored on a save of an existing draft.
	Workspace string `json:"workspace,omitempty"`
}

// DesignDraftTargetRequest sets a draft's slug and project. It is revision-free
// and never bumps Revision. An empty Slug clears it. KeepEditedAt writes
// without moving the edited-at time, as on the text save.
type DesignDraftTargetRequest struct {
	ID           string `json:"id"`
	Project      string `json:"project"`
	Slug         string `json:"slug"`
	KeepEditedAt bool   `json:"keep_edited_at,omitempty"`
}

// DesignLastProject is the project the New design and New chat forms last
// used (both share it), shared by every browser: the answer of the read route and the body of the set route.
// Project is empty on a fresh server.
type DesignLastProject struct {
	Project string `json:"project"`
}

// DesignDraftDeleteRequest deletes one draft.
type DesignDraftDeleteRequest struct {
	ID string `json:"id"`
}

// DesignDraftLeaseRequest pings the page lease of one draft; the remove-if-empty
// route takes the same body.
type DesignDraftLeaseRequest struct {
	ID string `json:"id"`
}

// DesignDraftSaveResult answers a text save, and a draft read (Draft set, or
// refused draft-gone) and delete (Deleted). A refusal is a result: Reason is
// one of the ReasonDraft* constants. Deleted: the save left the draft empty and
// removed it. Draft is set on a successful, non-deleting save.
type DesignDraftSaveResult struct {
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Deleted bool         `json:"deleted,omitempty"`
	Draft   *DesignDraft `json:"draft,omitempty"`
	// Started is the Design address ("project:epic") a gone draft was started as.
	Started string `json:"started,omitempty"`
}
