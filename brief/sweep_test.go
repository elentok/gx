package brief

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSweepIndex_KeepsConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, indexName)
	past := time.Now().Add(-30 * 24 * time.Hour)
	var seed strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&seed, `{"time":%q,"tool":"old"}`+"\n", past.Format(time.RFC3339))
	}
	if err := os.WriteFile(path, []byte(seed.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	const appends = 100
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < appends; i++ {
			e := indexEntry{Time: time.Now().Format(time.RFC3339), Tool: fmt.Sprintf("new%d", i)}
			if err := appendIndex(path, e); err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 50; i++ {
		sweepIndex(dir, time.Now().Add(-Retention))
	}
	wg.Wait()
	sweepIndex(dir, time.Now().Add(-Retention))

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for i := 0; i < appends; i++ {
		if !strings.Contains(got, fmt.Sprintf(`"new%d"`, i)) {
			t.Fatalf("appended line %d lost", i)
		}
	}
	if strings.Contains(got, `"old"`) {
		t.Error("old lines survived the sweep")
	}
}
