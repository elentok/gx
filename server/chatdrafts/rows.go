package chatdrafts

import "github.com/elentok/gx/server/wire"

// Exists reports whether a chat draft with this id is stored.
func (c *ChatDrafts) Exists(id string) bool { return c.store.Exists(id) }

// Rows lists the drafts as snapshot rows, most recently edited first. The row
// carries no brief. workspace names the workspace of a member key.
func (c *ChatDrafts) Rows(workspace func(key string) string) []wire.ChatDraftRow {
	items, err := c.store.List()
	if err != nil {
		if c.Log != nil {
			c.Log.Warn("list chat drafts", "err", err)
		}
		return nil
	}
	rows := make([]wire.ChatDraftRow, 0, len(items))
	for _, it := range items {
		key := MemberKey(it.ID)
		rows = append(rows, wire.ChatDraftRow{
			ID: it.ID, Label: it.Label, Project: it.Project, Harness: it.Harness,
			EditedAt: it.EditedAt, Workspace: workspace(key), MemberKey: key,
		})
	}
	return rows
}
