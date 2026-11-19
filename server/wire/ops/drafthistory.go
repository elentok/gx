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
