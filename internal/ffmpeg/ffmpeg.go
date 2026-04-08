package ffmpeg

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"foxstream-bridge/internal/config"
)

var timeRegex = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

// HasFFmpeg checks if ffmpeg is available and returns a user-friendly description.
func HasFFmpeg() (string, bool) {
	path := config.FindFFmpeg()
	if path == "" {
		return "", false
	}
	out, err := exec.Command(path, "-version").Output()
	if err != nil {
		return "", false
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	return path + " (" + line + ")", true
}

// RemuxToMP4 remuxes a single input file to .mp4 (for muxed HLS streams).
func RemuxToMP4(inputPath, outputPath string, durationSecs float64, onProgress func(percent int)) error {
	ffmpegPath := config.FindFFmpeg()
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
	scanner.Split(ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if durationSecs > 0 && onProgress != nil {
			if secs := ParseTime(line); secs > 0 {
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

// RunWithProgress runs an ffmpeg command with progress reporting and cancel support.
func RunWithProgress(args []string, durationSecs float64, cancel <-chan struct{}, onProgress func(percent int)) error {
	ffmpegPath := config.FindFFmpeg()
	fc := &ffmpegCmd{cmd: exec.Command(ffmpegPath, args...)}
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
		<-cancel
		fc.kill()
	}()

	var stderrLines []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			stderrLines = append(stderrLines, line)
		}
		if durationSecs > 0 && onProgress != nil {
			if secs := ParseTime(line); secs > 0 {
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

// MuxToMP4Cancellable merges video and audio files into a single .mp4 with cancel support.
func MuxToMP4Cancellable(videoPath, audioPath, outputPath string, durationSecs float64, onProgress func(percent int), cancel <-chan struct{}) error {
	ffmpegPath := config.FindFFmpeg()
	args := []string{
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	}

	fc := &ffmpegCmd{cmd: exec.Command(ffmpegPath, args...)}
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
		<-cancel
		fc.kill()
	}()

	var stderrLines []string
	scanner := bufio.NewScanner(stderr)
	scanner.Split(ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			stderrLines = append(stderrLines, line)
		}
		if durationSecs > 0 && onProgress != nil {
			if secs := ParseTime(line); secs > 0 {
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
		os.Remove(outputPath)
		return fmt.Errorf("cancelled")
	default:
	}

	if err := cmd.Wait(); err != nil {
		select {
		case <-cancel:
			os.Remove(outputPath)
			return fmt.Errorf("cancelled")
		default:
		}
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

// ParseTime extracts seconds from a line containing time=HH:MM:SS.ms.
func ParseTime(line string) float64 {
	matches := timeRegex.FindStringSubmatch(line)
	if len(matches) < 5 {
		return 0
	}
	h, _ := strconv.Atoi(matches[1])
	m, _ := strconv.Atoi(matches[2])
	s, _ := strconv.Atoi(matches[3])
	ms, _ := strconv.Atoi(matches[4])
	return float64(h)*3600 + float64(m)*60 + float64(s) + float64(ms)/100
}

// ScanLines is a split function for bufio.Scanner that splits on \r or \n.
// ffmpeg uses \r for progress lines (overwriting the same line in a terminal).
func ScanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
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

type ffmpegCmd struct {
	cmd     *exec.Cmd
	started bool
}

func (fc *ffmpegCmd) kill() {
	if fc.cmd != nil && fc.cmd.Process != nil && fc.started {
		fc.cmd.Process.Kill()
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
