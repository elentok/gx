package server

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/syshealth"
)

const (
	// fileTicksPerRead is how many cumulative ticks one read advances; a busy
	// percent is a share of it.
	fileTicksPerRead = 10_000
	fileTickRate     = 100
	fileCores        = 8
	// fileMemTotal is the machine size when the file names no total.
	fileMemTotal = 16 * 1000 * 1000 * 1000
)

// FileSystemHealth is a Config.SystemHealth source for tests that drive the
// health watcher from outside the process. The file at path holds
// space-separated key=value pairs:
//
//	cpu=20            CPU percent busy, or "unavailable" for no CPU source
//	mem=50            memory percent used
//	total=16G         memory size (optional)
//	pressure=none     none, normal, warn or critical (optional)
//
// Cumulative ticks advance by the set percent on each read, so the watcher's
// deltas give back that percent. An empty or unreadable file is a failed read.
func FileSystemHealth(path string) syshealth.Reader {
	var mu sync.Mutex
	var idle, total uint64
	return func() (syshealth.Reading, error) {
		cpu, r, err := parseHealthFile(path)
		if err != nil {
			return syshealth.Reading{}, err
		}
		if cpu < 0 {
			return syshealth.Reading{}, syshealth.ErrUnavailable
		}
		mu.Lock()
		defer mu.Unlock()
		busy := uint64(math.Round(cpu / 100 * fileTicksPerRead))
		idle += fileTicksPerRead - busy
		total += fileTicksPerRead
		r.CPUIdle, r.CPUTotal = idle, total
		return r, nil
	}
}

// parseHealthFile returns the CPU percent (negative for unavailable) and the
// reading with its memory fields set.
func parseHealthFile(path string) (float64, syshealth.Reading, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, syshealth.Reading{}, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, syshealth.Reading{}, fmt.Errorf("%s is empty", path)
	}
	cpu := 0.0
	memTotal := config.ByteSize(fileMemTotal)
	r := syshealth.Reading{TickRate: fileTickRate, Cores: fileCores}
	for _, f := range fields {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "cpu":
			if v == "unavailable" {
				cpu = -1
			} else if cpu, err = percentValue(v); err != nil {
				return 0, r, fmt.Errorf("%s: cpu: %w", path, err)
			}
		case "mem":
			if r.MemPercent, err = percentValue(v); err != nil {
				return 0, r, fmt.Errorf("%s: mem: %w", path, err)
			}
		case "total":
			if memTotal, err = config.ParseByteSize(v); err != nil {
				return 0, r, fmt.Errorf("%s: total: %w", path, err)
			}
		case "pressure":
			switch v {
			case "none":
				r.Pressure = 0
			case "normal":
				r.Pressure = syshealth.PressureNormal
			case "warn":
				r.Pressure = syshealth.PressureWarn
			case "critical":
				r.Pressure = syshealth.PressureCritical
			default:
				return 0, r, fmt.Errorf("%s: pressure %q is not none, normal, warn or critical", path, v)
			}
		default:
			return 0, r, fmt.Errorf("%s: unknown key %q", path, k)
		}
	}
	r.MemTotal = uint64(memTotal)
	r.MemUsed = uint64(float64(r.MemTotal) * r.MemPercent / 100)
	return cpu, r, nil
}

func percentValue(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 || v > 100 {
		return 0, fmt.Errorf("%v is not a percent", v)
	}
	return v, nil
}
