package rangediff

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/elentok/gx/git/internal/run"
)

// MaxBlobBytes is the largest blob whose content Blobs returns.
const MaxBlobBytes = 2 * 1024 * 1024

// ErrBadBlobRequest is wrapped by every refusal of a blob request.
var ErrBadBlobRequest = errors.New("rangediff: bad blob request")

// BlobKind tells which outcome one side of a blob call has.
type BlobKind int

const (
	// BlobAbsent: the file does not exist on this side (a pure add or a delete).
	BlobAbsent BlobKind = iota
	// BlobText: Text holds the content.
	BlobText
	// BlobImage: Image holds the raw bytes of a png, jpg, gif or webp.
	BlobImage
	// BlobBinary: no content, only Size (other binaries, svg, images over the cap).
	BlobBinary
	// BlobTooLarge: a text blob over MaxBlobBytes; no content, only Size.
	BlobTooLarge
)

// Blob is one side of a file in the range.
type Blob struct {
	Kind  BlobKind
	Size  int64
	Text  string
	Image []byte
}

// BlobPair is the old and the new content of one file.
type BlobPair struct {
	Old, New Blob
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// Blobs reads the old (at mergeBase) and new (at tip) content of one path from
// the object store. Both SHAs must be the resolved ids the file list returned,
// and the path must be in that list.
func Blobs(dir, mergeBase, tip, p string) (BlobPair, error) {
	if !isObjectID(mergeBase) || !isObjectID(tip) {
		return BlobPair{}, fmt.Errorf("%w: not an object id", ErrBadBlobRequest)
	}
	if err := checkBlobPath(p); err != nil {
		return BlobPair{}, err
	}
	list, err := ListFiles(dir, Range{MergeBase: mergeBase, SecondSHA: tip})
	if err != nil {
		return BlobPair{}, err
	}
	var entry *File
	for i := range list.Files {
		if list.Files[i].Path == p {
			entry = &list.Files[i]
			break
		}
	}
	if entry == nil {
		return BlobPair{}, fmt.Errorf("%w: %q is not in the range", ErrBadBlobRequest, p)
	}

	oldPath := p
	if entry.OldPath != "" {
		oldPath = entry.OldPath
	}
	var pair BlobPair
	if entry.Status != "A" {
		if pair.Old, err = readBlob(dir, mergeBase, oldPath, *entry); err != nil {
			return BlobPair{}, err
		}
	}
	if entry.Status != "D" {
		if pair.New, err = readBlob(dir, tip, p, *entry); err != nil {
			return BlobPair{}, err
		}
	}
	return pair, nil
}

func isObjectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func checkBlobPath(p string) error {
	bad := p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "-") ||
		strings.ContainsRune(p, 0) || strings.ContainsRune(p, '\\')
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			bad = true
		}
	}
	if bad {
		return fmt.Errorf("%w: refused path %q", ErrBadBlobRequest, p)
	}
	return nil
}

// readBlob reads <sha>:<p> with git cat-file, size first. cat-file never
// follows a symlink and never touches the working tree.
func readBlob(dir, sha, p string, f File) (Blob, error) {
	env := run.NonInteractiveEnv()
	obj := sha + ":" + p
	sizeOut, _, err := run.Run(dir, env, "cat-file", "-s", obj)
	if err != nil {
		return Blob{}, err
	}
	size, err := strconv.ParseInt(sizeOut, 10, 64)
	if err != nil {
		return Blob{}, fmt.Errorf("rangediff: bad blob size %q", sizeOut)
	}
	b := Blob{Size: size}
	ext := strings.ToLower(path.Ext(p))

	if size > MaxBlobBytes {
		// Content is not read: a file the list calls binary, or any image, is
		// a binary flag, anything else is plain too large.
		if f.Binary || imageExts[ext] || ext == ".svg" {
			b.Kind = BlobBinary
		} else {
			b.Kind = BlobTooLarge
		}
		return b, nil
	}

	data, _, err := run.RunRaw(dir, env, "cat-file", "blob", obj)
	if err != nil {
		return Blob{}, err
	}
	switch {
	case imageExts[ext]:
		b.Kind, b.Image = BlobImage, data
	case ext == ".svg" || isBinaryContent(data):
		b.Kind = BlobBinary
	default:
		b.Kind, b.Text = BlobText, string(data)
	}
	return b, nil
}

// isBinaryContent is git's own check: a NUL in the first 8000 bytes.
func isBinaryContent(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}
