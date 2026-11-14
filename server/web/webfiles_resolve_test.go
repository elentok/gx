package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/server/wire/ops"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/agentfake"
)

type resolveFixture struct {
	h      *servertest.Harness
	base   string
	cookie *http.Cookie
	repo   string // canonical path of the project's checkout
	store  string // the tickets store
}

// startResolve starts a web server over a project "proj" whose repo holds
// docs/plan.md, docs/sibling.md-style files and the denied names. A project
// "gone" points at a repo that does not exist. A non-nil uid replaces the gx
// user id the temp-owner rule compares against.
func startResolve(t *testing.T, uid *uint32) resolveFixture {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "root", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "proj", repo)
	servertest.WriteTicket(t, store, "gone", "epic-a", "01", "root", "")
	servertest.SetProjectRepoNoAutoMerge(t, store, "gone", filepath.Join(t.TempDir(), "missing-repo"))
	for name, body := range map[string]string{
		"docs/plan.md": "# plan\n", "sibling.md": "# sibling\n", ".env": "SECRET=1\n", "id_rsa": "key\n",
	} {
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := servertest.StartWithStore(t, store, servertest.WithTestHolder(), func(c *server.Config) {
		c.WebAddr = "127.0.0.1:0"
		c.HostedFilesUID = uid
	})
	hs, err := h.Client.Handshake(context.Background())
	if err != nil || hs.WebURL == "" {
		t.Fatalf("handshake web_url = %q (err %v)", hs.WebURL, err)
	}
	return resolveFixture{h: h, base: hs.WebURL, cookie: servertest.Login(t, h.StateDir, hs.WebURL), repo: repo, store: store}
}

func (f resolveFixture) resolve(t *testing.T, req ops.HostedResolveRequest) ops.HostedResolveResult {
	t.Helper()
	body, _ := json.Marshal(req)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/hosted-files/resolve", string(body), f.cookie, "", servertest.JSONPost(f.base))
	if code != http.StatusOK {
		t.Fatalf("resolve = %d %s", code, out)
	}
	var res ops.HostedResolveResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("resolve body %q: %v", out, err)
	}
	return res
}

// verdicts maps each candidate to its verdict.
func verdicts(t *testing.T, res ops.HostedResolveResult, candidates []string) map[string]ops.HostedVerdict {
	t.Helper()
	if res.Refused || len(res.Files) != len(candidates) {
		t.Fatalf("result = %+v, want %d entries in order", res, len(candidates))
	}
	out := map[string]ops.HostedVerdict{}
	for i, f := range res.Files {
		if f.Candidate != candidates[i] {
			t.Fatalf("entry %d is %q, want %q (request order)", i, f.Candidate, candidates[i])
		}
		out[f.Candidate] = f.Verdict
	}
	return out
}

func TestResolve_FoundCarriesPathSizeKindAndMtime(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	res := f.resolve(t, ops.HostedResolveRequest{Candidates: []string{filepath.Join(f.repo, "docs/plan.md")}})
	if len(res.Files) != 1 {
		t.Fatalf("result = %+v", res)
	}
	got := res.Files[0]
	if got.Verdict != ops.HostedFound || got.Path != filepath.Join(f.repo, "docs/plan.md") || got.Size != int64(len("# plan\n")) || got.Kind != ops.HostedKindMarkdown || got.Modified.IsZero() {
		t.Errorf("entry = %+v", got)
	}
}

func TestResolve_DeniedAndOutsideAreMissing(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	link := filepath.Join(f.repo, "outside-link")
	if err := os.Symlink("/etc/hosts", link); err != nil {
		t.Fatal(err)
	}
	cands := []string{
		filepath.Join(f.repo, "docs/plan.md"),
		filepath.Join(f.repo, "docs/../../../../etc/hosts"),
		link,
		filepath.Join(f.repo, ".env"),
		filepath.Join(f.repo, ".git/config"),
		filepath.Join(f.repo, "id_rsa"),
		"/etc/hosts",
		"",
	}
	got := verdicts(t, f.resolve(t, ops.HostedResolveRequest{Candidates: cands}), cands)
	for _, c := range cands {
		want := ops.HostedMissing
		if c == cands[0] {
			want = ops.HostedFound
		}
		if got[c] != want {
			t.Errorf("%q = %q, want %q", c, got[c], want)
		}
	}
}

func TestResolve_ForeignOwnedTempFileIsMissing(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := startResolve(t, nil).resolve(t, ops.HostedResolveRequest{Candidates: []string{file}})
	if len(mine.Files) != 1 || mine.Files[0].Verdict != ops.HostedFound {
		t.Fatalf("own temp file = %+v, want found", mine)
	}
	other := uint32(os.Getuid()) + 1
	theirs := startResolve(t, &other).resolve(t, ops.HostedResolveRequest{Candidates: []string{file}})
	if len(theirs.Files) != 1 || theirs.Files[0].Verdict != ops.HostedMissing {
		t.Errorf("foreign-owned temp file = %+v, want missing", theirs)
	}
}

