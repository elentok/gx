package tickets

// These exist so callers outside this package (e.g. ui/app's cross-tab
// overlay test) can drive LoopStatus() deterministically without a real
// ralphloop.Run — an export_test.go seam doesn't work here since _test.go
// files aren't visible to a normal import from another package's test
// binary.

// SetLoopStatusForTest seeds ralphLoopRegistry's state directly.
func SetLoopStatusForTest(epicName string, done, total int) {
	ralphLoopRegistry.mu.Lock()
	defer ralphLoopRegistry.mu.Unlock()
	ralphLoopRegistry.running = true
	ralphLoopRegistry.epicName = epicName
	ralphLoopRegistry.done = done
	ralphLoopRegistry.total = total
}

// ClearLoopStatusForTest resets ralphLoopRegistry back to idle.
func ClearLoopStatusForTest() {
	ralphLoopRegistry.mu.Lock()
	defer ralphLoopRegistry.mu.Unlock()
	ralphLoopRegistry.running = false
	ralphLoopRegistry.epicName = ""
	ralphLoopRegistry.done = 0
	ralphLoopRegistry.total = 0
}
