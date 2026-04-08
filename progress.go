package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// progressWriter wraps an io.Writer (stdout) and provides throttled progress reporting.
type progressWriter struct {
	out      io.Writer
	mu       sync.Mutex
	throttle time.Duration
	lastSend time.Time
}

func newProgressWriter(out io.Writer) *progressWriter {
	return &progressWriter{
		out:      out,
		throttle: 250 * time.Millisecond,
	}
}

// sendProgress sends a progress message, throttled to avoid flooding the extension.
// force=true bypasses throttling (used for final updates).
func (pw *progressWriter) sendProgress(id, phase string, percent int, bytesDownloaded int64, speed string, force bool) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	now := time.Now()
	if !force && now.Sub(pw.lastSend) < pw.throttle {
		return
	}
	pw.lastSend = now

	if percent > 100 {
		percent = 100
	}

	msg := ProgressMessage{
		Type:            "progress",
		ID:              id,
		Phase:           phase,
		Percent:         percent,
		BytesDownloaded: bytesDownloaded,
		Speed:           speed,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	writeMessage(pw.out, data)
}

func (pw *progressWriter) sendComplete(id, path string, size int64) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	msg := CompleteMessage{
		Type: "complete",
		ID:   id,
		Path: path,
		Size: size,
	}
	data, _ := json.Marshal(msg)
	writeMessage(pw.out, data)
}

func (pw *progressWriter) sendError(id, message, phase string) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	msg := ErrorMessage{
		Type:    "error",
		ID:      id,
		Message: message,
		Phase:   phase,
	}
	data, _ := json.Marshal(msg)
	writeMessage(pw.out, data)
}

// formatSpeed returns a human-readable speed string.
func formatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.0f KB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}
