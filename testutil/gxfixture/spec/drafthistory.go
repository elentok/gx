package spec

import (
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/tickets"
)

// Texts, names and slugs of the draft-history scenario. The e2e flows hard-code
// them; webui/e2e/CATALOG.md lists them.
const (
	// HistoryDesignSlug is the map of the scenario's Design (and of its Design session).
	HistoryDesignSlug = "history-design"
	// HistoryQueueEpic is the epic whose ticket the Queue agent works.
	HistoryQueueEpic = "history-queue"
	// HistoryOtherProject is a project other than the one the seeded draft page is in.
	HistoryOtherProject = "other"

	HistoryDraftIdea     = "Idea of the history draft page"
	HistoryChatBrief     = "Brief of the history chat draft"
	HistoryDesignGone    = "History design draft with a deleted image"
	HistoryDesignDropped = "History design draft with images not kept"
	HistoryDesignOther   = "History design draft of another project"
	// HistoryDesignExists has the slug HistoryDesignSlug, which now belongs to a Design.
	HistoryDesignExists = "History design draft whose Design exists"
	HistoryDesignImages = "History design draft with two images"
	HistoryDesignPlain  = "History design draft plain A"
	HistoryDesignPlainB = "History design draft plain B"
	HistoryDesignPlainC = "History design draft plain C"

	HistoryChatClaude  = "History chat draft of Claude"
	HistoryChatCodex   = "History chat draft of Codex"
	HistoryChatImages  = "History chat draft with an image"
	HistoryChatPlainA  = "History chat draft plain A"
	HistoryChatPlainB  = "History chat draft plain B"
	HistoryChatPlainC  = "History chat draft plain C"
	HistorySentChat    = "History sent chat message"
	HistorySentDesign  = "History sent design message"
	HistorySentCommand = "/history-command run it"
	HistorySentImages  = "" // an images-only entry has no text
	HistorySentPlainA  = "History sent plain A"
	HistorySentPlainB  = "History sent plain B"
	HistorySentPlainC  = "History sent plain C"
	HistorySentWithImg = "History sent message with images"
)

// Names of the sessions the history entries and the seeded sessions carry.
const (
	HistoryChatDraftName = "history-chat-draft"
	HistorySentChatName  = "history-sent-chat"
	HistorySentSlug      = "history-sent-slug"
)

// The three history lists a HistoryEntry can name.
const (
	HistoryListChatDrafts   = "chat-drafts"
	HistoryListDesignDrafts = "design-drafts"
	HistoryListSent         = "sent"
)

// HistoryEntry is one Draft history entry. Entries of one list are listed
// oldest first, so the last one is the newest. The entry's images are PNGs.
type HistoryEntry struct {
	List                                                                string
	Text, Project, Slug, Name, Harness, Model, Effort, Folder, Composer string
	Images                                                              int
	// DeleteImages removes the saved image files from disk after the save, so the
	// record still lists them.
	DeleteImages bool
	// DropImages saves the entry with a tiny cap, so its images are not kept. It
	// must be the oldest entry of its list: a later save under the same cap would
	// evict the others.
	DropImages bool
}

// DraftHistoryScenario starts a server whose Draft history holds entries in all
// three lists, plus the pages and sessions the Draft history popup flow opens:
// a Design draft with text, a chat draft with a brief, a plain Claude chat, a
// plain Codex chat, a Design session, a Fix agent and a Queue agent.
func DraftHistoryScenario() scenario {
	design := project + ":" + HistoryDesignSlug
	return scenario{
		Chat:     true,
		Codex:    true,
		Hold:     true,
		Projects: []string{HistoryOtherProject},
		Epics: []epic{
			{project, HistoryDesignSlug, nil},
			{project, HistoryQueueEpic, []ticket{{"01", "held", servertest.TicketOpts{}}}},
		},
		DesignSteps: map[string]string{design: string(tickets.StepResolveTickets)},
		OwnRepos:    []string{HistoryOtherProject},
		Queue:       []string{project + ":" + HistoryQueueEpic},
		// Fix agent: the project-level one a failed migration starts.
		FailedMigration: project,
		PromptGrace:     "1s",
		Drafts:          []Draft{{Project: project, Slug: "history-draft", Idea: HistoryDraftIdea}},
		ChatDrafts: []ChatDraft{
			{Project: project, Name: HistoryChatDraftName, Harness: "claude", Model: StrongestModel, Effort: "max",
				Folder: ops.ChatDraftFolderSource, Brief: HistoryChatBrief},
		},
		Sessions: []sessionSpec{
			{Project: project, Prompt: "hello", Read: true},
			{Project: project, Harness: "codex", Prompt: "hello", Read: true},
			{Project: project, Prompt: "hello", Link: design, Read: true},
		},
		History: historyEntries(),
	}
}

