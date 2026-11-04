package cmd

import (
	"bytes"
	"context"
	"github.com/elentok/gx/server/wire"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server/servertest"
)

func TestViMode_TogglesAndSets(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()
	for _, step := range []struct {
		args []string
		want string
	}{
		{nil, "vimode on\n"},
		{nil, "vimode off\n"},
		{[]string{"on"}, "vimode on\n"},
		{[]string{"on"}, "vimode on\n"},
		{[]string{"off"}, "vimode off\n"},
	} {
		var out bytes.Buffer
		if err := runViMode(ctx, h.Client, &out, false, step.args); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != step.want {
			t.Errorf("gx vimode %v = %q, want %q", step.args, got, step.want)
		}
	}

	var out bytes.Buffer
	if err := runViMode(ctx, h.Client, &out, true, []string{"on"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"vi_mode": true`) && !strings.Contains(out.String(), `"vi_mode":true`) {
		t.Errorf("--json output = %s, want vi_mode true", out.String())
	}
}

func TestViMode_RefusesWhenNoServerRunning(t *testing.T) {
	cl := apiclient.New(filepath.Join(shortTempDir(t), "a.sock"))
	var out bytes.Buffer
	err := runViMode(context.Background(), cl, &out, false, nil)
	if err == nil || !strings.Contains(err.Error(), wire.ReasonServerNotRunning) {
		t.Errorf("err = %v; want %s", err, wire.ReasonServerNotRunning)
	}
}
