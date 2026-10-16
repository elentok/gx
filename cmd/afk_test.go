package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elentok/gx/server/servertest"
)

func TestAFK_TogglesAndSets(t *testing.T) {
	h := servertest.Start(t)
	ctx := context.Background()
	afk := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := runAFK(ctx, h.Client, &out, false, args); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	for _, step := range []struct {
		args []string
		want string
	}{
		{nil, "afk on\n"},
		{nil, "afk off\n"},
		{[]string{"on"}, "afk on\n"},
		{[]string{"on"}, "afk on\n"},
		{[]string{"off"}, "afk off\n"},
	} {
		if got := afk(step.args...); got != step.want {
			t.Errorf("gx afk %v = %q, want %q", step.args, got, step.want)
		}
	}

	var out bytes.Buffer
	if err := runAFK(ctx, h.Client, &out, true, []string{"on"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"afk": true`) && !strings.Contains(out.String(), `"afk":true`) {
		t.Errorf("--json output = %s, want afk true", out.String())
	}
}
