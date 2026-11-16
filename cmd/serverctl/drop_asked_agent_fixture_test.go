//go:build gxfixture

package serverctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrapLateFinishScan_HoldsWhileFileExists(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wrap := wrapLateFinishScan(func(string) string { return hold })
	runs := 0
	scan := wrap(func() { runs++ })

	scan()
	if runs != 0 {
		t.Fatalf("scan ran %d times while held, want 0", runs)
	}
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	scan()
	if runs != 1 {
		t.Fatalf("scan ran %d times once released, want 1", runs)
	}
}

func TestWrapLateFinishScan_UnsetInstallsNoHold(t *testing.T) {
	if wrapLateFinishScan(func(string) string { return "" }) != nil {
		t.Fatal("unset variable installed a hold")
	}
}
