package agentlog_test

import (
	"bufio"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/agentlog"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestRender_Golden(t *testing.T) {
	for _, name := range []string{"out", "transcript"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", name+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var got []string
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				got = append(got, agentlog.Render(sc.Bytes())...)
			}
			if err := sc.Err(); err != nil {
				t.Fatal(err)
			}
			text := strings.Join(got, "\n") + "\n"

			golden := filepath.Join("testdata", name+".golden")
			if *update {
				if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if text != string(want) {
				t.Errorf("render mismatch (run with -update to accept)\ngot:\n%s\nwant:\n%s", text, want)
			}
		})
	}
}
