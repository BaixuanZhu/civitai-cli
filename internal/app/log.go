package app

import (
	"log"
	"sync/atomic"
)

// verboseEnabled gates [INFO]/[DEBUG] output. It is set once during command
// startup (before any request goroutines exist) and read afterwards; the
// atomic keeps race-detector runs clean even if that ordering ever changes.
var verboseEnabled atomic.Bool

// SetVerbose enables or disables [INFO]/[DEBUG] logging. [WARN] and above
// are always emitted. All log output goes to stderr via the standard log
// package (configured in main); stdout is reserved for command results.
func SetVerbose(v bool) {
	verboseEnabled.Store(v)
}

// Verbose reports whether verbose logging is currently enabled.
func Verbose() bool {
	return verboseEnabled.Load()
}

// Infof logs an [INFO] message to stderr, only when verbose mode is on.
func Infof(format string, args ...any) {
	if verboseEnabled.Load() {
		log.Printf("[INFO] "+format, args...)
	}
}

// Debugf logs a [DEBUG] message to stderr, only when verbose mode is on.
func Debugf(format string, args ...any) {
	if verboseEnabled.Load() {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// Warnf logs a [WARN] message to stderr regardless of verbose mode.
// Warnings are non-fatal degradations (e.g. an unregistered baseModel filter
// the API will silently ignore).
func Warnf(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}
