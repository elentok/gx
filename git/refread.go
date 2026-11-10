package git

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// Reading refs without forking git. A loose ref is a file holding a hash (or
// "ref: <name>" for a symbolic ref); the rest sit in packed-refs. Everything
// here reports ok=false when the repo or the name is not plainly supported
// (reftable storage, GIT_* overrides, odd names, anything unreadable), and the
// callers then ask git.

var (
	simpleRefName = regexp.MustCompile(`^[A-Za-z0-9._+/-]+$`)
	hexOnly       = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	reftableCfg   = regexp.MustCompile(`(?mi)^\s*refstorage\s*=`)
)

const maxSymrefDepth = 5

// validRefName accepts the plain names git itself accepts and rejects the rest.
func validRefName(name string) bool {
	if !simpleRefName.MatchString(name) || strings.Contains(name, "..") ||
		strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") || strings.HasSuffix(part, ".") {
			return false
		}
	}
	return true
}

// refStore is a repo's ref files.
type refStore struct{ gitDir, commonDir string }

func openRefStore(dir string) (refStore, bool) {
	l, ok := discoverGit(dir)
	if !ok {
		return refStore{}, false
	}
	b, err := os.ReadFile(filepath.Join(l.commonDir, "config"))
	if err != nil && !os.IsNotExist(err) {
		return refStore{}, false
	}
	if reftableCfg.Match(b) {
		return refStore{}, false
	}
	return refStore{l.gitDir, l.commonDir}, true
}

// read resolves a full ref name ("HEAD" or "refs/...") to a hash. found=false
// with ok=true means the ref definitely does not exist.
func (s refStore) read(name string, depth int) (hash string, found, ok bool) {
	if depth > maxSymrefDepth {
		return "", false, false
	}
	var base string
	switch {
	case name == "HEAD":
		base = s.gitDir
	case strings.HasPrefix(name, "refs/") && validRefName(name) &&
		!strings.HasPrefix(name, "refs/worktree/") && !strings.HasPrefix(name, "refs/bisect/") &&
		!strings.HasPrefix(name, "refs/rewritten/"):
		base = s.commonDir
	default:
		return "", false, false
	}

	// Loose first, packed second: pack-refs writes packed-refs before it
	// deletes the loose file, so this order never misses a ref mid-pack.
	b, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(name)))
	switch {
	case err == nil:
		content := strings.TrimSpace(string(b))
		if target, isSym := strings.CutPrefix(content, "ref:"); isSym {
			return s.read(strings.TrimSpace(target), depth+1)
		}
		if isHash(content) {
			return content, true, true
		}
		return "", false, false
	case os.IsNotExist(err) || isDirErr(base, name):
		// fall through to packed-refs
	default:
		return "", false, false
	}
	if name == "HEAD" {
		return "", false, true
	}
	return s.packed(name)
}

func isDirErr(base, name string) bool {
	fi, err := os.Stat(filepath.Join(base, filepath.FromSlash(name)))
	return err == nil && fi.IsDir()
}

func isHash(s string) bool { return (len(s) == 40 || len(s) == 64) && hexOnly.MatchString(s) }

func (s refStore) packed(name string) (hash string, found, ok bool) {
	f, err := os.Open(filepath.Join(s.commonDir, "packed-refs"))
	if os.IsNotExist(err) {
		return "", false, true
	}
	if err != nil {
		return "", false, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		h, ref, cut := strings.Cut(line, " ")
		if cut && ref == name && isHash(h) {
			return h, true, true
		}
	}
	if sc.Err() != nil {
		return "", false, false
	}
	return "", false, true
}

// resolveRev resolves HEAD, a full ref name, or a plain short name the way
// git's name lookup does. found=false with ok=true means no such ref; a short
// name that matches nothing is never ok, since it might be an abbreviated hash
// or another revision syntax.
func resolveRev(dir, rev string) (hash string, found, ok bool) {
	full := rev == "HEAD" || strings.HasPrefix(rev, "refs/")
	if !full && (!validRefName(rev) || (len(rev) >= 4 && hexOnly.MatchString(rev))) {
		return "", false, false
	}
	s, ok := openRefStore(dir)
	if !ok {
		return "", false, false
	}
	if full {
		return s.read(rev, 0)
	}
	for _, prefix := range []string{"refs/", "refs/tags/", "refs/heads/", "refs/remotes/"} {
		if h, found, ok := s.read(prefix+rev, 0); !ok {
			return "", false, false
		} else if found {
			return h, true, true
		}
	}
	if h, found, ok := s.read("refs/remotes/"+rev+"/HEAD", 0); ok && found {
		return h, true, true
	}
	return "", false, false
}

// revVerify is `git rev-parse --verify rev`, or "" when rev does not resolve.
func revVerify(dir, rev string) string {
	if h, found, ok := resolveRev(dir, rev); ok {
		if found {
			return h
		}
		return ""
	}
	return run.AllowFail(dir, []string{"rev-parse", "--verify", rev})
}

// currentBranchFast is `git rev-parse --abbrev-ref HEAD` for an attached HEAD
// on an existing branch, or detached HEAD ("HEAD").
func currentBranchFast(dir string) (string, bool) {
	s, ok := openRefStore(dir)
	if !ok {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(s.gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	content := strings.TrimSpace(string(b))
	target, isSym := strings.CutPrefix(content, "ref:")
	if !isSym {
		if isHash(content) {
			return "HEAD", true
		}
		return "", false
	}
	target = strings.TrimSpace(target)
	name, isBranch := strings.CutPrefix(target, "refs/heads/")
	if !isBranch || !validRefName(target) {
		return "", false
	}
	// An unborn branch makes git fail, and a tag of the same name makes it
	// print "heads/<name>"; leave both to git.
	if _, found, ok := s.read(target, 0); !ok || !found {
		return "", false
	}
	if _, tagFound, tagOK := s.read("refs/tags/"+name, 0); !tagOK || tagFound {
		return "", false
	}
	return name, true
}
