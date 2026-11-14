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

// HostedURLRequest is the POST /v1/hosted-files/url body behind `gx files url`.
type HostedURLRequest struct {
	// Path is an absolute path; the client joins relative paths and "~/".
	Path string `json:"path"`
	// Host replaces the host of the web URL; it must be an allowed host.
	Host string `json:"host,omitempty"`
	// Raw asks for the raw link instead of the viewer link.
	Raw bool `json:"raw,omitempty"`
}

// HostedURLResult is the url reply: the link, or a refusal.
type HostedURLResult struct {
	URL     string `json:"url,omitempty"`
	Refused bool   `json:"refused,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Refusal and Refuse let the CLI treat the payload as a Refusable.
func (r HostedURLResult) Refusal() (bool, string, string) { return r.Refused, r.Reason, r.Message }
func (HostedURLResult) Refuse(reason, message string) HostedURLResult {
	return HostedURLResult{Refused: true, Reason: reason, Message: message}
}

// HostedRootSource says where a hosted-file root comes from. The values are
// literals because tygo cannot resolve constants from another package.
type HostedRootSource string

const (
	HostedRootBuiltIn HostedRootSource = "built-in"
	HostedRootConfig  HostedRootSource = "config"
)

// HostedRoot is one folder gx may host files from.
type HostedRoot struct {
	Path   string           `json:"path"`
	Source HostedRootSource `json:"source"`
}

// HostedRootRequest is the body of the add and remove calls.
type HostedRootRequest struct {
	// Path is an absolute folder path.
	Path string `json:"path"`
}

// HostedRootsResult is the reply of the roots list, add and remove calls: every
// root afterwards, or a refusal.
type HostedRootsResult struct {
	Roots   []HostedRoot `json:"roots,omitempty"`
	Refused bool         `json:"refused,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	Message string       `json:"message,omitempty"`
}

// Refusal and Refuse let the CLI treat the payload as a Refusable.
func (r HostedRootsResult) Refusal() (bool, string, string) { return r.Refused, r.Reason, r.Message }
func (HostedRootsResult) Refuse(reason, message string) HostedRootsResult {
	return HostedRootsResult{Refused: true, Reason: reason, Message: message}
}

// MaxHostedCandidates is how many candidates one resolve call accepts.
const MaxHostedCandidates = 200

const (
	// ReasonTooManyCandidates refuses a resolve call with more than MaxHostedCandidates paths.
	ReasonTooManyCandidates = "too-many-candidates"
	// ReasonBadBase refuses a resolve call that names more than one base.
	ReasonBadBase = "bad-base"

	// ReasonPathNotAllowed refuses a url call for any path gx will not host or cannot link.
	// One reason and one message, so the verb is no path oracle.
	ReasonPathNotAllowed = "path-not-allowed"
	// ReasonHostNotAllowed refuses a url call whose host is not an IP, localhost or an allowed host.
	ReasonHostNotAllowed = "host-not-allowed"

	// Reasons an add refuses a folder.
	ReasonRootNotAbsolute = "not-absolute"
	ReasonRootIsSlash     = "root-folder"
	ReasonRootIsHome      = "home-folder"
	ReasonRootMissing     = "missing-path"
	ReasonRootNotFolder   = "not-a-folder"
	// Reasons a remove refuses a folder.
	ReasonRootBuiltIn  = "built-in-root"
	ReasonRootNotAdded = "not-a-configured-root"
	// ReasonRootWriteFailed refuses an add or remove whose config write failed.
	ReasonRootWriteFailed = "write-failed"
)
