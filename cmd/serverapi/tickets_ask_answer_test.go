package serverapi

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAskAndAnswerCmds_ArgAndJSONFlag(t *testing.T) {
	cases := map[string]*cobra.Command{
		"ask":               NewAskCmd(Deps{}),
		"answer":            NewAnswerCmd(Deps{}, false),
		"answer serverTree": NewAnswerCmd(Deps{}, true),
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.Flags().Lookup("json") == nil {
				t.Fatal("missing --json")
			}
			if err := c.Args(c, nil); err == nil {
				t.Error("no argument accepted")
			}
			if err := c.Args(c, []string{"p:e/01"}); err != nil {
				t.Errorf("one argument refused: %v", err)
			}
			if err := c.Args(c, []string{"a", "b"}); err == nil {
				t.Error("two arguments accepted")
			}
		})
	}
	if NewAnswerCmd(Deps{}, false).Flags().Lookup("state") != nil {
		t.Error("tickets answer has --state")
	}
	if NewAnswerCmd(Deps{}, true).Flags().Lookup("state") == nil {
		t.Error("server tickets answer lacks --state")
	}
}
