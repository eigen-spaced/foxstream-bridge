package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxWorkers = 6

// downloadHLS handles Path 2 (muxed) and Path 3 (demuxed).
func downloadHLS(msg IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	if msg.StreamStructure == "demuxed" {
		downloadHLSDemuxed(msg, pw, cancel)
		return
	}
	downloadHLSMuxed(msg, pw, cancel)
}

// downloadHLSMuxed handles Path 2: muxed HLS streams.
func downloadHLSMuxed(msg IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	outPath := outputPath(msg.Title, "mp4")

	playlistURL := msg.URL
	if msg.SelectedQuality != "" || len(msg.Qualities) > 0 {
		resolved, err := findSelectedVariantURL(msg.URL, msg.SelectedQuality, msg.Qualities, msg.Headers, msg.Cookies)
		if err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Failed to resolve quality: %v", err), "video-segments")
			return
		}
		playlistURL = resolved
	}

	segments, err := fetchAndParseMediaPlaylist(playlistURL, msg.Headers, msg.Cookies)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Failed to parse playlist: %v", err), "video-segments")
		return
	}

	pw.sendProgress(msg.ID, "video-segments", 0, 0, "", true)

	tmpDir, err := os.MkdirTemp("", "foxstream-*")
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create temp dir: %v", err), "video-segments")
		return
	}
	defer os.RemoveAll(tmpDir)

	concatPath := filepath.Join(tmpDir, "concat.ts")
	totalBytes, err := downloadSegmentsToFile(segments, concatPath, msg.Headers, msg.Cookies, tmpDir, cancel,
		func(done, total int, bytes int64, speed string) {
			pct := done * 90 / total
			pw.sendProgress(msg.ID, "video-segments", pct, bytes, speed, false)
		})
	if err != nil {
		if isCancelled(cancel) {
			pw.sendError(msg.ID, "Download cancelled.", "video-segments")
		} else {
			pw.sendError(msg.ID, fmt.Sprintf("Segment download failed: %v", err), "video-segments")
		}
		return
	}

	pw.sendProgress(msg.ID, "video-segments", 90, totalBytes, "", true)

	// Remux to .mp4 if ffmpeg is available, otherwise keep .ts
	ffmpegPath := findFFmpeg()
	if ffmpegPath != "" {
		pw.sendProgress(msg.ID, "muxing", 90, totalBytes, "", true)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "muxing")
			return
		}
		err := remuxToMP4(ffmpegPath, concatPath, outPath, msg.DurationSeconds, func(pct int) {
			muxPct := 90 + pct/10
			pw.sendProgress(msg.ID, "muxing", muxPct, totalBytes, "", false)
		})
		if err != nil {
			if isCancelled(cancel) {
				os.Remove(outPath)
				pw.sendError(msg.ID, "Download cancelled.", "muxing")
				return
			}
			pw.sendError(msg.ID, fmt.Sprintf("Remux failed: %v", err), "muxing")
			return
		}
	} else {
		// No ffmpeg — just rename the .ts file. Change extension to .ts.
		outPath = outputPath(msg.Title, "ts")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "video-segments")
			return
		}
		if err := copyFile(concatPath, outPath); err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Cannot save file: %v", err), "video-segments")
			return
		}
	}

	fi, _ := os.Stat(outPath)
	finalSize := fi.Size()
	pw.sendProgress(msg.ID, "video-segments", 100, finalSize, "", true)
	pw.sendComplete(msg.ID, outPath, finalSize)
}

