package config

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParse_DiskSpace(t *testing.T) {
	goClean := []string{"go", "clean", "-cache"}
	cases := map[string]struct {
		body           string
		warn, clean    ByteSize
		command        []string
		wantErrContain string
	}{
		"empty section is the defaults": {body: `{}`, warn: 20e9, clean: 10e9, command: goClean},
		"size strings parse":            {body: `{"disk-space":{"warn-below":"50G","clean-below":"500MB"}}`, warn: 50e9, clean: 500e6, command: goClean},
		"binary units parse":            {body: `{"disk-space":{"warn-below":"20GiB","clean-below":"1GiB"}}`, warn: 20 << 30, clean: 1 << 30, command: goClean},
		"zero turns a threshold off":    {body: `{"disk-space":{"warn-below":"0","clean-below":"0G"}}`, command: goClean},
		"clean above warn raises warn":  {body: `{"disk-space":{"warn-below":"10G","clean-below":"15G"}}`, warn: 15e9, clean: 15e9, command: goClean},
		"clean above zero warn":         {body: `{"disk-space":{"warn-below":"0","clean-below":"15G"}}`, warn: 0, clean: 15e9, command: goClean},
		"command replaces default":      {body: `{"disk-space":{"cleanup-command":["make","clean"]}}`, warn: 20e9, clean: 10e9, command: []string{"make", "clean"}},
		"empty command turns it off":    {body: `{"disk-space":{"cleanup-command":[]}}`, warn: 20e9, clean: 10e9, command: []string{}},
		"bad warn names the key":        {body: `{"disk-space":{"warn-below":"lots"}}`, wantErrContain: "disk-space.warn-below"},
		"bad clean names the key":       {body: `{"disk-space":{"clean-below":"1XB"}}`, wantErrContain: "disk-space.clean-below"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := Parse([]byte(tc.body))
			if tc.wantErrContain != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContain) {
					t.Fatalf("err = %v, want it to name %q", err, tc.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			d := cfg.DiskSpace
			if d.WarnBelow != tc.warn || d.CleanBelow != tc.clean || !slices.Equal(d.CleanupCommand, tc.command) || (d.CleanupCommand == nil) != (tc.command == nil) {
				t.Errorf("disk-space = %+v, want warn %d clean %d command %v", d, tc.warn, tc.clean, tc.command)
			}
		})
	}
}

func TestDefaultDiskSpaceMarshalsAsSizeStrings(t *testing.T) {
	data, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"warn-below":"20G","clean-below":"10G"`) {
		t.Errorf("marshaled config = %s, want 20G and 10G size strings", data)
	}
	back, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !reflect.DeepEqual(back, Default()) {
		t.Errorf("round trip = %+v, want Default()", back)
	}
}

func TestByteSize_StringRoundTrips(t *testing.T) {
	for in, want := range map[ByteSize]string{0: "0", 20e9: "20G", 500e6: "500M", 1500e6: "1500M", 7e3: "7K", 1234: "1234"} {
		if got := in.String(); got != want {
			t.Errorf("ByteSize(%d).String() = %q, want %q", in, got, want)
		}
		if back, err := ParseByteSize(in.String()); err != nil || back != in {
			t.Errorf("ParseByteSize(%q) = %d, %v, want %d", in.String(), back, err, in)
		}
	}
}

func TestWithProject_DiskSpaceCleanupCommand(t *testing.T) {
	cases := map[string]struct {
		body string
		want []string
	}{
		"absent key keeps the global command": {`{}`, []string{"go", "clean", "-cache"}},
		"absent sub-key keeps it too":         {`{"disk-space":{}}`, []string{"go", "clean", "-cache"}},
		"a list replaces the command":         {`{"disk-space":{"cleanup-command":["make","clean"]}}`, []string{"make", "clean"}},
		"an empty list turns cleanup off":     {`{"disk-space":{"cleanup-command":[]}}`, []string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pf, err := ParseProjectFile([]byte(tc.body))
			if err != nil {
				t.Fatalf("ParseProjectFile: %v", err)
			}
			global := Default()
			got := global.WithProject(pf).DiskSpace.CleanupCommand
			if !slices.Equal(got, tc.want) || len(got) != len(tc.want) {
				t.Errorf("command = %v, want %v", got, tc.want)
			}
			if !slices.Equal(global.DiskSpace.CleanupCommand, []string{"go", "clean", "-cache"}) {
				t.Errorf("global command = %v, want it untouched", global.DiskSpace.CleanupCommand)
			}
		})
	}
}

func TestParseProjectFile_DiskSpaceRejectsBadKeys(t *testing.T) {
	if _, err := ParseProjectFile([]byte(`{"disk-space":{"warn-below":"1G"}}`)); err == nil || !strings.Contains(err.Error(), "disk-space.warn-below") {
		t.Errorf("unknown sub-key: err = %v, want it to name disk-space.warn-below", err)
	}
	if _, err := ParseProjectFile([]byte(`{"disk-space":{"cleanup-command":"go clean"}}`)); err == nil {
		t.Error("string command: want a type error")
	}
}
