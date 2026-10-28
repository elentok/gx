package syshealth

import (
	"errors"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
)

func TestParseProcStat(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		idle, tot uint64
		wantErr   bool
	}{
		{"eight columns", "cpu  10 20 30 400 50 6 7 8\ncpu0 1 1 1 1 1 1 1 1\n", 450, 531, false},
		{"guest columns are not added", "cpu  10 20 30 400 50 6 7 8 900 1000\n", 450, 531, false},
		{"skips non-cpu lines first", "intr 5\ncpu  1 1 1 1 1 1 1 1\n", 2, 8, false},
		{"empty", "", 0, 0, true},
		{"no cpu line", "cpu0 1 1 1 1 1 1 1 1\n", 0, 0, true},
		{"too few fields", "cpu  1 2 3\n", 0, 0, true},
		{"not a number", "cpu  1 2 x 4 5 6 7 8\n", 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseProcStat([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got.CPUIdle != tc.idle || got.CPUTotal != tc.tot {
				t.Errorf("got idle %d total %d, want %d %d", got.CPUIdle, got.CPUTotal, tc.idle, tc.tot)
			}
		})
	}
}

func TestParseMeminfo(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		total, avail uint64
		wantErr      bool
	}{
		{"kB to bytes", "MemTotal:       1000 kB\nMemFree: 5 kB\nMemAvailable:    250 kB\n", 1000 * 1024, 250 * 1024, false},
		{"missing available", "MemTotal: 1000 kB\n", 0, 0, true},
		{"missing total", "MemAvailable: 1000 kB\n", 0, 0, true},
		{"not a number", "MemTotal: x kB\nMemAvailable: 1 kB\n", 0, 0, true},
		{"no value", "MemTotal:\nMemAvailable: 1 kB\n", 0, 0, true},
		{"available above total", "MemTotal: 10 kB\nMemAvailable: 20 kB\n", 0, 0, true},
		{"empty", "", 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMeminfo([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got.Total != tc.total || got.Available != tc.avail {
				t.Errorf("got %+v, want total %d avail %d", got, tc.total, tc.avail)
			}
		})
	}
}

func TestReadProcFS(t *testing.T) {
	procfs := fstest.MapFS{
		"stat":    {Data: []byte("cpu  10 20 30 400 50 6 7 8\n")},
		"meminfo": {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 250 kB\n")},
	}
	got, err := readProcFS(procfs)
	if err != nil {
		t.Fatal(err)
	}
	if got.MemPercent != 75 || got.MemUsed != 750*1024 || got.MemTotal != 1000*1024 {
		t.Errorf("memory = %v%% used %d total %d", got.MemPercent, got.MemUsed, got.MemTotal)
	}
	if got.CPUIdle != 450 || got.CPUTotal != 531 || got.TickRate != 100 || got.Wrap32 || got.Pressure != 0 {
		t.Errorf("cpu = %+v", got)
	}

	if _, err := readProcFS(fstest.MapFS{}); err == nil || errors.Is(err, ErrUnavailable) {
		t.Errorf("missing files err = %v, want a plain read error", err)
	}
}

func TestSelectBackend(t *testing.T) {
	for goos, want := range map[string]string{"linux": "linux", "darwin": "darwin", "windows": "none", "": "none"} {
		if got, _ := selectBackend(goos); got != want {
			t.Errorf("selectBackend(%q) = %q, want %q", goos, got, want)
		}
	}
	_, read := selectBackend("plan9")
	if _, err := read(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("unknown OS err = %v, want ErrUnavailable", err)
	}
}

func TestRealReader(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no backend on " + runtime.GOOS)
	}
	read := Default()
	first, err := read()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("backend unavailable on this build")
	}
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var second Reading
	for {
		if second, err = read(); err != nil {
			t.Fatal(err)
		}
		if second.CPUTotal > first.CPUTotal || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if second.CPUTotal <= first.CPUTotal {
		t.Errorf("total ticks did not rise: %d then %d", first.CPUTotal, second.CPUTotal)
	}
	if second.CPUIdle > second.CPUTotal {
		t.Errorf("idle %d above total %d", second.CPUIdle, second.CPUTotal)
	}
	if second.MemPercent < 0 || second.MemPercent > 100 {
		t.Errorf("MemPercent = %v", second.MemPercent)
	}
	if second.MemTotal == 0 || second.MemUsed > second.MemTotal {
		t.Errorf("MemUsed %d, MemTotal %d", second.MemUsed, second.MemTotal)
	}
	if second.Cores <= 0 || second.TickRate <= 0 {
		t.Errorf("cores %d, tick rate %d", second.Cores, second.TickRate)
	}
	if runtime.GOOS == "darwin" {
		switch second.Pressure {
		case PressureNormal, PressureWarn, PressureCritical:
		default:
			t.Errorf("Pressure = %d", second.Pressure)
		}
		if !second.Wrap32 {
			t.Error("macOS counters should be marked Wrap32")
		}
	}
}
