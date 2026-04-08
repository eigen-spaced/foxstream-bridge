package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ffmpegTimeRegex = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

// muxToMP4 merges video and audio .ts files into a single .mp4 using ffmpeg.
// durationSecs is used for progress estimation. Set to 0 if unknown.
func muxToMP4(ffmpegPath, videoPath, audioPath, outputPath string, durationSecs float64, onProgress func(percent int)) error {
	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	}

	cmd := exec.Command(ffmpegPath, args...)

	// ffmpeg writes progress to stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	// Parse ffmpeg's stderr for time= progress lines
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanFFmpegLines)
	for scanner.Scan() {
		line := scanner.Text()
		if durationSecs > 0 && onProgress != nil {
			if secs := parseFFmpegTime(line); secs > 0 {
				pct := int(secs / durationSecs * 100)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	// Verify output isn't empty
	fi, err := os.Stat(outputPath)
	if err != nil || fi.Size() == 0 {
		os.Remove(outputPath)
		return fmt.Errorf("mux produced empty output — stream may be unsupported")
	}

	return nil
}

// remuxToMP4 remuxes a single .ts file to .mp4 (for muxed HLS streams).
func remuxToMP4(ffmpegPath, inputPath, outputPath string, durationSecs float64, onProgress func(percent int)) error {
	args := []string{
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	}

	cmd := exec.Command(ffmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanFFmpegLines)
	for scanner.Scan() {
		line := scanner.Text()
		if durationSecs > 0 && onProgress != nil {
			if secs := parseFFmpegTime(line); secs > 0 {
				pct := int(secs / durationSecs * 100)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}

	fi, err := os.Stat(outputPath)
	if err != nil || fi.Size() == 0 {
		os.Remove(outputPath)
		return fmt.Errorf("mux produced empty output — stream may be unsupported")
	}

	return nil
}

// parseFFmpegTime extracts seconds from a line containing time=HH:MM:SS.ms
func parseFFmpegTime(line string) float64 {
	matches := ffmpegTimeRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return 0
	}
	h, _ := strconv.Atoi(matches[1])
	m, _ := strconv.Atoi(matches[2])
	s, _ := strconv.Atoi(matches[3])
	ms, _ := strconv.Atoi(matches[4])
	return float64(h)*3600 + float64(m)*60 + float64(s) + float64(ms)/100
}

// scanFFmpegLines is a split function for bufio.Scanner that splits on \r or \n.
// ffmpeg uses \r for progress lines (overwriting the same line in a terminal).
func scanFFmpegLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// killFFmpegCmd is a helper to kill an ffmpeg process and clean up.
type ffmpegCmd struct {
	cmd     *exec.Cmd
	started bool
}

func newFFmpegCmd(ffmpegPath string, args ...string) *ffmpegCmd {
	return &ffmpegCmd{
		cmd: exec.Command(ffmpegPath, args...),
	}
}

func (fc *ffmpegCmd) kill() {
	if fc.cmd != nil && fc.cmd.Process != nil && fc.started {
		fc.cmd.Process.Kill()
		// Wait briefly to reap the process
		done := make(chan struct{})
		go func() {
			fc.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}
}

// muxToMP4Cancellable is like muxToMP4 but respects a cancel channel.
func muxToMP4Cancellable(ffmpegPath, videoPath, audioPath, outputPath string, durationSecs float64, onProgress func(percent int), cancel <-chan struct{}) error {
	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	}

	fc := newFFmpegCmd(ffmpegPath, args...)
	cmd := fc.cmd

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	fc.started = true

	// Monitor cancel channel
	go func() {
		select {
		case <-cancel:
			fc.kill()
		}
	}()

	var stderrLines []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanFFmpegLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			stderrLines = append(stderrLines, line)
		}
		if durationSecs > 0 && onProgress != nil {
			if secs := parseFFmpegTime(line); secs > 0 {
				pct := int(secs / durationSecs * 100)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
			}
		}
	}

	// Check if cancelled
	select {
	case <-cancel:
		os.Remove(outputPath)
		return fmt.Errorf("cancelled")
	default:
	}

	if err := cmd.Wait(); err != nil {
		// Could be from kill
		select {
		case <-cancel:
			os.Remove(outputPath)
			return fmt.Errorf("cancelled")
		default:
		}
		// Include last few lines of ffmpeg stderr for diagnostics
		tail := stderrLines
		if len(tail) > 10 {
			tail = tail[len(tail)-10:]
		}
		return fmt.Errorf("ffmpeg: %w\nffmpeg output:\n%s", err, strings.Join(tail, "\n"))
	}

	fi, err := os.Stat(outputPath)
	if err != nil || fi.Size() == 0 {
		os.Remove(outputPath)
		return fmt.Errorf("mux produced empty output — stream may be unsupported")
	}

	return nil
}

// runFFmpegWithProgress runs an ffmpeg command with progress reporting and cancel support.
func runFFmpegWithProgress(ffmpegPath string, args []string, durationSecs float64, cancel <-chan struct{}, onProgress func(percent int)) error {
	fc := newFFmpegCmd(ffmpegPath, args...)
	cmd := fc.cmd

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	fc.started = true

	go func() {
		select {
		case <-cancel:
			fc.kill()
		}
	}()

	var stderrLines []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanFFmpegLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			stderrLines = append(stderrLines, line)
		}
		if durationSecs > 0 && onProgress != nil {
			if secs := parseFFmpegTime(line); secs > 0 {
				pct := int(secs / durationSecs * 100)
				if pct > 100 {
					pct = 100
				}
				onProgress(pct)
			}
		}
	}

	select {
	case <-cancel:
		os.Remove(args[len(args)-1]) // last arg is output path
		return fmt.Errorf("cancelled")
	default:
	}

	if err := cmd.Wait(); err != nil {
		select {
		case <-cancel:
			return fmt.Errorf("cancelled")
		default:
		}
		tail := stderrLines
		if len(tail) > 10 {
			tail = tail[len(tail)-10:]
		}
		return fmt.Errorf("ffmpeg: %w\nffmpeg output:\n%s", err, strings.Join(tail, "\n"))
	}

	return nil
}

// hasFFmpeg checks if ffmpeg is available and returns a user-friendly message.
func hasFFmpeg() (string, bool) {
	path := findFFmpeg()
	if path == "" {
		return "", false
	}
	// Quick version check
	out, err := exec.Command(path, "-version").Output()
	if err != nil {
		return "", false
	}
	// First line has version info
	line := strings.SplitN(string(out), "\n", 2)[0]
	return path + " (" + line + ")", true
}
