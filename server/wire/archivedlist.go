package wire

import "time"

// ArchivedEpicRow is one row of the archived list: a loop epic, or a Design
// with its -impl epic as one row.
type ArchivedEpicRow struct {
	// Address is the archive address, with any `-N` suffix, and no `.archive` segment.
	Address string `json:"address"`
	Project string `json:"project"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	// ArchivedAt and CompletedAt are absent when the epic has no stamp.
	ArchivedAt  time.Time `json:"archived_at,omitzero"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
	// Kind is EpicKindMap for a Design pair, else EpicKindLoop.
	Kind string `json:"kind"`
	// Addresses is the Design's address then its -impl address for a pair, else just Address.
	Addresses []string `json:"addresses"`
	// Workspace is the workspace the row was archived from; a name that no longer exists is the Default workspace.
	Workspace string `json:"workspace"`
}

// ArchivedEpicList is the archived list, newest archived_at first.
type ArchivedEpicList struct {
	Epics []ArchivedEpicRow `json:"epics"`
}
