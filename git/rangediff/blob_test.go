package rangediff_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/git/rangediff"
	"github.com/elentok/gx/testutil"
)

// blobRepo makes a repo whose range main..feat adds, edits, renames and deletes files.
func blobRepo(t *testing.T, base, tip func(dir string)) (string, rangediff.Range) {
	t.Helper()
	dir := testutil.TempRepo(t)
	base(dir)
	testutil.CommitAll(t, dir, "base")
	mustGit(t, dir, "branch", "feat")
	mustGit(t, dir, "checkout", "feat")
	tip(dir)
	testutil.CommitAll(t, dir, "tip")
	return dir, mustResolve(t, dir, "main", "feat")
}

func blobs(t *testing.T, dir string, r rangediff.Range, p string) (rangediff.BlobPair, error) {
	t.Helper()
	return rangediff.Blobs(dir, r.MergeBase, r.SecondSHA, p)
}

func TestBlobRefusesBadRequests(t *testing.T) {
	t.Parallel()
	dir, r := blobRepo(t,
		func(d string) { testutil.WriteFile(t, d, "outside.txt", "o\n") },
		func(d string) { testutil.WriteFile(t, d, "in.txt", "i\n") })

	for _, p := range []string{"outside.txt", "/etc/passwd", "../x", "a/../in.txt", "-in.txt", "in.txt\x00", ""} {
		if _, err := blobs(t, dir, r, p); !errors.Is(err, rangediff.ErrBadBlobRequest) {
			t.Errorf("path %q: err = %v, want refusal", p, err)
		}
	}
	for _, sha := range []string{"feat", "main", r.SecondSHA[:39], ""} {
		if _, err := rangediff.Blobs(dir, r.MergeBase, sha, "in.txt"); !errors.Is(err, rangediff.ErrBadBlobRequest) {
			t.Errorf("tip %q: err = %v, want refusal", sha, err)
		}
	}
	if _, err := blobs(t, dir, r, "in.txt"); err != nil {
		t.Fatalf("valid request refused: %v", err)
	}
}

func TestBlobSymlinkIsNotFollowedAndDiskIsNotRead(t *testing.T) {
	t.Parallel()
	dir, r := blobRepo(t,
		func(d string) { testutil.WriteFile(t, d, "target.txt", "secret\n") },
		func(d string) {
			testutil.WriteFile(t, d, "dirty.txt", "committed\n")
			if err := os.Symlink("target.txt", filepath.Join(d, "link")); err != nil {
				t.Fatal(err)
			}
		})
	testutil.WriteFile(t, dir, "dirty.txt", "dirty\n")

	pair, err := blobs(t, dir, r, "link")
	if err != nil {
		t.Fatal(err)
	}
	if pair.New.Kind != rangediff.BlobText || pair.New.Text != "target.txt" {
		t.Errorf("symlink = %+v, want target text", pair.New)
	}
	pair, err = blobs(t, dir, r, "dirty.txt")
	if err != nil {
		t.Fatal(err)
	}
	if pair.New.Text != "committed\n" {
		t.Errorf("dirty file read as %q, want committed content", pair.New.Text)
	}
}

func TestBlobSizeAndTypeRules(t *testing.T) {
	t.Parallel()
	small := append([]byte("\x89PNG\x00"), bytes.Repeat([]byte{1}, 100)...)
	big := bytes.Repeat([]byte{1, 0}, rangediff.MaxBlobBytes/2+1)
	dir, r := blobRepo(t,
		func(d string) { testutil.WriteFile(t, d, "keep.txt", "k\n") },
		func(d string) {
			testutil.WriteFile(t, d, "a.PNG", string(small))
			testutil.WriteFile(t, d, "big.png", string(big))
			testutil.WriteFile(t, d, "v.svg", "<svg/>\n")
			testutil.WriteFile(t, d, "x.bin", "a\x00b")
			testutil.WriteFile(t, d, "huge.txt", string(bytes.Repeat([]byte("a"), rangediff.MaxBlobBytes+1)))
			testutil.WriteFile(t, d, "ok.txt", "hi\n")
		})

	cases := []struct {
		path  string
		kind  rangediff.BlobKind
		image bool
	}{
		{"a.PNG", rangediff.BlobImage, true},
		{"big.png", rangediff.BlobBinary, false},
		{"v.svg", rangediff.BlobBinary, false},
		{"x.bin", rangediff.BlobBinary, false},
		{"huge.txt", rangediff.BlobTooLarge, false},
		{"ok.txt", rangediff.BlobText, false},
	}
	for _, tc := range cases {
		pair, err := blobs(t, dir, r, tc.path)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		n := pair.New
		if n.Kind != tc.kind || (len(n.Image) > 0) != tc.image || n.Size == 0 {
			t.Errorf("%s: kind=%v image=%d size=%d, want kind %v", tc.path, n.Kind, len(n.Image), n.Size, tc.kind)
		}
		if tc.kind != rangediff.BlobText && n.Text != "" {
			t.Errorf("%s: flagged blob carries text", tc.path)
		}
		if pair.Old.Kind != rangediff.BlobAbsent {
			t.Errorf("%s: old side = %v, want absent", tc.path, pair.Old.Kind)
		}
	}
}

func TestBlobRenameAndDelete(t *testing.T) {
	t.Parallel()
	body := "line one\nline two\nline three\nline four\n"
	dir, r := blobRepo(t,
		func(d string) {
			testutil.WriteFile(t, d, "old.txt", body)
			testutil.WriteFile(t, d, "gone.txt", "bye\n")
		},
		func(d string) {
			if err := os.Remove(filepath.Join(d, "old.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(d, "gone.txt")); err != nil {
				t.Fatal(err)
			}
			testutil.WriteFile(t, d, "new.txt", body+"five\n")
		})

	pair, err := blobs(t, dir, r, "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if pair.Old.Text != body || pair.New.Text != body+"five\n" {
		t.Errorf("rename pair = %q / %q", pair.Old.Text, pair.New.Text)
	}
	pair, err = blobs(t, dir, r, "gone.txt")
	if err != nil {
		t.Fatal(err)
	}
	if pair.Old.Text != "bye\n" || pair.New.Kind != rangediff.BlobAbsent {
		t.Errorf("delete pair = %+v", pair)
	}
}
