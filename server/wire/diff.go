package wire

// Blob kinds of a DiffBlob.
const (
	DiffBlobAbsent   = "absent"
	DiffBlobText     = "text"
	DiffBlobImage    = "image"
	DiffBlobBinary   = "binary"
	DiffBlobTooLarge = "too_large"
)

// DiffRange is what every Diff response says about the range: the resolved
// SHAs (the Diff page sends them back and never sends typed refs again).
// CommitsUnavailable means a SHA or ref names a commit that is not in this
// clone; every other field is then empty.
type DiffRange struct {
	FirstSHA           string `json:"first_sha"`
	SecondSHA          string `json:"second_sha"`
	MergeBase          string `json:"merge_base"`
	NoCommonHistory    bool   `json:"no_common_history,omitempty"`
	CommitsUnavailable bool   `json:"commits_unavailable,omitempty"`
}

// DiffFile is one changed file. It is the entry shared by the list and patch calls.
type DiffFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"`
	Added     int    `json:"added"`
	Deleted   int    `json:"deleted"`
	Large     bool   `json:"large,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
	Generated bool   `json:"generated,omitempty"`
}

// DiffFiles answers GET /v1/diff/files.
type DiffFiles struct {
	DiffRange
	Files     []DiffFile `json:"files"`
	Truncated bool       `json:"truncated,omitempty"`
}

// DiffCommitPatch is one selected commit's patch of a file.
type DiffCommitPatch struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Patch   string `json:"patch"`
}

// DiffPatch answers GET /v1/diff/patch. Patch holds the whole-range patch;
// CommitPatches holds one patch per selected commit when the request narrowed
// by commits.
type DiffPatch struct {
	DiffRange
	File          DiffFile          `json:"file"`
	Patch         string            `json:"patch,omitempty"`
	CommitPatches []DiffCommitPatch `json:"commit_patches,omitempty"`
}

// DiffCommit is one commit of the range. Ticket is the canonical
// "<project>:<epic>/<id>" and empty when the commit has no usable trailer.
type DiffCommit struct {
	SHA     string   `json:"sha"`
	Subject string   `json:"subject"`
	Time    string   `json:"time"`
	Ticket  string   `json:"ticket,omitempty"`
	Merge   bool     `json:"merge,omitempty"`
	Files   []string `json:"files"`
}

// DiffCommits answers GET /v1/diff/commits.
type DiffCommits struct {
	DiffRange
	Commits   []DiffCommit `json:"commits"`
	Truncated bool         `json:"truncated,omitempty"`
}

// DiffBlob is one side of a file. ImageBase64 is set for Kind image only.
type DiffBlob struct {
	Kind        string `json:"kind"`
	Size        int64  `json:"size,omitempty"`
	Text        string `json:"text,omitempty"`
	ImageBase64 string `json:"image_base64,omitempty"`
}

// DiffBlobs answers GET /v1/diff/blob. The call is made by SHA: MergeBase and
// Tip are the values the file list returned.
type DiffBlobs struct {
	MergeBase          string   `json:"merge_base"`
	Tip                string   `json:"tip"`
	CommitsUnavailable bool     `json:"commits_unavailable,omitempty"`
	Old                DiffBlob `json:"old"`
	New                DiffBlob `json:"new"`
}
