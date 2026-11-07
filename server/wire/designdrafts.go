package wire

import "time"

// Refusal reasons of a New Design draft text save.
const (
	// ReasonDraftRevisionConflict: the caller saved against a revision that is no longer current.
	ReasonDraftRevisionConflict = "revision-conflict"
	// ReasonDraftGone: the draft was deleted (or emptied) since the caller loaded it.
	ReasonDraftGone = "draft-gone"
)

// DesignDraft is one New Design form kept on the server. Slug may be empty;
// Images lists image ids the server alone writes.
type DesignDraft struct {
	ID       string    `json:"id"`
	Project  string    `json:"project"`
	Slug     string    `json:"slug"`
	Idea     string    `json:"idea"`
	Name     string    `json:"name,omitempty"`
	Images   []string  `json:"images"`
	Revision int       `json:"revision"`
	EditedAt time.Time `json:"edited_at"`
}

// DesignDraftItem is one row of the draft list: the inputs of its label and
// how many images it holds.
type DesignDraftItem struct {
	ID         string    `json:"id"`
	Project    string    `json:"project"`
	Slug       string    `json:"slug"`
	Idea       string    `json:"idea"`
	Name       string    `json:"name,omitempty"`
	ImageCount int       `json:"image_count"`
	EditedAt   time.Time `json:"edited_at"`
}

// DesignDraftSaveRequest saves the text fields of a draft. Revision is the one
// the caller last saw. It never carries the image list. ID names the draft on
// the save route; the store takes the id separately.
type DesignDraftSaveRequest struct {
	ID       string `json:"id,omitempty"`
	Project  string `json:"project"`
	Slug     string `json:"slug"`
	Idea     string `json:"idea"`
	Name     string `json:"name,omitempty"`
	Revision int    `json:"revision"`
}

// DesignLastProject is the project the New Design form last used, shared by
// every browser: the answer of the read route and the body of the set route.
// Project is empty on a fresh server.
type DesignLastProject struct {
	Project string `json:"project"`
}

// DesignDraftDeleteRequest deletes one draft.
type DesignDraftDeleteRequest struct {
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
}
