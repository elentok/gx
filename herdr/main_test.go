package herdr

import (
	"os"
	"testing"

	"github.com/elentok/gx/testutil/herdrfake"
)

// TestMain lets `go test` binaries in this package double as the fake herdr
// helper executable when re-invoked by herdrfake.Start (see smoke_test.go).
func TestMain(m *testing.M) {
	herdrfake.RunHelperProcess()
	os.Exit(m.Run())
}
