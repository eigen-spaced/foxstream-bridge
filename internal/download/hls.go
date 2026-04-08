package download

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"foxstream-bridge/internal/config"
	"foxstream-bridge/internal/ffmpeg"
	"foxstream-bridge/internal/hls"
	"foxstream-bridge/internal/protocol"
)

func downloadHLS(msg protocol.IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	if msg.StreamStructure == "demuxed" {
		downloadHLSDemuxed(msg, pw, cancel)
		return
	}
	downloadHLSMuxed(msg, pw, cancel)
}

func downloadHLSMuxed(msg protocol.IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	outputFormat := ffmpeg.ResolveOutputFormat(msg.OutputFormat)
	outPath := config.OutputPath(msg.Title, outputFormat)

	playlistURL := msg.URL
	if msg.SelectedQuality != "" || len(msg.Qualities) > 0 {
		qualities := toHLSQualities(msg.Qualities)
		resolved, err := hls.FindSelectedVariantURL(msg.URL, msg.SelectedQuality, qualities, msg.Headers, msg.Cookies)
		if err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Failed to resolve quality: %v", err), "video-segments")
			return
		}
		playlistURL = resolved
	}

	segments, err := hls.FetchAndParseMediaPlaylist(playlistURL, msg.Headers, msg.Cookies)
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

	ffmpegPath := config.FindFFmpeg()
	if ffmpegPath != "" {
		pw.sendProgress(msg.ID, "muxing", 90, totalBytes, "", true)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "muxing")
			return
		}

		codecArgs, extraArgs := ffmpeg.FormatArgs(outputFormat, true)
		args := []string{"-i", concatPath}
		args = append(args, codecArgs...)
		args = append(args, extraArgs...)
		args = append(args, "-y", outPath)

		err := ffmpeg.RunWithProgress(args, msg.DurationSeconds, cancel,
			func(pct int) {
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
		outPath = config.OutputPath(msg.Title, "ts")
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

func downloadHLSDemuxed(msg protocol.IncomingMessage, pw *progressWriter, cancel <-chan struct{}) {
	ffmpegPath := config.FindFFmpeg()
	if ffmpegPath == "" {
		pw.sendError(msg.ID, "FFmpeg not found. Install it to download demuxed streams.", "")
		return
	}

	outputFormat := ffmpeg.ResolveOutputFormat(msg.OutputFormat)
	outPath := config.OutputPath(msg.Title, outputFormat)

	qualities := toHLSQualities(msg.Qualities)
	videoPLURL, audioPLURL, err := hls.ParseDemuxedPair(msg.URL, msg.SelectedQuality, qualities, msg.Headers, msg.Cookies)
	if err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Failed to parse demuxed playlists: %v", err), "video-segments")
		return
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		pw.sendError(msg.ID, fmt.Sprintf("Cannot create output dir: %v", err), "muxing")
		return
	}

	pw.sendProgress(msg.ID, "downloading", 0, 0, "", true)

	args := []string{}
	headerStr := buildFFmpegHeaders(msg.Headers, msg.Cookies)
	if headerStr != "" {
		args = append(args, "-headers", headerStr)
	}

	codecArgs, extraArgs := ffmpeg.FormatArgs(outputFormat, true)
	args = append(args, "-i", videoPLURL, "-i", audioPLURL)
	args = append(args, codecArgs...)
	args = append(args, extraArgs...)
	args = append(args, "-y", outPath)

	err = ffmpeg.RunWithProgress(args, msg.DurationSeconds, cancel,
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

// toHLSQualities converts protocol qualities to hls.Quality.
func toHLSQualities(pq []protocol.Quality) []hls.Quality {
	out := make([]hls.Quality, len(pq))
	for i, q := range pq {
		out[i] = hls.Quality{Label: q.Label, Bandwidth: q.Bandwidth, URL: q.URL}
	}
	return out
}
