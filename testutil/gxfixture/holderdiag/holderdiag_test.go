package holderdiag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/nativerunner/holder"
)

func TestHolderErrTail(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "agents", "sess-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := holderErrTail(root, "sess-1", 20); got != "" {
		t.Fatalf("no holder.err: want empty, got %q", got)
	}
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	if err := os.WriteFile(filepath.Join(dir, holder.HolderErrFile), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := holderErrTail(root, "sess-1", 20)
	if !strings.Contains(got, "line 30") || !strings.Contains(got, "line 11") {
		t.Fatalf("want last 20 lines, got %q", got)
	}
	if strings.Contains(got, "line 10\n") || strings.HasSuffix(got, "line 10") || strings.Contains(got, "line 1\n") {
		t.Fatalf("want first 10 lines dropped, got %q", got)
	}
	if got := holderErrTail(root, "other", 20); got != "" {
		t.Fatalf("other session: want empty, got %q", got)
	}
}
