// Package nativerunner owns the herdr-free agent runners (headless and pty).
package nativerunner

// MinClaudeVersion is the oldest claude release the native runners are known
// to work with. Only `gx claude doctor` reports against it; launch-time
// preflight checks capabilities, never versions.
const MinClaudeVersion = "2.1.292"
