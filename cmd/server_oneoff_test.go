package cmd

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
)

func TestServerOneOff_ExitsEightWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out, errOut bytes.Buffer
	err := runServerOneOff(context.Background(), cl, &out, &errOut, false, server.OneOffRequest{Prompt: "x"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 8 {
		t.Fatalf("err = %v; want exit code 8", err)
	}
	if !strings.Contains(errOut.String(), "gx server start") {
		t.Errorf("stderr = %q; want the start hint", errOut.String())
	}
}
