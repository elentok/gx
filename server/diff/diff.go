// Package diff serves the Diff page's read routes: the file list, one file's
// patch, the commit list and the blob read of a three-dot range, all over the
// git core in git/rangediff. Nothing here writes.
package diff

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketgit"
)

// Deps is what the Diff routes need from the root.
type Deps struct {
	TicketStore string
	// Epics lists a project's epics, for the canonical ticket address of a commit.
	Epics func(project string) ([]tickets.Epic, error)
}

// Diff serves the Diff routes.
type Diff struct {
	Deps
	lookups Lookups
}

// New builds the Diff read model.
func New(d Deps) *Diff { return &Diff{Deps: d} }

// Routes lists the Diff reads. The getter is called per request.
func Routes(get func() *Diff) []wire.Route {
	read := func(pattern string, do func(*Diff, http.ResponseWriter, *http.Request)) wire.Route {
		return wire.Route{
			Pattern: pattern,
			Web:     wire.WebRead,
			Handler: func(w http.ResponseWriter, r *http.Request) { do(get(), w, r) },
		}
	}
	return []wire.Route{
		read("GET /v1/diff/files", (*Diff).Files),
		read("GET /v1/diff/patch", (*Diff).Patch),
		read("GET /v1/diff/commits", (*Diff).Commits),
		read("GET /v1/diff/blob", (*Diff).Blob),
		read("GET /v1/diff/worktree", (*Diff).Worktree),
	}
}

// repoDir is the repo of the request's project. An unknown project or one
// with no repo is an error response, never another project's repo.
func (d *Diff) repoDir(w http.ResponseWriter, r *http.Request) (project, dir string, ok bool) {
	project = r.URL.Query().Get("project")
	if project == "" {
		http.Error(w, "project is required", http.StatusBadRequest)
		return "", "", false
	}
	dir = ticketgit.ProjectRepoRoot(d.TicketStore, project)
	if dir == "" {
		http.Error(w, "unknown project or project has no repo: "+project, http.StatusNotFound)
		return "", "", false
	}
	return project, dir, true
}

// stamped reports whether sha is the RangeBase or RangeTip of one of the
// project's epics: a commit that may have been pruned after the merge.
func (d *Diff) stamped(project, sha string) (bool, error) {
	epics, err := d.Epics(project)
	if err != nil {
		return false, err
	}
	for _, e := range epics {
		if sha != "" && (sha == e.RangeBase || sha == e.RangeTip) {
			return true, nil
		}
	}
	return false, nil
}

