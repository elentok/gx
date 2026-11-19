package drafthistory

import (
	"fmt"
	"os"

	"github.com/elentok/gx/server/imagestore"
)

// Picked is an entry loaded for a restore, with the paths of its image files.
type Picked struct {
	Entry Entry
	Paths []string
}

// Load reads one entry for a restore. A missing entry is os.ErrNotExist.
func (s *Store) Load(l List, id string) (Picked, error) {
	unlock := s.lock(l)
	defer unlock()
	es, err := s.read(l)
	if err != nil {
		return Picked{}, err
	}
	for _, e := range es {
		if e.ID == id {
			return Picked{Entry: e, Paths: s.paths(l, e)}, nil
		}
	}
	return Picked{}, os.ErrNotExist
}

// MissingNotice is the notice for a restore that skipped n image files.
func MissingNotice(n int) string { return fmt.Sprintf("%d images missing", n) }

// Restored says what Restore did to the page's images.
type Restored struct {
	// Changed is false when the page already equals the entry: nothing was touched.
	Changed bool
	// Images are the draft's new image ids, in the entry's order.
	Images []string
	// Missing counts the entry's image files that could not be restored.
	Missing int
}

// Restore swaps a page's images for the entry's. The page is the draft's
// current text and settings (only what a pick changes matters) and pageImages
// are the ids in imagesDir. The caller holds the draft's lock, so the page
// cannot change in between. In order: saveCurrent saves the page to its own
// list (the list's empty and repeat rules apply there), the entry's files are
// read into memory (saving the page can evict the entry from a full list),
// the page's images are removed and the entry's are saved as the draft's own.
// A page equal to the entry changes nothing.
func (s *Store) Restore(l List, p Picked, page Entry, imagesDir string, pageImages []string, saveCurrent func() error) (Restored, error) {
	var pagePaths []string
	for _, id := range pageImages {
		if path, ok := imagestore.Find(imagesDir, id); ok {
			pagePaths = append(pagePaths, path)
		}
	}
	// Only text, slug, name and images are compared: a pick keeps the page's other settings.
	held := p.Entry
	page.Project, page.Harness, page.Model, page.Effort = held.Project, held.Harness, held.Model, held.Effort
	page.Folder, page.Composer = held.Folder, held.Composer
	if s.sameAsNewest(l, held, page, pagePaths) {
		return Restored{Images: pageImages}, nil
	}
	if err := saveCurrent(); err != nil {
		return Restored{}, fmt.Errorf("save page to history: %w", err)
	}
	var data [][]byte
	missing := 0
	for _, path := range p.Paths {
		b, err := os.ReadFile(path)
		if err != nil {
			missing++
			continue
		}
		data = append(data, b)
	}
	for _, id := range pageImages {
		if _, _, err := imagestore.Remove(imagesDir, id); err != nil {
			return Restored{}, err
		}
	}
	ids := []string{}
	for _, b := range data {
		res, err := imagestore.Save(imagesDir, b)
		if err != nil {
			return Restored{}, err
		}
		if res.Refused {
			missing++
			continue
		}
		ids = append(ids, res.ID)
	}
	return Restored{Changed: true, Images: ids, Missing: missing}, nil
}
