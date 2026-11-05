package cmdbase

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/wire"
)

func decodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output %q is not a JSON object: %v", b, err)
	}
	return m
}

// Read results that are JSON objects carry via/actor like writes do; a JSON
// array keeps its shape, since stamping it would mean wrapping it.
func TestEncodeProvenance_StampsObjectsLeavesArrays(t *testing.T) {
	t.Parallel()
	var obj bytes.Buffer
	if err := EncodeProvenance(&obj, wire.Explanation{}, ViaServer, ActorHuman); err != nil {
		t.Fatal(err)
	}
	if got := decodeJSON(t, obj.Bytes()); got["via"] != "server" || got["actor"] != "human" {
		t.Errorf("object via/actor = %v/%v; want server/human", got["via"], got["actor"])
	}

	var arr bytes.Buffer
	if err := EncodeProvenance(&arr, []wire.LockInfo{{}}, ViaServer, ActorHuman); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if err := json.Unmarshal(arr.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("array output %q: %v", arr.Bytes(), err)
	}
	if _, stamped := list[0]["via"]; stamped {
		t.Errorf("array element stamped: %v", list[0])
	}
}

func TestEnv_DirsAndDialFallBackToRealClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  []string
	}{
		{"nil factory, home only", []string{"HOME=" + t.TempDir()}},
		{"nil factory, state dir set", []string{"HOME=" + t.TempDir(), "XDG_STATE_HOME=" + t.TempDir()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := Env{Env: tt.env}
			got, gotErr := e.Dirs().StateDir()
			want, wantErr := config.DirsFrom(tt.env).StateDir()
			if got != want || (gotErr == nil) != (wantErr == nil) {
				t.Errorf("Dirs().StateDir() = %q, %v; want %q, %v", got, gotErr, want, wantErr)
			}
			c, err := e.Dial()()
			if err != nil || c == nil {
				t.Fatalf("Dial()() = %v, %v; want a real client", c, err)
			}
		})
	}
}

func TestEnv_DialUsesInjectedFactory(t *testing.T) {
	t.Parallel()
	want := apiclient.New("injected.sock")
	e := Env{NewClient: func() (*apiclient.Client, error) { return want, nil }}
	got, err := e.Dial()()
	if err != nil || got != want {
		t.Errorf("Dial()() = %v, %v; want the injected client", got, err)
	}
}
