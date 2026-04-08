package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// downloadDirect handles Path 1: direct file downloads (MP4, WebM, MOV).
func downloadDirect(msg IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	outPath := outputPath(msg.Title, msg.StreamType)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Cancel on signal
	go func() {
		select {
		case <-cancel:
			ctxCancel()
		case <-ctx.Done():
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", msg.URL, nil)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Invalid URL: %v", err), "downloading")
		return
	}

	applyHeaders(req, msg.Headers, msg.Cookies)

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			pw.sendError(msg.ID, "Download cancelled.", "downloading")
			return
		}
		pw.sendError(msg.ID, fmt.Sprintf("Download failed: %v", err), "downloading")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		pw.sendError(msg.ID, "Authentication failed. Try refreshing the page.", "downloading")
		return
	}
	if resp.StatusCode != http.StatusOK {
		pw.sendError(msg.ID, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status), "downloading")
		return
	}

	totalBytes := resp.ContentLength
	if totalBytes <= 0 && msg.ContentLength > 0 {
		totalBytes = msg.ContentLength
	}

	tmpPath := outPath + ".part"
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0755); err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create directory: %v", err), "downloading")
		return
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create file: %v", err), "downloading")
		return
	}

	var written int64
	startTime := time.Now()
	buf := make([]byte, 64*1024)

	for {
		// Check cancel
		select {
		case <-cancel:
			f.Close()
			os.Remove(tmpPath)
			pw.sendError(msg.ID, "Download cancelled.", "downloading")
			return
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				f.Close()
				os.Remove(tmpPath)
				pw.sendError(msg.ID, fmt.Sprintf("Write error: %v", writeErr), "downloading")
				return
			}
			written += int64(n)

			percent := 0
			if totalBytes > 0 {
				percent = int(written * 100 / totalBytes)
			}
			elapsed := time.Since(startTime).Seconds()
			speed := ""
			if elapsed > 0 {
				speed = formatSpeed(float64(written) / elapsed)
			}

			pw.sendProgress(msg.ID, "downloading", percent, written, speed, false)
		}
		if readErr != nil {
			if readErr != io.EOF {
				f.Close()
				os.Remove(tmpPath)
				if ctx.Err() != nil {
					pw.sendError(msg.ID, "Download cancelled.", "downloading")
					return
				}
				pw.sendError(msg.ID, fmt.Sprintf("Download error: %v", readErr), "downloading")
				return
			}
			break
		}
	}

	f.Close()

	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		pw.sendError(msg.ID, fmt.Sprintf("Cannot save file: %v", err), "downloading")
		return
	}

	elapsed := time.Since(startTime).Seconds()
	speed := ""
	if elapsed > 0 {
		speed = formatSpeed(float64(written) / elapsed)
	}
	pw.sendProgress(msg.ID, "downloading", 100, written, speed, true)
	pw.sendComplete(msg.ID, outPath, written)
}

func applyHeaders(req *http.Request, headers map[string]string, cookies string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
}
