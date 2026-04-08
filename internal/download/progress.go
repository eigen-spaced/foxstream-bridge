package download

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"foxstream-bridge/internal/protocol"
)

type progressWriter struct {
	mu   sync.Mutex
	out  io.Writer
	last map[string]time.Time
}

func newProgressWriter(out io.Writer) *progressWriter {
	return &progressWriter{
		out:  out,
		last: make(map[string]time.Time),
	}
}

func (pw *progressWriter) sendProgress(id, phase string, percent int, bytesDownloaded int64, speed string, force bool) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if !force {
		if t, ok := pw.last[id]; ok && time.Since(t) < 250*time.Millisecond {
			return
		}
	}
	pw.last[id] = time.Now()

	msg := protocol.ProgressMessage{
		Type:            "progress",
		ID:              id,
		Phase:           phase,
		Percent:         percent,
		BytesDownloaded: bytesDownloaded,
		Speed:           speed,
	}
	data, _ := json.Marshal(msg)
	protocol.WriteMessage(pw.out, data)
}

func (pw *progressWriter) sendComplete(id, path string, size int64) {
	msg := protocol.CompleteMessage{
		Type: "complete",
		ID:   id,
		Path: path,
		Size: size,
	}
	data, _ := json.Marshal(msg)
	pw.mu.Lock()
	defer pw.mu.Unlock()
	protocol.WriteMessage(pw.out, data)
}

func (pw *progressWriter) sendError(id, message, phase string) {
	msg := protocol.ErrorMessage{
		Type:    "error",
		ID:      id,
		Message: message,
		Phase:   phase,
	}
	data, _ := json.Marshal(msg)
	pw.mu.Lock()
	defer pw.mu.Unlock()
	protocol.WriteMessage(pw.out, data)
}

// FormatSpeed formats bytes per second as a human-readable string.
func FormatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.0f KB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}
