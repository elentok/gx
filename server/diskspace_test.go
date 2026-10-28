package server

import (
	"os"
	"path/filepath"
	"testing"
)

// Seam: the real source reads a non-zero free count and a stable disk id.
func TestReadFreeSpace_ReturnsFreeBytesAndStableID(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := readFreeSpace(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := readFreeSpace(sub)
	if err != nil {
		t.Fatal(err)
	}
	if a.Free == 0 || a.ID == "" || a.Name == "" {
		t.Errorf("reading = %+v, want free bytes, id and name", a)
	}
	if a.ID != b.ID {
		t.Errorf("ids differ on one disk: %q vs %q", a.ID, b.ID)
	}
}