// rangeOf resolves the typed refs. unavailable means a stamped SHA names a
// commit that is not in the clone: the caller answers with the distinct
// "commits not available" result. Any other missing ref, a mistyped SHA
// included, is a 400. ok is false once an error response is written.
func (d *Diff) rangeOf(w http.ResponseWriter, project, dir string, r *http.Request) (rng rangediff.Range, unavailable, ok bool) {
	q := r.URL.Query()
	rng, err := rangediff.Resolve(dir, q.Get("first"), q.Get("second"))
	var unknown *rangediff.UnknownRefError
	switch {
	case err == nil:
		return rng, false, true
	case errors.As(err, &unknown):
		if rangediff.IsObjectID(unknown.Ref) {
			stamped, serr := d.stamped(project, unknown.Ref)
			if serr != nil {
				http.Error(w, serr.Error(), http.StatusInternalServerError)
				return rangediff.Range{}, false, false
			}
			if stamped {
				return rangediff.Range{}, true, true
			}
		}
		http.Error(w, "Unknown ref: "+unknown.Side.String(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	return rangediff.Range{}, false, false
}

// request is a resolved range read: the project's repo and its typed refs.
type request struct {
	project, dir string
	rng          rangediff.Range
	unavailable  bool
}

// resolve runs the prologue every range route shares. ok is false once an
// error response is written.
func (d *Diff) resolve(w http.ResponseWriter, r *http.Request) (req request, ok bool) {
	req.project, req.dir, ok = d.repoDir(w, r)
	if !ok {
		return request{}, false
	}
	req.rng, req.unavailable, ok = d.rangeOf(w, req.project, req.dir, r)
	return req, ok
}

func (q request) rangeWire() wire.DiffRange {
	if q.unavailable {
		return wire.DiffRange{CommitsUnavailable: true}
	}
	return wire.DiffRange{
		FirstSHA: q.rng.FirstSHA, SecondSHA: q.rng.SecondSHA, MergeBase: q.rng.MergeBase,
		NoCommonHistory: q.rng.NoCommonHistory,
	}
}

func commitFilter(r *http.Request) []string {
	var shas []string
	for s := range strings.SplitSeq(r.URL.Query().Get("commits"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			shas = append(shas, s)
		}
	}
	return shas
}

// fail writes the error response of a core call. A commit filter or blob
// request the core refused is the caller's mistake (400).
func fail(w http.ResponseWriter, err error) {
	var unknownCommit *rangediff.UnknownCommitError
	if errors.As(err, &unknownCommit) || errors.Is(err, rangediff.ErrBadBlobRequest) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func fileWire(f rangediff.File) wire.DiffFile {
	return wire.DiffFile{
		Path: f.Path, OldPath: f.OldPath, Status: f.Status, Added: f.Added, Deleted: f.Deleted,
		Large: f.Large, Binary: f.Binary, Generated: f.Generated,
	}
}

func fileList(dir string, rng rangediff.Range, shas []string) (rangediff.FileList, error) {
	if len(shas) > 0 {
		return rangediff.ListFilesIn(dir, rng, shas)
	}
	return rangediff.ListFiles(dir, rng)
}

// Files serves GET /v1/diff/files?project=&first=&second=[&commits=sha,sha].
func (d *Diff) Files(w http.ResponseWriter, r *http.Request) {
	q, ok := d.resolve(w, r)
	if !ok {
		return
	}
	out := wire.DiffFiles{DiffRange: q.rangeWire(), Files: []wire.DiffFile{}}
	if !q.unavailable {
		list, err := fileList(q.dir, q.rng, commitFilter(r))
		if err != nil {
			fail(w, err)
			return
		}
		for _, f := range list.Files {
			out.Files = append(out.Files, fileWire(f))
		}
		out.Truncated = list.Truncated
	}
	wire.WriteJSON(w, out)
}

// Patch serves GET /v1/diff/patch with Files' query plus path=.
func (d *Diff) Patch(w http.ResponseWriter, r *http.Request) {
	q, ok := d.resolve(w, r)
	if !ok {
		return
	}
	dir, rng := q.dir, q.rng
	out := wire.DiffPatch{DiffRange: q.rangeWire()}
	if q.unavailable {
		wire.WriteJSON(w, out)
		return
	}
	shas := commitFilter(r)
	list, err := fileList(dir, rng, shas)
	if err != nil {
		fail(w, err)
		return
	}
	path := r.URL.Query().Get("path")
	entry := list.Find(path)
	if entry == nil {
		http.Error(w, "path is not in the file list: "+path, http.StatusBadRequest)
		return
	}
	out.File = fileWire(*entry)
	if len(shas) > 0 {
		patches, err := rangediff.PatchesIn(dir, rng, shas, *entry)
		if err != nil {
			fail(w, err)
			return
		}
		out.CommitPatches = []wire.DiffCommitPatch{}
		for _, p := range patches {
			out.CommitPatches = append(out.CommitPatches, wire.DiffCommitPatch(p))
		}
	} else if out.Patch, err = rangediff.Patch(dir, rng, *entry); err != nil {
		fail(w, err)
		return
	}
	wire.WriteJSON(w, out)
}

// Commits serves GET /v1/diff/commits?project=&first=&second=.
func (d *Diff) Commits(w http.ResponseWriter, r *http.Request) {
	q, ok := d.resolve(w, r)
	if !ok {
		return
	}
	out := wire.DiffCommits{DiffRange: q.rangeWire(), Commits: []wire.DiffCommit{}}
	if !q.unavailable {
		epics, err := d.Epics(q.project)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		names := make([]rangediff.EpicName, len(epics))
		for i, e := range epics {
			names[i] = rangediff.EpicName{Name: e.Name, Branch: e.Branch()}
		}
		list, err := rangediff.ListCommits(q.dir, q.rng, q.project, names)
		if err != nil {
			fail(w, err)
			return
		}
		for _, c := range list.Commits {
			files := c.Files
			if files == nil {
				files = []string{}
			}
			out.Commits = append(out.Commits, wire.DiffCommit{
				SHA: c.SHA, Subject: c.Subject, Time: c.Time.Format(time.RFC3339),
				Ticket: c.Ticket, Merge: c.Merge, Files: files,
			})
		}
		out.Truncated = list.Truncated
	}
	wire.WriteJSON(w, out)
}

// Blob serves GET /v1/diff/blob?project=&base=&tip=&path=. base and tip are the
// SHAs the file list echoed; typed refs are never accepted here.
func (d *Diff) Blob(w http.ResponseWriter, r *http.Request) {
	project, dir, ok := d.repoDir(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	base, tip := q.Get("base"), q.Get("tip")
	out := wire.DiffBlobs{MergeBase: base, Tip: tip}
	if !rangediff.IsObjectID(base) || !rangediff.IsObjectID(tip) {
		http.Error(w, "base and tip must be full commit SHAs", http.StatusBadRequest)
		return
	}
	if !rangediff.CommitsAvailable(dir, base, tip) {
		for _, sha := range []string{base, tip} {
			stamped, err := d.stamped(project, sha)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !stamped && !rangediff.CommitsAvailable(dir, sha, sha) {
				http.Error(w, "Unknown commit: "+sha, http.StatusBadRequest)
				return
			}
		}
		out.CommitsUnavailable = true
		wire.WriteJSON(w, out)
		return
	}
	pair, err := rangediff.Blobs(dir, base, tip, q.Get("path"))
	if err != nil {
		fail(w, err)
		return
	}
	out.Old, out.New = blobWire(pair.Old), blobWire(pair.New)
	wire.WriteJSON(w, out)
}

func blobWire(b rangediff.Blob) wire.DiffBlob {
	out := wire.DiffBlob{Size: b.Size}
	switch b.Kind {
	case rangediff.BlobText:
		out.Kind, out.Text = wire.DiffBlobText, b.Text
	case rangediff.BlobImage:
		out.Kind, out.ImageBase64 = wire.DiffBlobImage, base64.StdEncoding.EncodeToString(b.Image)
	case rangediff.BlobBinary:
		out.Kind = wire.DiffBlobBinary
	case rangediff.BlobTooLarge:
		out.Kind = wire.DiffBlobTooLarge
	default:
		out.Kind = wire.DiffBlobAbsent
	}
	return out
}
