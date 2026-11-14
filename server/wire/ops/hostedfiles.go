package ops

import "time"

// HostedKind is how the viewer shows a hosted file. The values are literals
// because tygo cannot resolve constants from another package.
type HostedKind string

const (
	HostedKindMarkdown HostedKind = "markdown"
	HostedKindMermaid  HostedKind = "mermaid"
	HostedKindHTML     HostedKind = "html"
	HostedKindImage    HostedKind = "image"
	HostedKindSVG      HostedKind = "svg"
	HostedKindPDF      HostedKind = "pdf"
	HostedKindCode     HostedKind = "code"
	HostedKindText     HostedKind = "text"
	HostedKindBinary   HostedKind = "binary"
)

// HostedVerdict is the answer for one resolve candidate. Denied and out-of-root
// candidates are "missing": the route is no path oracle.
type HostedVerdict string

const (
	HostedFound   HostedVerdict = "found"
	HostedMissing HostedVerdict = "missing"
)

// HostedResolveRequest is the POST /v1/hosted-files/resolve body behind
// `gx files resolve`. At most one of Session, Project and Dir is set; with none,
// only absolute and "~/" candidates resolve.
type HostedResolveRequest struct {
	Candidates []string `json:"candidates"`
	// Session is a session id; its folder is the base.
	Session string `json:"session,omitempty"`
	// Project is a project name; its main worktree is the base.
	Project string `json:"project,omitempty"`
	// Dir is a folder base.
	Dir string `json:"dir,omitempty"`
}

// HostedResolved is the answer for one candidate.
type HostedResolved struct {
	Candidate string        `json:"candidate"`
	Verdict   HostedVerdict `json:"verdict"`
	// Path, Size, Kind and Modified are set only when the verdict is "found".
	Path     string     `json:"path,omitempty"`
	Size     int64      `json:"size,omitempty"`
	Kind     HostedKind `json:"kind,omitempty"`
	Modified time.Time  `json:"modified,omitzero"`
}

// HostedResolveResult is the resolve reply: one entry per candidate, in request
// order, or a refusal.
type HostedResolveResult struct {
	Files   []HostedResolved `json:"files,omitempty"`
	Refused bool             `json:"refused,omitempty"`
	Reason  string           `json:"reason,omitempty"`
	Message string           `json:"message,omitempty"`
}

// Refusal and Refuse let the CLI treat the payload as a Refusable.
func (r HostedResolveResult) Refusal() (bool, string, string) { return r.Refused, r.Reason, r.Message }
func (HostedResolveResult) Refuse(reason, message string) HostedResolveResult {
	return HostedResolveResult{Refused: true, Reason: reason, Message: message}
}

// MaxHostedCandidates is how many candidates one resolve call accepts.
const MaxHostedCandidates = 200

const (
	// ReasonTooManyCandidates refuses a resolve call with more than MaxHostedCandidates paths.
	ReasonTooManyCandidates = "too-many-candidates"
	// ReasonBadBase refuses a resolve call that names more than one base.
	ReasonBadBase = "bad-base"
)
