package server

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeProcFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "processes")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFileProcesses_RowsAccumulateAndArgvIsLookedUp(t *testing.T) {
	path := writeProcFile(t, "busy 3000\nproc 10 1 1700000000 2500 200M go go test ./...\nhidden 11\n")
	read, argv := FileProcesses(path)

	first, err := read()
	if err != nil {
		t.Fatal(err)
	}
	second, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Processes) != 2 {
		t.Fatalf("rows = %+v, want 2", second.Processes)
	}
	p := second.Processes[0]
	if p.PID != 10 || p.PPID != 1 || p.StartUnix != 1700000000 || p.Comm != "go" || p.MemBytes != 200_000_000 {
		t.Errorf("row = %+v", p)
	}
	if d := p.CPUTicks - first.Processes[0].CPUTicks; d != 2500 {
		t.Errorf("row ticks advanced by %d, want 2500", d)
	}
	if !second.Processes[1].Unreadable || second.Processes[1].PID != 11 {
		t.Errorf("hidden row = %+v", second.Processes[1])
	}
	if d := second.Reading.CPUTotal - first.Reading.CPUTotal; d != fileTicksPerRead {
		t.Errorf("total advanced by %d, want %d", d, fileTicksPerRead)
	}
	if d := (second.Reading.CPUTotal - second.Reading.CPUIdle) - (first.Reading.CPUTotal - first.Reading.CPUIdle); d != 3000 {
		t.Errorf("busy advanced by %d, want 3000", d)
	}
	if got, want := argv(10), []string{"go", "test", "./..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %v, want %v", got, want)
	}
	if got := argv(11); got != nil {
		t.Errorf("hidden argv = %v, want none", got)
	}
}

func TestFileProcesses_BusyDefaultsToTheRowsSum(t *testing.T) {
	path := writeProcFile(t, "proc 10 1 1 100 1M a\nproc 11 1 1 200 1M b\n")
	read, _ := FileProcesses(path)
	first, _ := read()
	second, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if d := (second.Reading.CPUTotal - second.Reading.CPUIdle) - (first.Reading.CPUTotal - first.Reading.CPUIdle); d != 300 {
		t.Errorf("busy advanced by %d, want 300", d)
	}
}

func TestFileProcesses_BadFilesFailTheRead(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"no rows":       "busy 100\n",
		"unknown entry": "bogus 1\n",
		"short proc":    "proc 1 2 3\n",
		"bad pid":       "proc x 1 1 1 1M a\n",
		"bad mem":       "proc 1 1 1 1 lots a\n",
		"bad busy":      "busy x\nproc 1 1 1 1 1M a\n",
		"bad hidden":    "hidden\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			read, _ := FileProcesses(writeProcFile(t, body))
			if _, err := read(); err == nil {
				t.Fatal("read succeeded")
			}
		})
	}
	read, _ := FileProcesses(filepath.Join(t.TempDir(), "missing"))
	if _, err := read(); err == nil || strings.Contains(err.Error(), "no rows") {
		t.Errorf("missing file err = %v", err)
	}
}