func TestResolve_FolderBaseJoinsAndCleans(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	docs := filepath.Join(f.repo, "docs")
	res := f.resolve(t, ops.HostedResolveRequest{Candidates: []string{"../sibling.md", "plan.md", "../../../../etc/hosts"}, Dir: docs})
	got := verdicts(t, res, []string{"../sibling.md", "plan.md", "../../../../etc/hosts"})
	if got["../sibling.md"] != ops.HostedFound || got["plan.md"] != ops.HostedFound || got["../../../../etc/hosts"] != ops.HostedMissing {
		t.Errorf("verdicts = %v", got)
	}
	res = f.resolve(t, ops.HostedResolveRequest{Candidates: []string{"./docs/plan.md"}, Dir: f.repo})
	if res.Files[0].Verdict != ops.HostedFound {
		t.Errorf("./docs/plan.md = %+v", res.Files[0])
	}
}

func TestResolve_UnresolvableBaseMakesRelativeMissingNotARefusal(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	abs := filepath.Join(f.repo, "docs/plan.md")
	cands := []string{"docs/plan.md", abs}
	for name, req := range map[string]ops.HostedResolveRequest{
		"no base":         {Candidates: cands},
		"unknown session": {Candidates: cands, Session: "nope"},
		"unknown project": {Candidates: cands, Project: "nope"},
		"project no repo": {Candidates: cands, Project: "gone"},
		"folder no root":  {Candidates: cands, Dir: "/etc"},
	} {
		got := verdicts(t, f.resolve(t, req), cands)
		if got["docs/plan.md"] != ops.HostedMissing || got[abs] != ops.HostedFound {
			t.Errorf("%s: verdicts = %v, want relative missing and absolute found", name, got)
		}
	}
}

func TestResolve_ProjectBaseIsItsMainWorktree(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	res := f.resolve(t, ops.HostedResolveRequest{Candidates: []string{"docs/plan.md"}, Project: "proj"})
	if res.Files[0].Verdict != ops.HostedFound {
		t.Errorf("project base = %+v", res.Files[0])
	}
}

func TestResolve_SessionBaseIsTheSessionFolder(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	created, err := f.h.Client.Sessions.Create(context.Background(), wireSessionRequest(f.repo))
	if err != nil || created.Session == nil {
		t.Fatalf("create = %+v, %v", created, err)
	}
	res := f.resolve(t, ops.HostedResolveRequest{Candidates: []string{"docs/plan.md"}, Session: created.Session.ID})
	if res.Files[0].Verdict != ops.HostedFound {
		t.Errorf("session base = %+v", res.Files[0])
	}
}

func TestResolve_CandidateLimitAndTwoBases(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("/nope/%d.md", i)
		}
		return out
	}
	if res := f.resolve(t, ops.HostedResolveRequest{Candidates: many(200)}); res.Refused || len(res.Files) != 200 {
		t.Errorf("200 candidates: refused=%v files=%d", res.Refused, len(res.Files))
	}
	if res := f.resolve(t, ops.HostedResolveRequest{Candidates: many(201)}); res.Reason != ops.ReasonTooManyCandidates {
		t.Errorf("201 candidates = %+v, want too-many-candidates", res)
	}
	if res := f.resolve(t, ops.HostedResolveRequest{Candidates: []string{"a"}, Project: "proj", Dir: f.repo}); res.Reason != ops.ReasonBadBase {
		t.Errorf("two bases = %+v, want bad-base", res)
	}
}

func TestResolve_GxStateInsideTempRootIsMissing(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	auth := filepath.Join(f.h.StateDir, "web-auth.json")
	transcript := filepath.Join(f.h.SessionsDir, "s1", "transcript.jsonl")
	for _, p := range []string{auth, transcript} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cands := []string{auth, transcript, filepath.Join(f.h.StateDir, "web-tls", "root.pem")}
	for c, v := range verdicts(t, f.resolve(t, ops.HostedResolveRequest{Candidates: cands}), cands) {
		if v != ops.HostedMissing {
			t.Errorf("%s = %q, want missing", c, v)
		}
	}
}

func TestResolve_NeedsTheLoginCookie(t *testing.T) {
	t.Parallel()
	f := startResolve(t, nil)
	code, _ := servertest.WebPostJSON(t, f.base+"/v1/hosted-files/resolve", `{"candidates":["/etc/hosts"]}`, nil, "", servertest.JSONPost(f.base))
	if code != http.StatusUnauthorized {
		t.Errorf("no cookie = %d, want 401", code)
	}
}

func wireSessionRequest(dir string) wire.CreateSessionRequest {
	return wire.CreateSessionRequest{Project: "proj", Dir: dir, Harness: string(agentfake.HarnessKind)}
}