// downloadHLSDemuxed handles Path 3: demuxed HLS (separate video + audio).
// Uses ffmpeg to download and mux directly from playlist URLs, which correctly
// handles both MPEG-TS and fMP4 segment formats.
func downloadHLSDemuxed(msg IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	ffmpegPath := findFFmpeg()
	if ffmpegPath == "" {
		pw.sendError(msg.ID, "FFmpeg not found. Install it to download demuxed streams.", "")
		return
	}

	outPath := outputPath(msg.Title, "mp4")

	// Parse the master playlist to find video + audio playlist URLs
	videoPLURL, audioPLURL, err := parseDemuxedPair(msg.URL, msg.SelectedQuality, msg.Qualities, msg.Headers, msg.Cookies)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Failed to parse demuxed playlists: %v", err), "video-segments")
		return
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "muxing")
		return
	}

	pw.sendProgress(msg.ID, "downloading", 0, 0, "", true)

	// Build ffmpeg args — let ffmpeg handle HLS segment downloading + muxing
	args := []string{}

	// Add headers if present
	headerStr := buildFFmpegHeaders(msg.Headers, msg.Cookies)
	if headerStr != "" {
		args = append(args, "-headers", headerStr)
	}

	args = append(args,
		"-i", videoPLURL,
		"-i", audioPLURL,
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-y",
		outPath,
	)

	err = runFFmpegWithProgress(ffmpegPath, args, msg.DurationSeconds, cancel,
		func(pct int) {
			pw.sendProgress(msg.ID, "downloading", pct, 0, "", false)
		})
	if err != nil {
		if isCancelled(cancel) {
			os.Remove(outPath)
			pw.sendError(msg.ID, "Download cancelled.", "downloading")
		} else {
			os.Remove(outPath)
			pw.sendError(msg.ID, fmt.Sprintf("Download failed: %v", err), "downloading")
		}
		return
	}

	fi, _ := os.Stat(outPath)
	finalSize := fi.Size()
	pw.sendProgress(msg.ID, "downloading", 100, finalSize, "", true)
	pw.sendComplete(msg.ID, outPath, finalSize)
}

// buildFFmpegHeaders formats headers and cookies for ffmpeg's -headers flag.
func buildFFmpegHeaders(headers map[string]string, cookies string) string {
	var parts []string
	for k, v := range headers {
		parts = append(parts, fmt.Sprintf("%s: %s\r\n", k, v))
	}
	if cookies != "" {
		parts = append(parts, fmt.Sprintf("Cookie: %s\r\n", cookies))
	}
	return strings.Join(parts, "")
}

// downloadSegmentsToFile downloads all segments and concatenates them into outPath.
// onProgress is called with (completedCount, totalCount, bytesDownloaded, speed).
func downloadSegmentsToFile(segments []Segment, outPath string, headers map[string]string, cookies string,
	tmpDir string, cancel <-chan struct{}, onProgress func(done, total int, bytes int64, speed string)) (int64, error) {

	totalSegments := len(segments)
	segFiles := make([]string, totalSegments)
	var totalBytes atomic.Int64
	var downloadErr error
	var errOnce sync.Once
	startTime := time.Now()
	var done atomic.Int32

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
		go func(idx int, s Segment) {
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
			completed := int(done.Add(1))

			elapsed := time.Since(startTime).Seconds()
			speed := ""
			if elapsed > 0 {
				speed = formatSpeed(float64(totalBytes.Load()) / elapsed)
			}
			if onProgress != nil {
				onProgress(completed, totalSegments, totalBytes.Load(), speed)
			}
		}(i, seg)
	}

	wg.Wait()

	if downloadErr != nil {
		return 0, downloadErr
	}

	// Concatenate segments in order
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

// downloadSegmentWithRetry fetches a single segment with exponential backoff retries.
func downloadSegmentWithRetry(ctx context.Context, seg Segment, outPath string, headers map[string]string, cookies string) (int64, error) {
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

// downloadSegment fetches a single segment and writes it to disk.
// Supports byte-range segments (EXT-X-BYTERANGE).
func downloadSegment(ctx context.Context, seg Segment, outPath string, headers map[string]string, cookies string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", seg.URL, nil)
	if err != nil {
		return 0, err
	}

	applyHeaders(req, headers, cookies)

	// Add byte-range header if this is a byte-range segment
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
