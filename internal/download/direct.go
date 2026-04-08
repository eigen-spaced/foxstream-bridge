package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"foxstream-bridge/internal/config"
	"foxstream-bridge/internal/protocol"
)

func downloadDirect(msg protocol.IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	outPath := config.OutputPath(msg.Title, msg.StreamType)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	go func() {
		select {
		case <-cancel:
			ctxCancel()
		case <-ctx.Done():
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", msg.URL, nil)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Invalid URL: %v", err), "download")
		return
	}

	ApplyHeaders(req, msg.Headers, msg.Cookies)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			pw.sendError(msg.ID, "Download cancelled.", "download")
			return
		}
		pw.sendError(msg.ID, fmt.Sprintf("HTTP request failed: %v", err), "download")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		pw.sendError(msg.ID, "Authentication failed (HTTP 403). Cookies may be required.", "download")
		return
	}
	if resp.StatusCode != http.StatusOK {
		pw.sendError(msg.ID, fmt.Sprintf("HTTP %d", resp.StatusCode), "download")
		return
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "download")
		return
	}

	partPath := outPath + ".part"
	f, err := os.Create(partPath)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create file: %v", err), "download")
		return
	}

	totalSize := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	startTime := time.Now()

	pw.sendProgress(msg.ID, "download", 0, 0, "", true)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				f.Close()
				os.Remove(partPath)
				pw.sendError(msg.ID, fmt.Sprintf("Write error: %v", writeErr), "download")
				return
			}
			downloaded += int64(n)

			pct := 0
			if totalSize > 0 {
				pct = int(downloaded * 100 / totalSize)
			}
			elapsed := time.Since(startTime).Seconds()
			speed := ""
			if elapsed > 0 {
				speed = FormatSpeed(float64(downloaded) / elapsed)
			}
			pw.sendProgress(msg.ID, "download", pct, downloaded, speed, false)
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			f.Close()
			os.Remove(partPath)
			if ctx.Err() != nil {
				pw.sendError(msg.ID, "Download cancelled.", "download")
			} else {
				pw.sendError(msg.ID, fmt.Sprintf("Download error: %v", readErr), "download")
			}
			return
		}
	}

	f.Close()

	if err := os.Rename(partPath, outPath); err != nil {
		os.Remove(partPath)
		pw.sendError(msg.ID, fmt.Sprintf("Cannot rename file: %v", err), "download")
		return
	}

	fi, _ := os.Stat(outPath)
	finalSize := fi.Size()
	pw.sendProgress(msg.ID, "download", 100, finalSize, "", true)
	pw.sendComplete(msg.ID, outPath, finalSize)
}

// ApplyHeaders sets HTTP headers and cookies on a request.
func ApplyHeaders(req *http.Request, headers map[string]string, cookies string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
}
