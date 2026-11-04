package servertest

import (
	"os"
	"testing"
)

// ReadFile returns the text of path, failing the test when it cannot be read.
func ReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
