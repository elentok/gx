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

// compared keeps the fields a pick compares and clears the rest, so a pick
// keeps the page's other settings. Draft lists compare text, slug and name;
// Sent messages compare text only. Images are compared by sameAsNewest.
func compared(l List, e Entry) Entry {
	out := Entry{ID: e.ID, Images: e.Images, Text: e.Text}
	if l != Sent {
		out.Slug, out.Name = e.Slug, e.Name
	}
	return out
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
	pagePaths := ImageFiles(imagesDir, pageImages)
	if s.sameAsNewest(l, compared(l, p.Entry), compared(l, page), pagePaths) {
		return Restored{Images: pageImages}, nil
	}
	// Read before saving: the save can evict this entry from a full list.
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
	if err := saveCurrent(); err != nil {
		return Restored{}, fmt.Errorf("save page to history: %w", err)
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
