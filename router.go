package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

const version = "2.0.0"

// downloadTracker manages active downloads and their cancel channels.
type downloadTracker struct {
	mu      sync.Mutex
	cancels map[string]chan struct{}
}

func newDownloadTracker() *downloadTracker {
	return &downloadTracker{
		cancels: make(map[string]chan struct{}),
	}
}

func (dt *downloadTracker) register(id string) chan struct{} {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	ch := make(chan struct{})
	dt.cancels[id] = ch
	return ch
}

func (dt *downloadTracker) cancel(id string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if ch, ok := dt.cancels[id]; ok {
		close(ch)
		delete(dt.cancels, id)
	}
}

func (dt *downloadTracker) done(id string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	delete(dt.cancels, id)
}

// router reads messages from the extension and dispatches by action type.
func router(in io.Reader, out io.Writer) {
	pw := newProgressWriter(out)
	dt := newDownloadTracker()

	for {
		raw, err := readMessage(in)
		if err != nil {
			return
		}

		var msg IncomingMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			sendJSON(out, ErrorMessage{
				Type:    "error",
				Message: fmt.Sprintf("Invalid JSON: %v", err),
			})
			continue
		}

		switch msg.Action {
		case "ping":
			info, hasFF := hasFFmpeg()
			pong := PongMessage{Type: "pong", Version: version}
			if hasFF {
				pong.FFmpeg = info
			}
			sendJSON(out, pong)

		case "download":
			cancel := dt.register(msg.ID)
			go func() {
				defer dt.done(msg.ID)
				handleDownload(msg, pw, cancel)
			}()

		case "cancel":
			dt.cancel(msg.ID)

		default:
			sendJSON(out, ErrorMessage{
				Type:    "error",
				ID:      msg.ID,
				Message: fmt.Sprintf("Unknown action: %s", msg.Action),
			})
		}
	}
}

func sendJSON(w io.Writer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	writeMessage(w, data)
}

// handleDownload dispatches to the correct download path based on stream type.
func handleDownload(msg IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	switch msg.StreamType {
	case "mp4", "webm", "mov":
		downloadDirect(msg, pw, cancel)
	case "hls":
		downloadHLS(msg, pw, cancel)
	default:
		pw.sendError(msg.ID, fmt.Sprintf("Unsupported stream type: %s", msg.StreamType), "")
	}
}
