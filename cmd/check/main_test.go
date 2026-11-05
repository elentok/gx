package check

import (
	"testing"

	"github.com/elentok/gx/cmd/cmdtest"
)

// TestMain builds the shared gx before any test, for check_record_test.go.
func TestMain(m *testing.M) { cmdtest.MainWithGx(m) }
