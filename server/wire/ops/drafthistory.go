package ops

import "time"

// DraftHistoryListRequest names one of the three lists: "chat-drafts",
// "design-drafts" or "sent".
type DraftHistoryListRequest struct {
	List string `json:"list"`
}

// DraftHistoryEntry is one popup row. Fields that do not apply to the list's
// entry kind are empty. ImagesDropped replaces the count in the popup.
type DraftHistoryEntry struct {
	ID            string    `json:"id"`
	SavedAt       time.Time `json:"savedAt"`
	Text          string    `json:"text"`
	ImageCount    int       `json:"imageCount"`
	ImagesDropped bool      `json:"imagesDropped"`
	Project       string    `json:"project"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Harness       string    `json:"harness"`
	Model         string    `json:"model"`
	Effort        string    `json:"effort"`
	Folder        string    `json:"folder"`
	// Composer is "chat" or "design" for a Sent message.
	Composer string `json:"composer"`
}

// DraftHistoryListResult is one list, newest first. Max is the list cap.
type DraftHistoryListResult struct {
	List    string              `json:"list"`
	Max     int                 `json:"max"`
	Entries []DraftHistoryEntry `json:"entries"`
}

// ReasonHistoryEntryGone: a pick named an entry that is no longer in the list
// (it was evicted since the popup loaded).
const ReasonHistoryEntryGone = "history-entry-gone"

// DraftHistoryPickRequest picks one entry onto a draft page. Revision is the
// page's revision; a stale one is refused with ReasonDraftRevisionConflict.
type DraftHistoryPickRequest struct {
	EntryID  string `json:"entry_id"`
	DraftID  string `json:"draft_id"`
	Revision int    `json:"revision"`
}

// ComposerPickRequest picks a Sent messages entry into a session's composer.
// The composer keeps its text and image ids in the browser, so the call
// carries them. It takes no revision: composers have none.
type ComposerPickRequest struct {
	SessionID string   `json:"session_id"`
	EntryID   string   `json:"entry_id"`
	Text      string   `json:"text"`
	Images    []string `json:"images"`
}

// ComposerPickResult answers a composer pick. Text and Images are what the
// composer holds afterwards (the request's own when nothing changed). Images
// are new image ids of the session folder. Notices say what the restore left out.
type ComposerPickResult struct {
	Refused bool     `json:"refused,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Message string   `json:"message,omitempty"`
	Text    string   `json:"text"`
	Images  []string `json:"images"`
	Notices []string `json:"notices"`
}

// DesignDraftPickResult answers a pick on a Design draft page. Draft is the
// page after the pick (the same draft when nothing changed). Notices say what
// the restore left out.
type DesignDraftPickResult struct {
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Draft   *DesignDraft `json:"draft,omitempty"`
	Notices []string     `json:"notices"`
}

// ChatDraftPickResult answers a pick on a Chat draft page, like DesignDraftPickResult.
type ChatDraftPickResult struct {
	Refused bool       `json:"refused,omitempty"`
	Reason  string     `json:"reason,omitempty"`
	Draft   *ChatDraft `json:"draft,omitempty"`
	Notices []string   `json:"notices"`
}