func historyEntries() []HistoryEntry {
	return []HistoryEntry{
		// Design drafts, oldest first. The marked entry comes first (see DropImages).
		{List: HistoryListDesignDrafts, Text: HistoryDesignDropped, Project: project, Slug: "dropped-slug", Images: 2, DropImages: true},
		{List: HistoryListDesignDrafts, Text: HistoryDesignPlainC, Project: project, Slug: "plain-c-slug"},
		{List: HistoryListDesignDrafts, Text: HistoryDesignPlainB, Project: project, Slug: "plain-b-slug"},
		{List: HistoryListDesignDrafts, Text: HistoryDesignPlain, Project: project, Slug: "plain-a-slug"},
		{List: HistoryListDesignDrafts, Text: HistoryDesignImages, Project: project, Slug: "images-slug", Images: 2},
		{List: HistoryListDesignDrafts, Text: HistoryDesignOther, Project: HistoryOtherProject, Slug: "other-slug"},
		{List: HistoryListDesignDrafts, Text: HistoryDesignGone, Project: project, Slug: "gone-slug", Images: 1, DeleteImages: true},
		{List: HistoryListDesignDrafts, Text: HistoryDesignExists, Project: project, Slug: HistoryDesignSlug},

		// Chat drafts.
		{List: HistoryListChatDrafts, Text: HistoryChatPlainC, Project: project, Name: "chat-c", Harness: "claude", Model: StrongestModel, Effort: "low", Folder: ops.ChatDraftFolderSource},
		{List: HistoryListChatDrafts, Text: HistoryChatPlainB, Project: project, Name: "chat-b", Harness: "claude", Model: "sonnet", Effort: "medium", Folder: ops.ChatDraftFolderSource},
		{List: HistoryListChatDrafts, Text: HistoryChatPlainA, Project: project, Name: "chat-a", Harness: "codex", Model: "gpt-5.5-codex", Effort: "low", Folder: ops.ChatDraftFolderSource},
		{List: HistoryListChatDrafts, Text: HistoryChatImages, Project: project, Name: "chat-img", Harness: "claude", Model: StrongestModel, Effort: "high", Folder: ops.ChatDraftFolderWorktree, Images: 1},
		{List: HistoryListChatDrafts, Text: HistoryChatCodex, Project: project, Name: "chat-codex", Harness: "codex", Model: "gpt-5.5-codex", Effort: "high", Folder: ops.ChatDraftFolderSource},
		{List: HistoryListChatDrafts, Text: HistoryChatClaude, Project: project, Name: "chat-claude", Harness: "claude", Model: StrongestModel, Effort: "max", Folder: ops.ChatDraftFolderSource},

		// Sent messages.
		{List: HistoryListSent, Text: HistorySentPlainC, Project: project, Name: "sent-c", Harness: "claude", Composer: "chat"},
		{List: HistoryListSent, Text: HistorySentPlainB, Project: project, Name: "sent-b", Harness: "claude", Composer: "chat"},
		{List: HistoryListSent, Text: HistorySentPlainA, Project: project, Slug: "sent-a-slug", Composer: "design"},
		{List: HistoryListSent, Text: HistorySentWithImg, Project: project, Name: "sent-img", Harness: "claude", Composer: "chat", Images: 2},
		{List: HistoryListSent, Text: HistorySentChat, Project: project, Name: HistorySentChatName, Harness: "claude", Composer: "chat"},
		{List: HistoryListSent, Text: HistorySentDesign, Project: project, Slug: HistorySentSlug, Composer: "design"},
		{List: HistoryListSent, Text: HistorySentImages, Project: project, Name: "sent-only-images", Harness: "claude", Composer: "chat", Images: 1},
		{List: HistoryListSent, Text: HistorySentCommand, Project: project, Name: "sent-command", Harness: "claude", Composer: "chat"},
	}
}
