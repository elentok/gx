//go:build darwin && cgo

package syshealth

/*
#include <mach/mach.h>
#include <mach/host_info.h>

static int cpuTicks(unsigned int out[CPU_STATE_MAX]) {
	host_cpu_load_info_data_t info;
	mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
	kern_return_t kr = host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)&info, &count);
	if (kr != KERN_SUCCESS) {
		return (int)kr;
	}
	for (int i = 0; i < CPU_STATE_MAX; i++) {
		out[i] = info.cpu_ticks[i];
	}
	return 0;
}
*/
import "C"

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

// darwinTickRate is sysconf(_SC_CLK_TCK) on macOS.
const darwinTickRate = 100

func readDarwin() (Reading, error) {
	var ticks [C.CPU_STATE_MAX]C.uint
	if kr := C.cpuTicks(&ticks[0]); kr != 0 {
		return Reading{}, fmt.Errorf("host_statistics: kern_return_t %d", int(kr))
	}
	var total uint64
	for _, t := range ticks {
		total += uint64(t)
	}
	level, err := unix.SysctlUint32("kern.memorystatus_level")
	if err != nil {
		return Reading{}, fmt.Errorf("kern.memorystatus_level: %w", err)
	}
	if level > 100 {
		return Reading{}, fmt.Errorf("kern.memorystatus_level: %d is not a percent", level)
	}
	memsize, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return Reading{}, fmt.Errorf("hw.memsize: %w", err)
	}
	pressure, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return Reading{}, fmt.Errorf("kern.memorystatus_vm_pressure_level: %w", err)
	}
	avail := memsize / 100 * uint64(level)
	return Reading{
		CPUIdle:    uint64(ticks[C.CPU_STATE_IDLE]),
		CPUTotal:   total,
		TickRate:   darwinTickRate,
		Wrap32:     true,
		Cores:      runtime.NumCPU(),
		MemPercent: float64(100 - level),
		MemUsed:    memsize - avail,
		MemTotal:   memsize,
		Pressure:   int(pressure),
	}, nil
}
