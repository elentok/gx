package wire

// CardSection is the Epics page section a card belongs to. The server decides it;
// the web client does not derive sections.
type CardSection string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	SectionNeedsYou      CardSection = "needs-you"
	SectionImplementing  CardSection = "implementing"
	SectionDesigning     CardSection = "designing"
	SectionReadyToQueue  CardSection = "ready-to-queue"
	SectionReadyToDesign CardSection = "ready-to-design"
	SectionFinished      CardSection = "finished"
)

// CardKind is the kind of an Epics page card.
type CardKind string

const (
	CardKindImplementation CardKind = "implementation"
	CardKindDesign         CardKind = "design"
	CardKindOneOff         CardKind = "one-off"
	CardKindFollowUps      CardKind = "follow-ups"
)

// CardActionKind names what a card button does.
type CardActionKind string

const (
	CardActionQueue      CardActionKind = "queue"
	CardActionAnswer     CardActionKind = "answer"
	CardActionOpenTicket CardActionKind = "open-ticket"
	CardActionOpenEpic   CardActionKind = "open-epic"
	CardActionMerge      CardActionKind = "merge"
	// StashMerge targets the epic like Merge; its route is a later ticket's.
	CardActionStashMerge CardActionKind = "stash-merge"
	// The Design actions target the Design's epic address. Design and Continue
	// start a session first (POST open-design); Answer and Open only navigate.
	CardActionDesign   CardActionKind = "design"
	CardActionContinue CardActionKind = "continue"
	CardActionOpen     CardActionKind = "open"
	// Triage targets the project whose follow-ups the card counts.
	CardActionTriage CardActionKind = "triage"
)

// CardAction is one button of a card. Target is the address the button opens (a
// ticket for Answer and Open ticket, an epic for Open epic) or acts on (Merge).
type CardAction struct {
	Kind   CardActionKind `json:"kind"`
	Label  string         `json:"label"`
	Target string         `json:"target,omitempty"`
	// Link is the hash route the button opens, when the server decides it. Answer
	// sets it for a Design question; the client falls back to the ticket page.
	Link string `json:"link,omitempty"`
}

// CardTimeKind says what the card's start time measures; the client words it.
type CardTimeKind string

// Spelled as literals for tygo; see the comment on EpicListState.
const (
	CardTimeElapsed  CardTimeKind = "elapsed"
	CardTimeWaiting  CardTimeKind = "waiting"
	CardTimeCreated  CardTimeKind = "created"
	CardTimeFinished CardTimeKind = "finished"
)
