package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"foxstream-bridge/internal/hls"
)

const maxWorkers = 6

// downloadSegmentsToFile downloads all segments and concatenates them into outPath.
func downloadSegmentsToFile(segments []hls.Segment, outPath string, headers map[string]string, cookies string,
	tmpDir string, cancel <-chan struct{}, onProgress func(done, total int, bytes int64, speed string)) (int64, error) {

	totalSegments := len(segments)
	segFiles := make([]string, totalSegments)
	var totalBytes atomic.Int64
	var downloadErr error
	var errOnce sync.Once
	startTime := time.Now()
	var completed atomic.Int32

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	go func() {
		select {
		case <-cancel:
			ctxCancel()
		case <-ctx.Done():
		}
	}()

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, seg := range segments {
		wg.Add(1)
		go func(idx int, s hls.Segment) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errOnce.Do(func() { downloadErr = fmt.Errorf("cancelled") })
				return
			}
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			segPath := filepath.Join(tmpDir, fmt.Sprintf("seg_%05d.ts", idx))
			n, err := downloadSegmentWithRetry(ctx, s, segPath, headers, cookies)
			if err != nil {
				errOnce.Do(func() {
					downloadErr = fmt.Errorf("segment %d: %w", idx, err)
				})
				ctxCancel()
				return
			}

			segFiles[idx] = segPath
			totalBytes.Add(n)
			done := int(completed.Add(1))

			elapsed := time.Since(startTime).Seconds()
			speed := ""
			if elapsed > 0 {
				speed = FormatSpeed(float64(totalBytes.Load()) / elapsed)
			}
			if onProgress != nil {
				onProgress(done, totalSegments, totalBytes.Load(), speed)
			}
		}(i, seg)
	}

	wg.Wait()

	if downloadErr != nil {
		return 0, downloadErr
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return 0, fmt.Errorf("create output: %w", err)
	}

	for _, segPath := range segFiles {
		sf, err := os.Open(segPath)
		if err != nil {
			outFile.Close()
			os.Remove(outPath)
			return 0, fmt.Errorf("read segment: %w", err)
		}
		io.Copy(outFile, sf)
		sf.Close()
	}

	outFile.Close()
	return totalBytes.Load(), nil
}

func downloadSegmentWithRetry(ctx context.Context, seg hls.Segment, outPath string, headers map[string]string, cookies string) (int64, error) {
	var lastErr error
	backoff := []time.Duration{0, 1 * time.Second, 2 * time.Second, 4 * time.Second}

	for attempt, delay := range backoff {
		if attempt > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return 0, fmt.Errorf("cancelled")
			}
		}

		n, err := downloadSegment(ctx, seg, outPath, headers, cookies)
		if err == nil {
			return n, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return 0, fmt.Errorf("cancelled")
		}
	}

	return 0, fmt.Errorf("failed after %d attempts: %w", len(backoff), lastErr)
}

func downloadSegment(ctx context.Context, seg hls.Segment, outPath string, headers map[string]string, cookies string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", seg.URL, nil)
	if err != nil {
		return 0, err
	}

	ApplyHeaders(req, headers, cookies)

	if seg.ByteOffset >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", seg.ByteOffset, seg.ByteOffset+seg.ByteLength-1))
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return 0, fmt.Errorf("authentication failed (HTTP 403)")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	return n, err
}

func isCancelled(cancel <-chan struct{}) bool {
	select {
	case <-cancel:
		return true
	default:
		return false
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
