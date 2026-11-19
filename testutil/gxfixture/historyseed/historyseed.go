// Package historyseed seeds the draft-history scenario's entries.
package historyseed

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// tinyImageCap is smaller than one PNG, so an entry saved under it keeps no images.
const tinyImageCap = 1

// Seed stores the entries with the real draft history store, so the server reads
// them as it reads entries a person made. pngBase64 is the image every entry uses.
func Seed(stateDir string, list []spec.HistoryEntry, pngBase64 string) error {
	png, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		return err
	}
	src, err := os.MkdirTemp("", "draft-history-images")
	if err != nil {
		return err
	}
	defer os.RemoveAll(src)

	normal, tiny := drafthistory.New(stateDir), drafthistory.NewWith(stateDir, tinyImageCap, nil)
	for n, h := range list {
		var paths []string
		for i := 0; i < h.Images; i++ {
			p := filepath.Join(src, fmt.Sprintf("%d-%d.png", n, i))
			if err := os.WriteFile(p, png, 0o644); err != nil {
				return err
			}
			paths = append(paths, p)
		}
		store := normal
		if h.DropImages {
			store = tiny
		}
		l := drafthistory.List(h.List)
		e := drafthistory.Entry{
			Text: h.Text, Project: h.Project, Slug: h.Slug, Name: h.Name, Harness: h.Harness,
			Model: h.Model, Effort: h.Effort, Folder: h.Folder, Composer: drafthistory.Composer(h.Composer),
		}
		res, err := store.Save(l, e, paths)
		if err != nil || res != drafthistory.Saved {
			return fmt.Errorf("draft history entry %q: %v (result %v)", h.Text, err, res)
		}
		if !h.DeleteImages {
			continue
		}
		// The new entry is the newest of its list.
		items, err := normal.List(l)
		if err != nil {
			return err
		}
		files, err := normal.ImagePaths(l, items[0].ID)
		if err != nil {
			return err
		}
		for _, p := range files {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}
