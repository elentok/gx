package config

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
)

// ByteSize is a size in bytes. It reads and writes the short SI form
// ("20G" is 20×10^9 bytes); zero means off.
type ByteSize uint64

const (
	defaultDiskWarnBelow  = ByteSize(20 * humanize.GByte)
	defaultDiskCleanBelow = ByteSize(10 * humanize.GByte)
)

// ParseByteSize parses a size such as "20G", "500MB" or "0". Binary units
// ("20GiB") are accepted too; a bare "G" is decimal.
func ParseByteSize(s string) (ByteSize, error) {
	n, err := humanize.ParseBytes(s)
	if err != nil {
		return 0, err
	}
	return ByteSize(n), nil
}

// String prints the largest decimal unit that divides b evenly, so the result
// parses back to b.
func (b ByteSize) String() string {
	for _, u := range []struct {
		size   uint64
		suffix string
	}{{humanize.GByte, "G"}, {humanize.MByte, "M"}, {humanize.KByte, "K"}} {
		if b != 0 && uint64(b)%u.size == 0 {
			return strconv.FormatUint(uint64(b)/u.size, 10) + u.suffix
		}
	}
	return strconv.FormatUint(uint64(b), 10)
}

// MarshalJSON writes the size string, so a written config loads back.
func (b ByteSize) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// DiskSpaceConfig controls when gx warns about low free disk space, when it
// cleans, and the command it cleans with.
type DiskSpaceConfig struct {
	// WarnBelow and CleanBelow are free-space thresholds; zero turns one off.
	WarnBelow  ByteSize `json:"warn-below"`
	CleanBelow ByteSize `json:"clean-below"`
	// CleanupCommand is an argv list, not a shell string. Empty means no cleanup.
	CleanupCommand []string `json:"cleanup-command"`
}

// DefaultDiskSpaceConfig returns the disk-space defaults.
func DefaultDiskSpaceConfig() DiskSpaceConfig {
	return DiskSpaceConfig{
		WarnBelow:      defaultDiskWarnBelow,
		CleanBelow:     defaultDiskCleanBelow,
		CleanupCommand: []string{"go", "clean", "-cache"},
	}
}

// clampDiskSpace raises a non-zero warn threshold to the clean threshold when
// it is below it, so a warning always comes before a clean.
func clampDiskSpace(cfg DiskSpaceConfig) DiskSpaceConfig {
	if cfg.WarnBelow != 0 && cfg.CleanBelow > cfg.WarnBelow {
		cfg.WarnBelow = cfg.CleanBelow
	}
	return cfg
}

// applyDiskSpace overlays the explicitly-set keys onto cfg and clamps. A bad
// size string is an error naming its key.
func applyDiskSpace(cfg DiskSpaceConfig, warn, clean *string, command []string) (DiskSpaceConfig, error) {
	for _, f := range []struct {
		key string
		in  *string
		out *ByteSize
	}{{"disk-space.warn-below", warn, &cfg.WarnBelow}, {"disk-space.clean-below", clean, &cfg.CleanBelow}} {
		if f.in == nil {
			continue
		}
		n, err := ParseByteSize(*f.in)
		if err != nil {
			return cfg, fmt.Errorf("%s: %w", f.key, err)
		}
		*f.out = n
	}
	if command != nil {
		cfg.CleanupCommand = command
	}
	return clampDiskSpace(cfg), nil
}
